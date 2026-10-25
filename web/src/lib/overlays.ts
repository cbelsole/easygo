import type { GroupAnalysis, GroupStrength, Mode } from "../api/types";
import { indexOf } from "./coords";

export type FeatureKey =
  | "shadow"
  | "coords"
  | "liberties"
  | "territory"
  | "liveTerritory"
  | "annotations"
  | "katago"
  | "joseki"
  | "shapes"
  | "lifeDeath"
  | "strength";

export interface Availability {
  enabled: boolean;
  reason?: string;
}

export interface Environment {
  mode: Mode;
  katago: boolean;
  /** Server-side message when analysis was refused (for example AI help is off). */
  katagoBlocked?: string;
  josekiLoaded: boolean;
}

const NEEDS_KATAGO: FeatureKey[] = ["liveTerritory", "katago", "lifeDeath", "strength"];

/** The toggle matrix from the spec, plus runtime availability of KataGo and the joseki book. */
export function availability(key: FeatureKey, env: Environment): Availability {
  const playing = env.mode === "live" || env.mode === "teaching";
  if (NEEDS_KATAGO.includes(key)) {
    if (!playing) return { enabled: false, reason: "Not available in Joseki Example games" };
    if (!env.katago) return { enabled: false, reason: "KataGo is not running" };
    if (env.katagoBlocked) return { enabled: false, reason: env.katagoBlocked };
  }
  if (key === "joseki") {
    if (!playing) return { enabled: false, reason: "The book is the game itself in Joseki Example mode" };
    if (!env.josekiLoaded) return { enabled: false, reason: "The joseki dictionary is not downloaded" };
  }
  if (key === "annotations" && env.mode === "live") {
    return { enabled: false, reason: "Annotations are for Teaching and Joseki games" };
  }
  return { enabled: true };
}

export const needsAnalysis = (key: FeatureKey): boolean => NEEDS_KATAGO.includes(key);

/** Territory cells to paint: contested territory from analysis when asked for, otherwise the settled one. */
export function territoryCells(
  settled: number[],
  live: number[] | undefined,
  show: { settled: boolean; live: boolean },
): number[] | undefined {
  if (show.live && live && live.length > 0) return live;
  if (show.settled) return settled;
  return undefined;
}

export interface GroupStyle {
  status: GroupStatus;
  strength: GroupStrength;
  /** Whether to tint the group by strength. */
  tint: boolean;
}
type GroupStatus = GroupAnalysis["status"];

/** Per-point group judgments, limited to the overlays that are switched on. */
export function groupStyles(
  groups: GroupAnalysis[],
  size: number,
  show: { lifeDeath: boolean; strength: boolean },
): Map<number, GroupStyle> {
  const out = new Map<number, GroupStyle>();
  if (!show.lifeDeath && !show.strength) return out;
  for (const g of groups) {
    const style: GroupStyle = {
      status: show.lifeDeath ? g.status : "alive",
      strength: g.strength,
      tint: show.strength,
    };
    for (const s of g.stones) out.set(indexOf(s, size), style);
  }
  return out;
}
