export const DEFAULT_RANK = "16k";
const KEY = "easygo.aiRank";

/** The rank to preselect: the last one used if still offered, else the default. */
export function pickRank(ranks: string[], stored: string | null): string {
  if (stored && ranks.includes(stored)) return stored;
  if (ranks.includes(DEFAULT_RANK)) return DEFAULT_RANK;
  return ranks[0] ?? DEFAULT_RANK;
}

function storage(): Storage | null {
  try {
    return globalThis.localStorage ?? null;
  } catch {
    return null; // blocked or unavailable (private windows)
  }
}

export function loadRank(ranks: string[]): string {
  let stored: string | null = null;
  try {
    stored = storage()?.getItem(KEY) ?? null;
  } catch {
    stored = null;
  }
  return pickRank(ranks, stored);
}

export function saveRank(rank: string): void {
  try {
    storage()?.setItem(KEY, rank);
  } catch {
    // remembering the choice is a convenience, never required
  }
}

/** "16k" -> "16 kyu", "3d" -> "3 dan", for places with room to spell it out. */
export function spellRank(rank: string): string {
  const m = /^(\d+)([kd])$/.exec(rank);
  if (!m) return rank;
  return `${m[1]} ${m[2] === "k" ? "kyu" : "dan"}`;
}
