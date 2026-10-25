import type { Color, Point, TreeNode } from "../api/types";

export interface Position {
  board: Color[];
  liberties: number[];
  captures: [number, number];
  lastMove?: Point;
  toPlay: 1 | 2;
  moveNumber: number;
}

/** Mirrors the server's standard handicap layout (2-9 stones). */
export function handicapPoints(size: number, n: number): Point[] {
  if (n < 2 || n > 9) return [];
  const edge = size === 9 ? 2 : 3;
  const lo = edge;
  const hi = size - 1 - edge;
  const mid = Math.floor(size / 2);
  const corners = [
    { x: hi, y: lo },
    { x: lo, y: hi },
    { x: hi, y: hi },
    { x: lo, y: lo },
  ];
  const sides = [
    { x: lo, y: mid },
    { x: hi, y: mid },
    { x: mid, y: lo },
    { x: mid, y: hi },
  ];
  const center = { x: mid, y: mid };
  if (n <= 4) return corners.slice(0, n);
  if (n === 5) return [...corners, center];
  if (n === 6) return [...corners, sides[0]!, sides[1]!];
  if (n === 7) return [...corners, sides[0]!, sides[1]!, center];
  if (n === 8) return [...corners, ...sides];
  return [...corners, ...sides, center];
}

const neighbors = (i: number, size: number): number[] => {
  const x = i % size;
  const out: number[] = [];
  if (x > 0) out.push(i - 1);
  if (x < size - 1) out.push(i + 1);
  if (i >= size) out.push(i - size);
  if (i < size * (size - 1)) out.push(i + size);
  return out;
};

/** Stones of the group containing `start`, plus its liberty points. */
function groupAt(board: Color[], size: number, start: number) {
  const color = board[start]!;
  const stones = [start];
  const seen = new Set(stones);
  const libs = new Set<number>();
  for (let k = 0; k < stones.length; k++) {
    for (const nb of neighbors(stones[k]!, size)) {
      const c = board[nb]!;
      if (c === 0) libs.add(nb);
      else if (c === color && !seen.has(nb)) {
        seen.add(nb);
        stones.push(nb);
      }
    }
  }
  return { stones, libs };
}

export function libertyMap(board: Color[], size: number): number[] {
  const out = new Array<number>(board.length).fill(0);
  const done = new Set<number>();
  board.forEach((c, i) => {
    if (c === 0 || done.has(i)) return;
    const { stones, libs } = groupAt(board, size, i);
    for (const s of stones) {
      done.add(s);
      out[s] = libs.size;
    }
  });
  return out;
}

/** Replay moves (root excluded) onto an empty or handicap board. Captures only; legality is the server's job. */
export function replay(size: number, handicap: number, moves: TreeNode[]): Position {
  const board = new Array<Color>(size * size).fill(0);
  for (const p of handicapPoints(size, handicap)) board[p.y * size + p.x] = 1;
  const captures: [number, number] = [0, 0];
  let lastMove: Point | undefined;
  let toPlay: 1 | 2 = handicap >= 2 ? 2 : 1;
  let moveNumber = 0;

  for (const m of moves) {
    if (m.color === 0) continue;
    moveNumber++;
    toPlay = m.color === 1 ? 2 : 1;
    if (m.pass) {
      lastMove = undefined;
      continue;
    }
    const at = m.y * size + m.x;
    board[at] = m.color;
    lastMove = { x: m.x, y: m.y };
    const enemy = m.color === 1 ? 2 : 1;
    for (const nb of neighbors(at, size)) {
      if (board[nb] !== enemy) continue;
      const g = groupAt(board, size, nb);
      if (g.libs.size > 0) continue;
      for (const s of g.stones) board[s] = 0;
      captures[m.color - 1] += g.stones.length;
    }
    const own = groupAt(board, size, at);
    if (own.libs.size === 0) for (const s of own.stones) board[s] = 0;
  }
  return { board, liberties: libertyMap(board, size), captures, lastMove, toPlay, moveNumber };
}

/** Sum of liberties over distinct groups of one color (shared liberties count once per group). */
export function totalLiberties(board: Color[], size: number, color: 1 | 2): number {
  const done = new Set<number>();
  let total = 0;
  board.forEach((c, i) => {
    if (c !== color || done.has(i)) return;
    const { stones, libs } = groupAt(board, size, i);
    stones.forEach((s) => done.add(s));
    total += libs.size;
  });
  return total;
}
