import { useState, type FormEvent } from "react";
import { api } from "../api/client";
import type { AppConfig, User } from "../api/types";
import { useToast } from "../hooks/useToast";

export function SignIn({
  config,
  onSignedIn,
}: {
  config: AppConfig;
  onSignedIn: (u: User) => void;
}) {
  const toast = useToast();
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    setBusy(true);
    try {
      onSignedIn(await api.devLogin(name.trim()));
    } catch (err) {
      toast.error(err);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="panel signin">
      <h1>Go on Easy Mode</h1>
      <p className="muted">Sign in to play, review and practice.</p>
      {config.googleAuth && (
        <a className="btn btn-primary" href="/auth/google/login">
          Sign in with Google
        </a>
      )}
      {config.devLogin && (
        <form className="form" onSubmit={(e) => void submit(e)}>
          <h2>Dev sign-in</h2>
          <label className="field">
            <span>Name</span>
            <input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
          </label>
          <button className="btn" type="submit" disabled={busy || !name.trim()}>
            Sign in
          </button>
        </form>
      )}
      {!config.googleAuth && !config.devLogin && (
        <p className="status-bad">No sign-in method is enabled on this server.</p>
      )}
    </div>
  );
}
