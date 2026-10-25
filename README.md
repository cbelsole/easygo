# Go on Easy Mode

A local-first Go (baduk) client with OGS-style player cards and clocks, a KataGo coach, a live joseki
recognizer, and a tactical shape sensor. Go backend, React frontend, SQLite storage.

See `docs/spec.md` for the product spec, `docs/decisions.md` for the choices made, and
`docs/api.md` for the HTTP API.

## What is in it

| Feature | How it works | Needs KataGo |
|---|---|---|
| Board, stones, captures, ko, 9/13/19 | Go rules engine (`internal/board`), server-validated | no |
| Rules: Japanese, Chinese, Korean, AGA, New Zealand, Tromp-Taylor, stone scoring, or custom | One rules struct that mirrors KataGo's, so the engine enforces exactly what KataGo analyzes | no |
| Live / Teaching / Joseki Example modes | `internal/session` | no |
| Branching move tree, comments, annotations, SGF import/export | `internal/tree`, `internal/sgf` (hand-written) | no |
| Main time + Japanese byo-yomi, per-game on/off | `internal/clock`, server decides timeouts | no |
| Liberties, settled territory | engine | no |
| Ladders (with breakers), nets, snapbacks, knight / large knight | `internal/shapes` | no |
| Live joseki recognition across all 8 symmetries and both colors | `internal/joseki`, position-hash lookup in Kogo's dictionary | no |
| Next-move suggestions, contested territory, life and death, strong / weak groups | `internal/katago` | yes |
| Computer opponent | KataGo, strength set by visit count | yes |
| Accounts, networked games | Google sign-in, server-sent events | no |

Features that need KataGo are disabled when it is not running.

## Quick start (Windows)

Prerequisites: Go, [pnpm](https://pnpm.io) 12 (`volta install pnpm` or `npm i -g pnpm`; `packageManager` in `web/package.json` makes pnpm 10+ use the exact version), and Node 22.18+ / 24.11+ / 26 (Vite+ requires it). `web/package.json` pins Node
26.12.1 for [Volta](https://volta.sh), so if you use Volta, `pnpm` inside `web/` switches to it automatically
without touching your global Node. Without Volta, install a recent Node yourself.

```powershell
# 1. KataGo (engine + newest network), installed outside the repo
powershell -ExecutionPolicy Bypass -File scripts\setup-katago.ps1

# 2. Joseki dictionary (downloaded for your own use, see "Joseki data" below)
go run ./cmd/easygo fetch-joseki

# 3. Frontend
cd web
pnpm install
pnpm run build
cd ..

# 4. Run
go run ./cmd/easygo
```

Open http://127.0.0.1:8080 and sign in with a name (dev login is enabled automatically when the server
listens on localhost and Google is not configured).

To produce one self-contained executable that serves its own frontend (after `pnpm run build`):

```powershell
go build -tags embedweb -o bin\easygo.exe .\cmd\easygo
```

For frontend development run the Go server as above and `pnpm run dev` inside `web/`
(Vite+ on port 5173, proxying `/api` and `/auth` to the Go server).

## Configuration

Flags, with environment variables as defaults:

| Flag | Env | Default |
|---|---|---|
| `-addr` | `EASYGO_ADDR` | `127.0.0.1:8080` |
| `-data` | `EASYGO_DATA` | `data` (SQLite database, secret key, joseki cache) |
| `-web` | `EASYGO_WEB` | `web/dist` |
| `-base-url` | `EASYGO_BASE_URL` | derived from `-addr`; set to the public URL when deployed |
| `-katago-dir` | `EASYGO_KATAGO_DIR` | `%USERPROFILE%\katago` |
| | `EASYGO_KATAGO_BIN`, `_MODEL`, `_CONFIG` | override the three KataGo paths individually |
| | `EASYGO_SECRET` | hex key that signs session cookies; generated into `data/secret.key` otherwise |
| | `EASYGO_ADMINS` | comma-separated emails allowed to change KataGo paths and download data |
| | `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` | enables Google sign-in |
| | `EASYGO_DEV_LOGIN=1` | force name-only sign-in on (never on a public server) |

KataGo paths can also be changed in the app (Settings) by an administrator; the change is saved in
`data/settings.json`.

## Google sign-in

Only you can create the credentials:

1. Go to https://console.cloud.google.com/apis/credentials and create (or choose) a project.
2. Configure the OAuth consent screen (External is fine for testing; add yourself as a test user).
3. Create credentials, OAuth client ID, type "Web application".
4. Add an authorized redirect URI: `<base-url>/auth/google/callback`, for example
   `http://localhost:8080/auth/google/callback` locally.
5. Start the server with:
   ```powershell
   $env:GOOGLE_CLIENT_ID = "..."
   $env:GOOGLE_CLIENT_SECRET = "..."
   $env:EASYGO_ADMINS = "you@example.com"
   go run ./cmd/easygo
   ```
   With Google configured, name-only dev login is off unless `EASYGO_DEV_LOGIN=1`.

## Joseki data

The live suggester uses Kogo's Joseki Dictionary (copyright Gary Odom, with contributions by Alexandre
Dinerchtein, Andre Ay and Stefan Verstraeten). Its header asks that it not be distributed without
permission, so this repository does not contain it. `easygo fetch-joseki` downloads it from its
author's site into `data/cache/` (git-ignored) for your own use. If you deploy this app publicly,
ask the author first (contact address is in the file's header). The parser tolerates the dictionary's
quirks: tenuki as an empty move, mid-tree setup stones, duplicate variations.

## Tests

```powershell
go test ./...          # engine, rules, SGF, shapes, clock, joseki, auth, store, HTTP API
cd web; pnpm test      # frontend logic
```

The KataGo tests (`internal/katago`, `internal/server`) run against a real KataGo and are skipped
when none is installed. Set `EASYGO_KATAGO_DIR` if it is not in `%USERPROFILE%\katago`.

## Known limits

* The SEKI tax rule works by leaving shared liberties uncounted, which is correct for ordinary seki; the
  ALL tax rule (stone scoring) is accepted and sent to KataGo but is scored like SEKI locally.
  Button go (`hasButton`) is passed to KataGo but not scored locally.
* Nets and snapbacks are depth-limited local searches. They are flagged "bounded" because ko and seki
  are not read out.
* Large knight's move is the (2,3) offset. The spec's "(1,3)" is not a standard named shape and is not detected.
* Joseki recognition covers 13x13 and 19x19 and needs 3 stones in a corner window.
* KataGo setup uses the OpenCL build. CUDA/TensorRT are faster on NVIDIA cards but need a system-wide CUDA install.
