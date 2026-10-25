import { describe, expect, it } from "vitest";
import type { Mark } from "../api/types";
import { applyTool, nextLabel } from "./marks";

describe("nextLabel", () => {
  it("fills gaps in numbers and letters", () => {
    const marks: Mark[] = [
      { x: 0, y: 0, kind: "label", text: "1" },
      { x: 1, y: 0, kind: "label", text: "3" },
      { x: 2, y: 0, kind: "label", text: "A" },
    ];
    expect(nextLabel(marks, "numbers")).toBe("2");
    expect(nextLabel(marks, "letters")).toBe("B");
  });

  it("continues past Z", () => {
    const marks: Mark[] = [...("ABCDEFGHIJKLMNOPQRSTUVWXYZ")].map((t, i) => ({
      x: i % 9,
      y: 0,
      kind: "label" as const,
      text: t,
    }));
    expect(nextLabel(marks, "letters")).toBe("AA");
  });
});

describe("applyTool", () => {
  it("adds a numbered label, then removes it on a second click", () => {
    const one = applyTool([], { x: 2, y: 2 }, "numbers");
    expect(one).toEqual([{ x: 2, y: 2, kind: "label", text: "1" }]);
    expect(applyTool(one, { x: 2, y: 2 }, "numbers")).toEqual([]);
  });

  it("replaces a different mark at the same point", () => {
    const tri = applyTool([], { x: 1, y: 1 }, "triangle");
    expect(applyTool(tri, { x: 1, y: 1 }, "circle")).toEqual([{ x: 1, y: 1, kind: "circle" }]);
  });
});
