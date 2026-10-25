import { describe, expect, it } from "vite-plus/test";
import { columnLabel, pointLabel, rowLabel, starPoints } from "./coords";

describe("labels", () => {
  it("skips the letter I", () => {
    const cols = Array.from({ length: 19 }, (_, i) => columnLabel(i)).join("");
    expect(cols).toBe("ABCDEFGHJKLMNOPQRST");
  });

  it("numbers rows from the bottom", () => {
    expect(rowLabel(0, 19)).toBe("19");
    expect(rowLabel(18, 19)).toBe("1");
    expect(pointLabel({ x: 3, y: 15 }, 19)).toBe("D4");
    expect(pointLabel({ x: 8, y: 0 }, 19)).toBe("J19");
  });
});

describe("starPoints", () => {
  it("19x19 has nine points on the 4-4 lines", () => {
    const pts = starPoints(19);
    expect(pts).toHaveLength(9);
    expect(pts).toContainEqual({ x: 3, y: 3 });
    expect(pts).toContainEqual({ x: 9, y: 9 });
    expect(pts).toContainEqual({ x: 15, y: 9 });
  });

  it("13x13 has nine points on the 4-4 lines", () => {
    const pts = starPoints(13);
    expect(pts).toHaveLength(9);
    expect(pts).toContainEqual({ x: 3, y: 3 });
    expect(pts).toContainEqual({ x: 6, y: 6 });
    expect(pts).toContainEqual({ x: 9, y: 3 });
  });

  it("9x9 has four corners at 3-3 and the center", () => {
    const pts = starPoints(9);
    expect(pts).toHaveLength(5);
    expect(pts).toContainEqual({ x: 2, y: 2 });
    expect(pts).toContainEqual({ x: 6, y: 6 });
    expect(pts).toContainEqual({ x: 4, y: 4 });
  });
});
