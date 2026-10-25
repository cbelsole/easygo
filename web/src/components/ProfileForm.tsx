import { useState, type FormEvent } from "react";
import { api } from "../api/client";
import type { User } from "../api/types";
import { useToast } from "../hooks/useToast";

export function ProfileForm({ user, onSaved }: { user: User; onSaved: (u: User) => void }) {
  const toast = useToast();
  const [name, setName] = useState(user.name);
  const [rank, setRank] = useState(user.rank);
  const [location, setLocation] = useState(user.location);
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      const { user: u } = await api.updateMe({
        name: name.trim(),
        rank: rank.trim(),
        location: location.trim(),
      });
      onSaved(u);
      toast.info("Profile saved.");
    } catch (err) {
      toast.error(err);
    } finally {
      setBusy(false);
    }
  };

  return (
    <details className="panel">
      <summary>
        <h2 className="inline">Profile</h2>
      </summary>
      <form className="form" onSubmit={(e) => void submit(e)}>
        <div className="grid3">
          <label className="field">
            <span>Name</span>
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <label className="field">
            <span>Rank (for example 5k)</span>
            <input value={rank} onChange={(e) => setRank(e.target.value)} />
          </label>
          <label className="field">
            <span>Location</span>
            <input value={location} onChange={(e) => setLocation(e.target.value)} />
          </label>
        </div>
        <button className="btn" type="submit" disabled={busy}>
          Save profile
        </button>
      </form>
    </details>
  );
}
