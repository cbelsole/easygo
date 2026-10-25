import type { Point } from "../api/types";

const LETTERS = "ABCDEFGHJKLMNOPQRSTUVWXYZ";

/** Column letters skip I, as on every Go board. */
export function columnLabel(x: number): string {
  return LETTERS[x] ?? String(x + 1);
}

/** Rows count up from the bottom; y is measured from the top. */
export function rowLabel(y: number, size: number): string {
  return String(size - y);
}

export function pointLabel(p: Point, size: number): string {
  return `${columnLabel(p.x)}${rowLabel(p.y, size)}`;
}

export function starPoints(size: number): Point[] {
  const edge = size >= 13 ? 3 : size >= 9 ? 2 : -1;
  if (edge < 0) return [];
  const lo = edge;
  const hi = size - 1 - edge;
  const mid = (size - 1) / 2;
  if (size === 9) {
    return [lo, hi].flatMap((y) => [lo, hi].map((x) => ({ x, y }))).concat({ x: mid, y: mid });
  }
  const lines = [lo, mid, hi];
  return lines.flatMap((y) => lines.map((x) => ({ x, y })));
}

export const indexOf = (p: Point, size: number): number => p.y * size + p.x;

export const pointAt = (i: number, size: number): Point => ({
  x: i % size,
  y: Math.floor(i / size),
});

export const inBounds = (p: Point, size: number): boolean =>
  p.x >= 0 && p.y >= 0 && p.x < size && p.y < size;

export const samePoint = (a: Point, b: Point): boolean => a.x === b.x && a.y === b.y;
