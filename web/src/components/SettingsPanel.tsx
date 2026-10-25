import { useEffect, useState, type FormEvent } from "react";
import { api } from "../api/client";
import type { AppConfig, KataGoConfig } from "../api/types";
import { useToast } from "../hooks/useToast";

interface Props {
  config: AppConfig;
  onRefresh: () => void;
}

export function SettingsPanel({ config, onRefresh }: Props) {
  const toast = useToast();
  const [paths, setPaths] = useState<KataGoConfig>(config.katago.config);
  const [saving, setSaving] = useState(false);
  const [fetching, setFetching] = useState(false);

  useEffect(() => setPaths(config.katago.config), [config.katago.config]);

  // KataGo takes a while to start after a settings change; keep polling until it settles.
  useEffect(() => {
    if (!config.katago.starting) return;
    const t = setInterval(onRefresh, 2000);
    return () => clearInterval(t);
  }, [config.katago.starting, onRefresh]);

  const save = async (e: FormEvent) => {
    e.preventDefault();
    setSaving(true);
    try {
      await api.setKatago(paths);
      toast.info("Saved. Restarting KataGo...");
      onRefresh();
    } catch (err) {
      toast.error(err);
    } finally {
      setSaving(false);
    }
  };

  const download = async () => {
    setFetching(true);
    try {
      await api.fetchJoseki();
      toast.info("Joseki dictionary downloaded.");
      onRefresh();
    } catch (err) {
      toast.error(err);
    } finally {
      setFetching(false);
    }
  };

  const k = config.katago;
  const j = config.joseki;
  const set = (key: keyof KataGoConfig) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setPaths((p) => ({ ...p, [key]: e.target.value }));

  return (
    <details className="panel">
      <summary>
        <h2 className="inline">Settings</h2>
      </summary>
      <form className="form" onSubmit={(e) => void save(e)}>
        <h3>KataGo</h3>
        <p className={k.available ? "status-ok" : "status-bad"}>
          {k.available
            ? `Ready${k.version ? ` (version ${k.version})` : ""}`
            : k.starting
              ? "Starting..."
              : `Not running${k.error ? `: ${k.error}` : ""}`}
        </p>
        <label className="field">
          <span>Binary</span>
          <input value={paths.binary} onChange={set("binary")} />
        </label>
        <label className="field">
          <span>Model</span>
          <input value={paths.model} onChange={set("model")} />
        </label>
        <label className="field">
          <span>Analysis config</span>
          <input value={paths.config} onChange={set("config")} />
        </label>
        <button className="btn" type="submit" disabled={saving}>
          Save and restart KataGo
        </button>
      </form>
      <h3>Joseki dictionary</h3>
      <p className={j.loaded ? "status-ok" : "status-bad"}>
        {j.loaded
          ? `Loaded: ${j.positions.toLocaleString()} positions from ${j.source}`
          : `Not loaded${j.error ? `: ${j.error}` : ""}`}
      </p>
      {!j.loaded && (
        <button className="btn" disabled={fetching} onClick={() => void download()}>
          {fetching ? "Downloading..." : "Download dictionary"}
        </button>
      )}
    </details>
  );
}
