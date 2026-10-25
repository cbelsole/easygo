import { describe, expect, it } from "vite-plus/test";
import type { Color } from "../api/types";
import { addGhost, removeLastGhost } from "./ghosts";

const empty = (): Color[] => Array.from<Color>({ length: 81 }).fill(0);

describe("ghost stones", () => {
  it("alternate colors starting with the side to play", () => {
    let g = addGhost([], { x: 1, y: 1 }, empty(), 9, 2);
    g = addGhost(g, { x: 2, y: 2 }, empty(), 9, 2);
    g = addGhost(g, { x: 3, y: 3 }, empty(), 9, 2);
    expect(g.map((s) => s.color)).toEqual([2, 1, 2]);
  });

  it("ignores occupied and duplicate points", () => {
    const board = empty();
    board[10] = 1;
    expect(addGhost([], { x: 1, y: 1 }, board, 9, 1)).toEqual([]);
    const g = addGhost([], { x: 4, y: 4 }, board, 9, 1);
    expect(addGhost(g, { x: 4, y: 4 }, board, 9, 1)).toBe(g);
  });

  it("removes only the most recent", () => {
    let g = addGhost([], { x: 1, y: 1 }, empty(), 9, 1);
    g = addGhost(g, { x: 2, y: 2 }, empty(), 9, 1);
    expect(removeLastGhost(g)).toEqual([{ x: 1, y: 1, color: 1 }]);
    expect(removeLastGhost([])).toEqual([]);
  });
});
