import { useCallback, useEffect, useRef, useState } from "react";
import { api, eventsUrl, normalizeView } from "../api/client";
import type { GameView } from "../api/types";

export interface GameState {
  view: GameView | null;
  /** Date.now() when the current view arrived; clocks count down from here. */
  receivedAt: number;
  connected: boolean;
  loadError: string | null;
  /** Apply a view returned by a mutating call. */
  accept: (v: GameView) => void;
}

/** Loads a game, then follows its SSE stream, reconnecting if the stream drops. */
export function useGame(id: string): GameState {
  const [view, setView] = useState<GameView | null>(null);
  const [receivedAt, setReceivedAt] = useState(() => Date.now());
  const [connected, setConnected] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const version = useRef(-1);

  const accept = useCallback((v: GameView) => {
    if (v.version < version.current) return;
    version.current = v.version;
    setView(v);
    setReceivedAt(Date.now());
  }, []);

  useEffect(() => {
    let closed = false;
    let source: EventSource | null = null;
    let retry: ReturnType<typeof setTimeout> | undefined;
    version.current = -1;
    setView(null);
    setLoadError(null);

    api
      .getGame(id)
      .then((v) => !closed && accept(v))
      .catch((e: unknown) => !closed && setLoadError(e instanceof Error ? e.message : String(e)));

    const connect = () => {
      source = new EventSource(eventsUrl(id));
      source.onopen = () => setConnected(true);
      source.addEventListener("state", (ev) => {
        try {
          accept(normalizeView(JSON.parse((ev as MessageEvent<string>).data) as GameView));
        } catch {
          // A malformed frame is skipped; the next one replaces it.
        }
      });
      source.onerror = () => {
        setConnected(false);
        if (source?.readyState === EventSource.CLOSED && !closed) {
          retry = setTimeout(connect, 2000);
        }
      };
    };
    connect();

    return () => {
      closed = true;
      clearTimeout(retry);
      source?.close();
    };
  }, [id, accept]);

  return { view, receivedAt, connected, loadError, accept };
}
