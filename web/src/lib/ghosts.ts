import type { Color, Point } from "../api/types";
import { samePoint } from "./coords";

export interface Ghost extends Point {
  color: 1 | 2;
}

const other = (c: 1 | 2): 1 | 2 => (c === 1 ? 2 : 1);

/** Add a shadow stone; colors alternate starting with the side to play. Occupied points are ignored. */
export function addGhost(
  ghosts: Ghost[],
  p: Point,
  board: Color[],
  size: number,
  toPlay: 1 | 2,
): Ghost[] {
  if (board[p.y * size + p.x] !== 0) return ghosts;
  if (ghosts.some((g) => samePoint(g, p))) return ghosts;
  const last = ghosts[ghosts.length - 1];
  const color = last ? other(last.color) : toPlay;
  return [...ghosts, { ...p, color }];
}

export const removeLastGhost = (ghosts: Ghost[]): Ghost[] => ghosts.slice(0, -1);
