import { useEffect, useState } from "react";
import { ApiError } from "../api/client";

export interface Remote<T> {
  data: T | null;
  loading: boolean;
  /** The server's message when it refused (403/409). */
  refused: string | null;
}

/**
 * Debounced fetch keyed by `key`. Pass a null fetcher to switch it off.
 * Stale responses are dropped so a quick sequence of moves never shows an old answer.
 */
export function useRemote<T>(
  fetcher: (() => Promise<T>) | null,
  key: string,
  delayMs = 300,
): Remote<T> {
  const [state, setState] = useState<Remote<T>>({ data: null, loading: false, refused: null });
  const active = fetcher !== null;

  useEffect(() => {
    if (!fetcher) {
      setState({ data: null, loading: false, refused: null });
      return;
    }
    let stale = false;
    setState((s) => ({ ...s, loading: true }));
    const timer = setTimeout(() => {
      fetcher()
        .then((data) => !stale && setState({ data, loading: false, refused: null }))
        .catch((e: unknown) => {
          if (stale) return;
          const refused = e instanceof ApiError && (e.status === 403 || e.status === 409);
          setState({
            data: null,
            loading: false,
            refused: refused ? (e as ApiError).message : e instanceof Error ? e.message : String(e),
          });
        });
    }, delayMs);
    return () => {
      stale = true;
      clearTimeout(timer);
    };
    // The key stands in for everything the fetcher closes over.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, active, delayMs]);

  return state;
}
