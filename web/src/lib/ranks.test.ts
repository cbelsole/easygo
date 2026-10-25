import { describe, expect, it } from "vite-plus/test";
import { DEFAULT_RANK, pickRank, spellRank } from "./ranks";

const ranks = ["20k", "16k", "1k", "1d", "9d"];

describe("pickRank", () => {
  it("keeps the remembered rank when it is still offered", () => {
    expect(pickRank(ranks, "1d")).toBe("1d");
  });
  it("falls back to the default for a missing or unknown rank", () => {
    expect(pickRank(ranks, null)).toBe(DEFAULT_RANK);
    expect(pickRank(ranks, "12k")).toBe(DEFAULT_RANK);
    expect(pickRank(ranks, "nonsense")).toBe(DEFAULT_RANK);
  });
  it("uses the first rank when even the default is not offered", () => {
    expect(pickRank(["5d", "6d"], null)).toBe("5d");
  });
});

describe("spellRank", () => {
  it("spells kyu and dan", () => {
    expect(spellRank("16k")).toBe("16 kyu");
    expect(spellRank("3d")).toBe("3 dan");
  });
  it("leaves anything else alone", () => {
    expect(spellRank("100 visits")).toBe("100 visits");
  });
});
