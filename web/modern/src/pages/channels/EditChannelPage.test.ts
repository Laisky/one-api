import { describe, expect, it } from "vitest";

import { normalizeChannelType } from "./helpers";

describe("normalizeChannelType", () => {
  it("returns numbers as-is when finite", () => {
    expect(normalizeChannelType(14)).toBe(14);
    expect(normalizeChannelType(0)).toBe(0);
  });

  it("parses numeric strings", () => {
    expect(normalizeChannelType("33")).toBe(33);
    expect(normalizeChannelType(" 51 ")).toBe(51);
  });

  it("treats blank values as null", () => {
    expect(normalizeChannelType("")).toBeNull();
    expect(normalizeChannelType("   ")).toBeNull();
    expect(normalizeChannelType(null)).toBeNull();
    expect(normalizeChannelType(undefined)).toBeNull();
  });

  it("filters out non-finite values", () => {
    expect(normalizeChannelType(Number.NaN)).toBeNull();
    expect(normalizeChannelType("NaN")).toBeNull();
    expect(normalizeChannelType(Infinity)).toBeNull();
  });
});
