<#
.SYNOPSIS
  Installs KataGo (OpenCL build) and a neural network outside the repo.

.DESCRIPTION
  Downloads the newest KataGo release that has a Windows build of the chosen
  backend, and the newest network from katagotraining.org (checked against the
  SHA-256 the site publishes). Layout created under -Dir:

    bin\katago.exe        the engine and its DLLs
    analysis.cfg          copy of the example analysis config
    nets\<network>.bin.gz
    human\b18c384nbt-humanv0.bin.gz   rank-based opponents (use -SkipHuman to omit)

  The OpenCL build runs on the GPU driver alone. CUDA and TensorRT builds are
  faster on NVIDIA cards but need a system-wide CUDA/cuDNN install first.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File scripts\setup-katago.ps1
#>
param(
  [string]$Dir = (Join-Path $env:USERPROFILE "katago"),
  [string]$Backend = "opencl",
  [switch]$SkipNet,
  [switch]$SkipHuman
)
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$headers = @{ "User-Agent" = "easygo-setup" }

New-Item -ItemType Directory -Force -Path (Join-Path $Dir "dl"), (Join-Path $Dir "nets") | Out-Null

# --- engine -------------------------------------------------------------
$releases = Invoke-RestMethod "https://api.github.com/repos/lightvector/KataGo/releases?per_page=15" -Headers $headers
$asset = $null
foreach ($r in $releases) {
  $asset = $r.assets | Where-Object { $_.name -like "katago-*-$Backend-windows-x64.zip" } | Select-Object -First 1
  if ($asset) { Write-Host "KataGo $($r.tag_name): $($asset.name)"; break }
}
if (-not $asset) { throw "No Windows '$Backend' build found in the 15 newest KataGo releases." }

$binDir = Join-Path $Dir "bin"
$stamp = Join-Path $binDir ".installed"
$installed = if (Test-Path $stamp) { (Get-Content $stamp -Raw).Trim() } else { "" }
if ($installed -eq $asset.name -and (Test-Path (Join-Path $binDir "katago.exe"))) {
  # Already current. Leaving it alone also avoids replacing a katago.exe that
  # a running easygo server has open, which Windows refuses.
  Write-Host "Engine already installed ($installed)."
} else {
  $zip = Join-Path $Dir "dl\$($asset.name)"
  if (-not (Test-Path $zip)) { Invoke-WebRequest $asset.browser_download_url -OutFile $zip -Headers $headers }
  try {
    Expand-Archive -Path $zip -DestinationPath $binDir -Force
  } catch {
    throw "Could not replace the engine in $binDir. If easygo or katago is running, stop it and run this again. ($($_.Exception.Message))"
  }
  Set-Content -Path $stamp -Value $asset.name
}
$cfg = Join-Path $Dir "analysis.cfg"
if (-not (Test-Path $cfg)) {
  # The example writes logs to a relative folder, i.e. into whatever directory
  # easygo is started from (the repo). Send them to the KataGo folder instead.
  $logs = (Join-Path $Dir "logs") -replace '\\', '/'
  (Get-Content (Join-Path $Dir "bin\analysis_example.cfg")) -replace '^logDir\s*=.*$', "logDir = $logs" | Set-Content $cfg
}

# --- network ------------------------------------------------------------
if (-not $SkipNet) {
  $net = (Invoke-RestMethod "https://katagotraining.org/api/networks/?format=json&ordering=-created_at&limit=1" -Headers $headers).results[0]
  $file = Join-Path $Dir "nets\$($net.name).bin.gz"
  if (-not (Test-Path $file)) {
    Write-Host "Network $($net.name) ($([math]::Round($net.model_file_bytes / 1MB)) MB)"
    Invoke-WebRequest $net.model_file -OutFile $file -Headers $headers
  }
  $hash = (Get-FileHash $file -Algorithm SHA256).Hash.ToLower()
  if ($hash -ne $net.model_file_sha256.ToLower()) {
    Remove-Item $file
    throw "Checksum mismatch for $($net.name): got $hash, expected $($net.model_file_sha256)."
  }
  Write-Host "Checksum OK."
}


# --- human-style model --------------------------------------------------
# Lets KataGo imitate a human of a chosen rank (20k to 9d). KataGo publishes it
# on its v1.15.0 release but no checksum, so the size is checked against the
# release listing and the SHA-256 is printed for your records.
if (-not $SkipHuman) {
  $rel = Invoke-RestMethod "https://api.github.com/repos/lightvector/KataGo/releases/tags/v1.15.0" -Headers $headers
  $h = $rel.assets | Where-Object { $_.name -eq "b18c384nbt-humanv0.bin.gz" } | Select-Object -First 1
  if (-not $h) { throw "The human model was not found on KataGo's v1.15.0 release." }
  $humanDir = Join-Path $Dir "human"
  New-Item -ItemType Directory -Force -Path $humanDir | Out-Null
  $humanFile = Join-Path $humanDir $h.name
  if (-not (Test-Path $humanFile) -or (Get-Item $humanFile).Length -ne $h.size) {
    Write-Host "Human-style model $($h.name) ($([math]::Round($h.size / 1MB)) MB)"
    Invoke-WebRequest $h.browser_download_url -OutFile $humanFile -Headers $headers
  }
  if ((Get-Item $humanFile).Length -ne $h.size) {
    Remove-Item $humanFile
    throw "Human model download is the wrong size (expected $($h.size) bytes)."
  }
  Write-Host "Human model OK. SHA-256 $((Get-FileHash $humanFile -Algorithm SHA256).Hash.ToLower())"
}

Write-Host ""
& (Join-Path $Dir "bin\katago.exe") version
Write-Host ""
Write-Host "Done. Start the app with:  easygo -katago-dir `"$Dir`""
Write-Host "The first run tunes the GPU and can take a few minutes."
