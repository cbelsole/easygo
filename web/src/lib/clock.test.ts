import { describe, expect, it } from "vitest";
import type { ClockView } from "../api/types";
import { formatClock, isLow, readClock } from "./clock";

function clock(over: Partial<ClockView> = {}): ClockView {
  return {
    enabled: true,
    mainMs: 60_000,
    periodMs: 0,
    periods: 3,
    inByo: false,
    flagged: false,
    running: true,
    byoConfigured: true,
    config: { enabled: true, mainSeconds: 60, periods: 3, periodSeconds: 30 },
    ...over,
  };
}

describe("readClock", () => {
  it("counts main time down only while running", () => {
    expect(readClock(clock(), 5_000).ms).toBe(55_000);
    expect(readClock(clock({ running: false }), 5_000).ms).toBe(60_000);
  });

  it("rolls from main time into a fresh first period", () => {
    const r = readClock(clock(), 70_000);
    expect(r).toMatchObject({ inByo: true, periods: 3, ms: 20_000, flagged: false });
  });

  it("consumes periods that expire in byo-yomi", () => {
    const c = clock({ mainMs: 0, inByo: true, periodMs: 10_000, periods: 3 });
    expect(readClock(c, 5_000)).toMatchObject({ periods: 3, ms: 5_000 });
    expect(readClock(c, 15_000)).toMatchObject({ periods: 2, ms: 25_000 });
  });

  it("flags when the last period runs out", () => {
    const c = clock({ mainMs: 0, inByo: true, periodMs: 10_000, periods: 1 });
    expect(readClock(c, 10_000).flagged).toBe(true);
  });

  it("flags when main time ends and there is no byo-yomi", () => {
    const c = clock({
      byoConfigured: false,
      periods: 0,
      config: { enabled: true, mainSeconds: 60, periods: 0, periodSeconds: 0 },
    });
    expect(readClock(c, 61_000).flagged).toBe(true);
  });
});

describe("formatClock", () => {
  it("formats main time as m:ss", () => {
    expect(formatClock({ inByo: false, ms: 599_999 })).toBe("10:00");
    expect(formatClock({ inByo: false, ms: 65_000 })).toBe("1:05");
    expect(formatClock({ inByo: false, ms: 3_725_000 })).toBe("1:02:05");
  });

  it("formats byo-yomi as seconds", () => {
    expect(formatClock({ inByo: true, ms: 24_200 })).toBe("25");
  });
});

describe("isLow", () => {
  it("is stricter in main time than in byo-yomi", () => {
    expect(isLow({ inByo: true, ms: 9_000, periods: 1, flagged: false })).toBe(true);
    expect(isLow({ inByo: false, ms: 25_000, periods: 1, flagged: false })).toBe(true);
    expect(isLow({ inByo: false, ms: 120_000, periods: 1, flagged: false })).toBe(false);
  });
});
