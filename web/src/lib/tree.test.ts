import { describe, expect, it } from "vitest";
import type { TreeNode } from "../api/types";
import {
  buildSegments,
  hasUnloadedChildren,
  nodeMap,
  onMainLine,
  pathTo,
  rootId,
  step,
} from "./tree";

const n = (id: number, parent: number, children: number[]): TreeNode => ({
  id,
  parent,
  color: id === 1 ? 0 : (((id % 2) + 1) as 1 | 2),
  x: 0,
  y: 0,
  children,
});

// 1 - 2 - 3 - 4
//         \ 5 - 6
const nodes = [n(1, 0, [2]), n(2, 1, [3]), n(3, 2, [4, 5]), n(4, 3, []), n(5, 3, [6]), n(6, 5, [])];

describe("buildSegments", () => {
  it("keeps unforked runs together and branches at forks", () => {
    const map = nodeMap(nodes);
    const seg = buildSegments(map, rootId(map)!);
    expect(seg.ids).toEqual([1, 2, 3]);
    expect(seg.branches.map((b) => b.ids)).toEqual([[4], [5, 6]]);
  });

  it("treats children missing from a partial window as not loaded", () => {
    const map = nodeMap([n(1, 0, [2, 9]), n(2, 1, [])]);
    const seg = buildSegments(map, 1);
    expect(seg.ids).toEqual([1, 2]);
    expect(hasUnloadedChildren(map, map.get(1)!)).toBe(true);
  });
});

describe("navigation", () => {
  const map = nodeMap(nodes);

  it("steps to the first child and the parent", () => {
    expect(step(map, 3, "child")).toBe(4);
    expect(step(map, 3, "parent")).toBe(2);
    expect(step(map, 1, "parent")).toBeUndefined();
  });

  it("switches between sibling variations", () => {
    expect(step(map, 4, "nextSibling")).toBe(5);
    expect(step(map, 5, "prevSibling")).toBe(4);
    expect(step(map, 5, "nextSibling")).toBeUndefined();
  });

  it("finds paths and main-line membership", () => {
    expect(pathTo(map, 6)).toEqual([1, 2, 3, 5, 6]);
    expect(onMainLine(map, 4)).toBe(true);
    expect(onMainLine(map, 6)).toBe(false);
  });

  it("uses the top of a window as root when the real root is not loaded", () => {
    const m = nodeMap([n(40, 39, [41]), n(41, 40, [])]);
    expect(rootId(m)).toBe(40);
  });
});
