import { describe, expect, it } from "vitest";
import type { GroupAnalysis } from "../api/types";
import { availability, groupStyles, territoryCells } from "./overlays";

const env = { mode: "live" as const, katago: true, josekiLoaded: true };

describe("availability", () => {
  it("disables KataGo features when KataGo is down, with a reason", () => {
    const a = availability("katago", { ...env, katago: false });
    expect(a.enabled).toBe(false);
    expect(a.reason).toMatch(/KataGo/);
  });

  it("uses the server's refusal message", () => {
    const a = availability("lifeDeath", { ...env, katagoBlocked: "AI help is off" });
    expect(a).toEqual({ enabled: false, reason: "AI help is off" });
  });

  it("greys out a feature the server refused", () => {
    const a = availability("shapes", { ...env, blocked: { shapes: "Both players must agree" } });
    expect(a).toEqual({ enabled: false, reason: "Both players must agree" });
  });

  it("follows the mode matrix", () => {
    expect(availability("katago", { ...env, mode: "joseki" }).enabled).toBe(false);
    expect(availability("annotations", env).enabled).toBe(false);
    expect(availability("annotations", { ...env, mode: "teaching" }).enabled).toBe(true);
    expect(availability("shapes", { ...env, mode: "joseki" }).enabled).toBe(true);
    expect(availability("joseki", { ...env, josekiLoaded: false }).enabled).toBe(false);
  });
});

describe("territoryCells", () => {
  const settled = [1, 0];
  const live = [2, 2];
  it("prefers live territory when switched on and present", () => {
    expect(territoryCells(settled, live, { settled: true, live: true })).toBe(live);
  });
  it("falls back to settled territory without analysis", () => {
    expect(territoryCells(settled, undefined, { settled: true, live: true })).toBe(settled);
  });
  it("returns nothing when both are off", () => {
    expect(territoryCells(settled, live, { settled: false, live: false })).toBeUndefined();
  });
});

describe("groupStyles", () => {
  const groups: GroupAnalysis[] = [
    { stones: [{ x: 0, y: 0 }], color: 1, status: "dead", strength: "weak", own: -0.9 },
    { stones: [{ x: 1, y: 0 }], color: 2, status: "alive", strength: "strong", own: -0.9 },
  ];
  it("is empty when both overlays are off", () => {
    expect(groupStyles(groups, 9, { lifeDeath: false, strength: false }).size).toBe(0);
  });
  it("hides the life and death verdict when only strength is shown", () => {
    const m = groupStyles(groups, 9, { lifeDeath: false, strength: true });
    expect(m.get(0)).toEqual({ status: "alive", strength: "weak", tint: true });
  });
  it("includes dead groups when life and death is on", () => {
    const m = groupStyles(groups, 9, { lifeDeath: true, strength: false });
    expect(m.get(0)?.status).toBe("dead");
  });
});
