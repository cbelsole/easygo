import type { Mark, MarkKind, Point } from "../api/types";
import { samePoint } from "./coords";

export type Tool = "letters" | "numbers" | "triangle" | "square" | "circle" | "cross";

const LETTERS = "ABCDEFGHIJKLMNOPQRSTUVWXYZ";

/** First unused label of the requested family on this node. */
export function nextLabel(marks: Mark[], family: "letters" | "numbers"): string {
  const used = new Set(marks.filter((m) => m.kind === "label").map((m) => m.text));
  if (family === "numbers") {
    let n = 1;
    while (used.has(String(n))) n++;
    return String(n);
  }
  for (let round = 0; ; round++) {
    for (const ch of LETTERS) {
      const label = ch.repeat(round + 1);
      if (!used.has(label)) return label;
    }
  }
}

const kindOf = (tool: Tool): MarkKind =>
  tool === "letters" || tool === "numbers" ? "label" : tool;

/**
 * Apply a tool click: clicking a mark of the same shape removes it, otherwise it replaces
 * whatever mark was at the point. Labels are numbered automatically.
 */
export function applyTool(marks: Mark[], p: Point, tool: Tool): Mark[] {
  const kind = kindOf(tool);
  const existing = marks.find((m) => samePoint(m, p));
  const rest = marks.filter((m) => !samePoint(m, p));
  if (existing && existing.kind === kind) return rest;
  if (kind === "label") {
    return [
      ...rest,
      { x: p.x, y: p.y, kind, text: nextLabel(marks, tool as "letters" | "numbers") },
    ];
  }
  return [...rest, { x: p.x, y: p.y, kind }];
}
