import { useCallback, useEffect, useState } from "react";
import { api } from "./api/client";
import type { AppConfig, User } from "./api/types";
import { GameScreen } from "./components/GameScreen";
import { Lobby } from "./components/Lobby";
import { SignIn } from "./components/SignIn";
import { ToastProvider, useToast } from "./hooks/useToast";

/** Hash routing: "#/game/<id>" is a game, anything else is the lobby. */
function useGameRoute(): [string | null, (id: string | null) => void] {
  const read = () => /^#\/game\/(.+)$/.exec(window.location.hash)?.[1] ?? null;
  const [id, setId] = useState(read);
  useEffect(() => {
    const on = () => setId(read());
    window.addEventListener("hashchange", on);
    return () => window.removeEventListener("hashchange", on);
  }, []);
  const go = useCallback((next: string | null) => {
    window.location.hash = next ? `#/game/${next}` : "#/";
  }, []);
  return [id ? decodeURIComponent(id) : null, go];
}

function Shell() {
  const toast = useToast();
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [user, setUser] = useState<User | null | undefined>(undefined);
  const [gameId, openGame] = useGameRoute();
  const [fatal, setFatal] = useState<string | null>(null);

  const refreshConfig = useCallback(() => {
    api.config().then(setConfig).catch(toast.error);
  }, [toast.error]);

  useEffect(() => {
    Promise.all([api.config(), api.me()])
      .then(([c, m]) => {
        setConfig(c);
        setUser(m.user);
      })
      .catch((e: unknown) => setFatal(e instanceof Error ? e.message : String(e)));
  }, []);

  if (fatal) return <p className="notice notice-error">Cannot reach the server: {fatal}</p>;
  if (!config || user === undefined) return <p className="muted loading">Loading...</p>;

  const signOut = async () => {
    try {
      await api.logout();
    } finally {
      setUser(null);
      openGame(null);
    }
  };

  return (
    <>
      <header className="topbar">
        <a href="#/" className="brand">
          Go on Easy Mode
        </a>
        {user && (
          <div className="row">
            <span>{user.name}</span>
            <button className="btn btn-quiet" onClick={() => void signOut()}>
              Sign out
            </button>
          </div>
        )}
      </header>
      <main>
        {!user ? (
          <SignIn config={config} onSignedIn={setUser} />
        ) : gameId ? (
          <GameScreen key={gameId} id={gameId} config={config} onExit={() => openGame(null)} />
        ) : (
          <Lobby
            user={user}
            config={config}
            onOpen={openGame}
            onUser={setUser}
            onRefreshConfig={refreshConfig}
          />
        )}
      </main>
    </>
  );
}

export function App() {
  return (
    <ToastProvider>
      <Shell />
    </ToastProvider>
  );
}
