import { describe, expect, it } from "vitest";
import type { Color, TreeNode } from "../api/types";
import { handicapPoints, libertyMap, replay, totalLiberties } from "./replay";

const mv = (id: number, color: 1 | 2, x: number, y: number): TreeNode => ({
  id,
  parent: id - 1,
  color,
  x,
  y,
  children: [],
});

describe("replay", () => {
  it("captures a stone with no liberties", () => {
    // White stone at (1,0) in the top edge is surrounded by black at (0,0), (2,0), (1,1).
    const pos = replay(9, 0, [
      mv(2, 1, 0, 0),
      mv(3, 2, 1, 0),
      mv(4, 1, 2, 0),
      mv(5, 2, 8, 8),
      mv(6, 1, 1, 1),
    ]);
    expect(pos.board[1]).toBe(0);
    expect(pos.captures).toEqual([1, 0]);
    expect(pos.toPlay).toBe(2);
    expect(pos.moveNumber).toBe(5);
  });

  it("starts white after a handicap and counts passes as moves", () => {
    const pos = replay(9, 2, [{ ...mv(2, 2, 0, 0), pass: true }]);
    expect(pos.board.filter((c) => c === 1)).toHaveLength(2);
    expect(pos.toPlay).toBe(1);
    expect(pos.lastMove).toBeUndefined();
  });
});

describe("libertyMap", () => {
  it("counts shared liberties for a connected group", () => {
    const board = new Array<Color>(81).fill(0);
    board[0] = 1;
    board[1] = 1;
    const libs = libertyMap(board, 9);
    expect(libs[0]).toBe(3);
    expect(libs[1]).toBe(3);
    expect(libs[2]).toBe(0);
  });
});

describe("handicapPoints", () => {
  it("gives the standard counts and includes the center for odd numbers", () => {
    expect(handicapPoints(19, 4)).toHaveLength(4);
    expect(handicapPoints(19, 5)).toContainEqual({ x: 9, y: 9 });
    expect(handicapPoints(19, 9)).toHaveLength(9);
    expect(handicapPoints(19, 0)).toHaveLength(0);
  });
});

describe("totalLiberties", () => {
  it("counts each group once", () => {
    const board = new Array<Color>(81).fill(0);
    board[0] = 1;
    board[1] = 1;
    board[80] = 1;
    expect(totalLiberties(board, 9, 1)).toBe(3 + 2);
    expect(totalLiberties(board, 9, 2)).toBe(0);
  });
});
