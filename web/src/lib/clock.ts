import type { ClockView } from "../api/types";

export interface ClockReading {
  inByo: boolean;
  /** Milliseconds left in the current phase (main time, or the current byo-yomi period). */
  ms: number;
  periods: number;
  flagged: boolean;
}

/**
 * Project a server snapshot forward by `elapsedMs`. Only a running clock moves.
 * The server decides actual flagging; locally we just stop at zero.
 */
export function readClock(c: ClockView, elapsedMs: number): ClockReading {
  const base = { inByo: c.inByo, ms: c.inByo ? c.periodMs : c.mainMs, periods: c.periods };
  if (!c.enabled || !c.running || c.flagged || elapsedMs <= 0) {
    return { ...base, flagged: c.flagged };
  }
  if (c.inByo) return burnByo(c, c.periodMs, c.periods, elapsedMs);
  if (elapsedMs < c.mainMs) return { ...base, ms: c.mainMs - elapsedMs, flagged: false };
  if (!c.byoConfigured || c.config.periods <= 0)
    return { inByo: false, ms: 0, periods: 0, flagged: true };
  return burnByo(c, c.config.periodSeconds * 1000, c.periods, elapsedMs - c.mainMs, true);
}

function burnByo(
  c: ClockView,
  periodMs: number,
  periods: number,
  elapsed: number,
  fresh = false,
): ClockReading {
  const full = c.config.periodSeconds * 1000;
  let left = periodMs;
  let remaining = periods;
  let spent = elapsed;
  // When entering byo-yomi the first period starts full; afterwards each expiry consumes one.
  if (fresh) left = full;
  while (spent >= left) {
    spent -= left;
    remaining -= 1;
    if (remaining <= 0) return { inByo: true, ms: 0, periods: 0, flagged: true };
    left = full;
  }
  return { inByo: true, ms: left - spent, periods: remaining, flagged: false };
}

/** Main time as m:ss (h:mm:ss from an hour); byo-yomi as whole seconds. */
export function formatClock(r: Pick<ClockReading, "inByo" | "ms">): string {
  const secs = Math.ceil(Math.max(0, r.ms) / 1000);
  if (r.inByo) return String(secs);
  const h = Math.floor(secs / 3600);
  const m = Math.floor((secs % 3600) / 60);
  const s = secs % 60;
  const ss = String(s).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${ss}` : `${m}:${ss}`;
}

export function isLow(r: ClockReading): boolean {
  return r.inByo ? r.ms <= 10_000 : r.ms <= 30_000;
}
