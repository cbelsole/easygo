import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";

interface Toast {
  id: number;
  text: string;
  tone: "error" | "info";
}

interface ToastApi {
  error: (e: unknown) => void;
  info: (text: string) => void;
}

const ToastContext = createContext<ToastApi>({ error: () => {}, info: () => {} });

let nextId = 1;

export const messageOf = (e: unknown): string => (e instanceof Error ? e.message : String(e));

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const push = useCallback((text: string, tone: Toast["tone"]) => {
    const id = nextId++;
    setToasts((t) => [...t, { id, text, tone }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 5000);
  }, []);

  const api = useMemo<ToastApi>(
    () => ({ error: (e) => push(messageOf(e), "error"), info: (t) => push(t, "info") }),
    [push],
  );

  return (
    <ToastContext.Provider value={api}>
      {children}
      <div className="toasts" role="status" aria-live="polite">
        {toasts.map((t) => (
          <div key={t.id} className={`toast toast-${t.tone}`}>
            {t.text}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export const useToast = () => useContext(ToastContext);
