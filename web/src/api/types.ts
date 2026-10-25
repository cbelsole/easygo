/** Types mirroring docs/api.md. Colors: 0 empty, 1 black, 2 white. Points have a top-left origin. */

export type Color = 0 | 1 | 2;
export type Mode = "live" | "teaching" | "joseki";
export type Status = "waiting" | "playing" | "scoring" | "finished";
export type Opponent = "local" | "ai" | "open";
export type ColorChoice = "black" | "white" | "random";

export interface Point {
  x: number;
  y: number;
}

export interface User {
  id: string;
  name: string;
  email?: string;
  picture?: string;
  rank: string;
  location: string;
  provider: string;
}

export interface Rules {
  ko: "SIMPLE" | "POSITIONAL" | "SITUATIONAL";
  scoring: "AREA" | "TERRITORY";
  tax: "NONE" | "SEKI" | "ALL";
  suicide: boolean;
  hasButton: boolean;
  whiteHandicapBonus: "0" | "N-1" | "N";
  komi: number;
  preset?: string;
}

export interface KataGoConfig {
  binary: string;
  model: string;
  config: string;
  humanModel?: string;
}

export interface KataGoStatus {
  available: boolean;
  starting: boolean;
  version?: string;
  /** True when rank-based (human-style) opponents can be played. */
  humanAvailable: boolean;
  error?: string;
  config: KataGoConfig;
}

export interface JosekiInfo {
  loaded: boolean;
  positions: number;
  source: string;
  error?: string;
}

export interface AppConfig {
  presets: { name: string; rules: Rules }[];
  sizes: number[];
  /** Ranks the human-style opponent can imitate, weakest first (20k ... 9d). */
  ranks: string[];
  katago: KataGoStatus;
  joseki: JosekiInfo;
  googleAuth: boolean;
  devLogin: boolean;
}

export interface ClockConfig {
  enabled: boolean;
  mainSeconds: number;
  periods: number;
  periodSeconds: number;
}

export interface ClockView {
  enabled: boolean;
  mainMs: number;
  periodMs: number;
  periods: number;
  inByo: boolean;
  flagged: boolean;
  running: boolean;
  byoConfigured: boolean;
  config: ClockConfig;
}

export type MarkKind = "label" | "triangle" | "square" | "circle" | "cross";

export interface Mark extends Point {
  kind: MarkKind;
  text?: string;
}

export type Judgment = "" | "correct" | "playable" | "mistake";

export interface TreeNode {
  id: number;
  parent: number;
  color: Color;
  pass?: boolean;
  x: number;
  y: number;
  comment?: string;
  name?: string;
  judgment?: Judgment;
  marks?: Mark[];
  children: number[];
}

export interface PlayerView {
  name: string;
  rank: string;
  location: string;
  kind: "human" | "ai" | "open";
  you: boolean;
}

export interface YouView {
  seat: 0 | 1 | 2 | 3;
  canMove: boolean;
  owner: boolean;
}

export interface ScoreView {
  black: number;
  white: number;
  margin: number;
  blackTerritory: number;
  whiteTerritory: number;
}

export interface GameView {
  id: string;
  name: string;
  mode: Mode;
  status: Status;
  result?: string;
  size: number;
  rules: Rules;
  handicap: number;
  version: number;
  toPlay: 1 | 2;
  moveNumber: number;
  cursor: number;
  board: Color[];
  liberties: number[];
  captures: [number, number];
  lastMove?: Point;
  nodes: TreeNode[];
  mainLine: number[];
  partial: boolean;
  players: [PlayerView, PlayerView];
  clock: [ClockView, ClockView];
  you: YouView;
  territory: number[];
  dead: Point[];
  score?: ScoreView;
  accepted: [boolean, boolean];
  aiAssist: boolean;
}

export interface GameSummary {
  id: string;
  name: string;
  mode: Mode;
  status: Status;
  result: string;
  updated: number;
  black: string;
  white: string;
  size: number;
  owner: string;
}

export interface GameList {
  mine: GameSummary[];
  open: GameSummary[];
}

export interface NewGameRequest {
  name: string;
  mode: Mode;
  size: number;
  rules: string | Rules;
  komi?: number;
  handicap: number;
  opponent: Opponent;
  color: ColorChoice;
  /** Full-strength opponent: search visits. Ignored when aiRank is set. */
  aiVisits: number;
  /** Human-style opponent, e.g. "16k". */
  aiRank?: string;
  clock: ClockConfig;
}

export type GroupStatus = "alive" | "dead" | "seki" | "unsettled";
export type GroupStrength = "strong" | "moderate" | "weak";

export interface Suggestion extends Point {
  pass?: boolean;
  winrate: number;
  scoreLead: number;
  visits: number;
  prior: number;
  pv: Point[];
}

export interface GroupAnalysis {
  stones: Point[];
  color: Color;
  status: GroupStatus;
  strength: GroupStrength;
  own: number;
}

export interface Analysis {
  available: boolean;
  toPlay: number;
  winrate: number;
  scoreLead: number;
  visits: number;
  suggestions: Suggestion[];
  ownership: number[];
  territory: number[];
  groups: GroupAnalysis[];
}

export type ShapeKind = "ladder" | "net" | "snapback" | "knight" | "large-knight";

export interface ShapeFinding {
  kind: ShapeKind;
  result?: "works" | "broken" | "unknown";
  color: Color;
  move?: Point;
  points: Point[];
  breaker?: Point;
  confidence: "exact" | "bounded";
  text: string;
}

export interface JosekiNext {
  color: Color;
  x: number;
  y: number;
  comment?: string;
}

export interface JosekiCorner {
  corner: "tl" | "tr" | "bl" | "br";
  status: "joseki" | "deviated";
  stones: number;
  comment?: string;
  next: JosekiNext[];
  deviatedAt?: number;
  swapped?: boolean;
}

export interface JosekiReport {
  loaded: boolean;
  corners: JosekiCorner[];
}

export interface AnnotateRequest {
  comment?: string;
  name?: string;
  judgment?: Judgment;
  marks?: Mark[];
}
