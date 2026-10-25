import { useCallback, useEffect, useState, type DragEvent } from "react";
import { api } from "../api/client";
import type { AppConfig, GameList, GameSummary, User } from "../api/types";
import { useToast } from "../hooks/useToast";
import { NewGameForm } from "./NewGameForm";
import { ProfileForm } from "./ProfileForm";
import { SettingsPanel } from "./SettingsPanel";

interface Props {
  user: User;
  config: AppConfig;
  onOpen: (id: string) => void;
  onUser: (u: User) => void;
  onRefreshConfig: () => void;
}

const modeLabel = { live: "Live", teaching: "Teaching", joseki: "Joseki" } as const;

export function Lobby({ user, config, onOpen, onUser, onRefreshConfig }: Props) {
  const toast = useToast();
  const [games, setGames] = useState<GameList | null>(null);

  const reload = useCallback(() => {
    api.listGames().then(setGames).catch(toast.error);
  }, [toast.error]);

  useEffect(() => {
    reload();
    const t = setInterval(reload, 10000);
    return () => clearInterval(t);
  }, [reload]);

  const join = async (g: GameSummary) => {
    try {
      await api.join(g.id);
      onOpen(g.id);
    } catch (e) {
      toast.error(e);
      reload();
    }
  };

  const remove = async (g: GameSummary) => {
    if (!window.confirm(`Delete "${g.name}"?`)) return;
    try {
      await api.deleteGame(g.id);
      reload();
    } catch (e) {
      toast.error(e);
    }
  };

  const openJoseki = async () => {
    try {
      onOpen((await api.josekiExample()).id);
    } catch (e) {
      toast.error(e);
    }
  };

  return (
    <div className="lobby">
      <div className="lobby-col">
        <NewGameForm config={config} onCreated={(g) => onOpen(g.id)} />
        <ImportSgf onImported={onOpen} />
      </div>
      <div className="lobby-col">
        <section className="panel">
          <h2>My games</h2>
          {games === null ? (
            <p className="muted">Loading...</p>
          ) : games.mine.length === 0 ? (
            <p className="muted">No games yet. Start one on the left.</p>
          ) : (
            <ul className="game-list">
              {games.mine.map((g) => (
                <li key={g.id}>
                  <div>
                    <strong>{g.name}</strong>
                    <div className="muted">
                      {modeLabel[g.mode]} - {g.black} vs {g.white} - {g.status}
                      {g.result ? ` (${g.result})` : ""}
                    </div>
                  </div>
                  <div className="row">
                    <button className="btn" onClick={() => onOpen(g.id)}>
                      Resume
                    </button>
                    {g.owner === user.id && (
                      <button
                        className="btn btn-quiet"
                        aria-label={`Delete ${g.name}`}
                        onClick={() => void remove(g)}
                      >
                        Delete
                      </button>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section className="panel">
          <h2>Open games</h2>
          {games && games.open.length === 0 ? (
            <p className="muted">Nobody is waiting for an opponent.</p>
          ) : (
            <ul className="game-list">
              {(games?.open ?? []).map((g) => (
                <li key={g.id}>
                  <div>
                    <strong>{g.name}</strong>
                    <div className="muted">Started by {g.black || g.white}</div>
                  </div>
                  <button className="btn btn-primary" onClick={() => void join(g)}>
                    Join
                  </button>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section className="panel">
          <h2>Joseki dictionary</h2>
          {config.joseki.loaded ? (
            <>
              <p className="muted">
                {config.joseki.positions.toLocaleString()} positions from {config.joseki.source}.
              </p>
              <button className="btn" onClick={() => void openJoseki()}>
                Open Joseki Dictionary
              </button>
            </>
          ) : (
            <>
              <p className="muted">The dictionary is not downloaded yet.</p>
              <button
                className="btn"
                onClick={() => {
                  api
                    .fetchJoseki()
                    .then(() => {
                      toast.info("Joseki dictionary downloaded.");
                      onRefreshConfig();
                    })
                    .catch(toast.error);
                }}
              >
                Download dictionary
              </button>
            </>
          )}
        </section>

        <ProfileForm user={user} onSaved={onUser} />
        <SettingsPanel config={config} onRefresh={onRefreshConfig} />
      </div>
    </div>
  );
}

function ImportSgf({ onImported }: { onImported: (id: string) => void }) {
  const toast = useToast();
  const [text, setText] = useState("");
  const [mode, setMode] = useState<"teaching" | "joseki">("teaching");
  const [busy, setBusy] = useState(false);
  const [over, setOver] = useState(false);

  const readFile = async (f: File | undefined) => {
    if (f) setText(await f.text());
  };
  const onDrop = (e: DragEvent) => {
    e.preventDefault();
    setOver(false);
    void readFile(e.dataTransfer.files[0]);
  };

  const submit = async () => {
    setBusy(true);
    try {
      onImported((await api.importSgf(text, mode)).id);
    } catch (e) {
      toast.error(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <section
      className={`panel form ${over ? "drop-over" : ""}`}
      onDragOver={(e) => {
        e.preventDefault();
        setOver(true);
      }}
      onDragLeave={() => setOver(false)}
      onDrop={onDrop}
    >
      <h2>Import SGF</h2>
      <label className="field">
        <span>Paste SGF text, drop a .sgf file here, or choose one</span>
        <textarea
          rows={4}
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="(;GM[1]SZ[19];B[pd];W[dp])"
        />
      </label>
      <div className="row">
        <input
          type="file"
          accept=".sgf,text/plain"
          aria-label="Choose an SGF file"
          onChange={(e) => void readFile(e.target.files?.[0])}
        />
        <select
          value={mode}
          aria-label="Open as"
          onChange={(e) => setMode(e.target.value as "teaching" | "joseki")}
        >
          <option value="teaching">Open as Teaching game</option>
          <option value="joseki">Open as Joseki example</option>
        </select>
        <button className="btn" disabled={busy || !text.trim()} onClick={() => void submit()}>
          Import
        </button>
      </div>
    </section>
  );
}
