import type { TreeNode } from "../api/types";

export type NodeMap = Map<number, TreeNode>;

export const nodeMap = (nodes: TreeNode[]): NodeMap => new Map(nodes.map((n) => [n.id, n]));

/** The node whose parent is not in the map: the game root, or the top of a partial window. */
export function rootId(map: NodeMap): number | undefined {
  for (const n of map.values()) {
    if (n.parent === 0 || !map.has(n.parent)) return n.id;
  }
  return undefined;
}

export const loadedChildren = (map: NodeMap, n: TreeNode): number[] =>
  n.children.filter((c) => map.has(c));

/** True when some children exist on the server but are not in the window we were sent. */
export const hasUnloadedChildren = (map: NodeMap, n: TreeNode): boolean =>
  n.children.some((c) => !map.has(c));

/**
 * A run of moves with no fork, then the branches that follow it.
 * branches[0] is the main continuation; the rest are variations.
 */
export interface Segment {
  ids: number[];
  branches: Segment[];
}

export function buildSegments(map: NodeMap, startId: number): Segment {
  const ids: number[] = [];
  let cur = map.get(startId);
  while (cur) {
    ids.push(cur.id);
    const kids = loadedChildren(map, cur);
    if (kids.length === 1) {
      cur = map.get(kids[0]!);
      continue;
    }
    return { ids, branches: kids.map((k) => buildSegments(map, k)) };
  }
  return { ids, branches: [] };
}

/** Node ids from the root down to `id`. */
export function pathTo(map: NodeMap, id: number): number[] {
  const path: number[] = [];
  let cur = map.get(id);
  while (cur) {
    path.push(cur.id);
    cur = map.get(cur.parent);
  }
  return path.reverse();
}

export type NavKey = "parent" | "child" | "prevSibling" | "nextSibling";

/** Target of a keyboard step from `id`; undefined when there is nowhere to go. */
export function step(map: NodeMap, id: number, key: NavKey): number | undefined {
  const n = map.get(id);
  if (!n) return undefined;
  if (key === "parent") return map.has(n.parent) ? n.parent : undefined;
  if (key === "child") return loadedChildren(map, n)[0];
  const parent = map.get(n.parent);
  if (!parent) return undefined;
  const sibs = loadedChildren(map, parent);
  const i = sibs.indexOf(id);
  return sibs[key === "prevSibling" ? i - 1 : i + 1];
}

/** Ids that follow `id` along first children, inclusive. */
export function mainLineFrom(map: NodeMap, id: number): number[] {
  const out: number[] = [];
  let cur = map.get(id);
  while (cur) {
    out.push(cur.id);
    cur = map.get(loadedChildren(map, cur)[0] ?? -1);
  }
  return out;
}

/** Whether `id` is on the main line (every ancestor is a first child). */
export function onMainLine(map: NodeMap, id: number): boolean {
  let cur = map.get(id);
  while (cur) {
    const parent = map.get(cur.parent);
    if (!parent) return true;
    if (parent.children[0] !== cur.id) return false;
    cur = parent;
  }
  return true;
}
