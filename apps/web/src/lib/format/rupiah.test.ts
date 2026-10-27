import { describe, expect, it } from "vitest";

import { formatRupiah, parseRupiahInput } from "./rupiah";

describe("formatRupiah", () => {
  it.each([
    { name: "a typical rent", amount: 1_250_000, want: "Rp1.250.000" },
    { name: "zero", amount: 0, want: "Rp0" },
    { name: "negative zero", amount: -0, want: "Rp0" },
    { name: "below a thousand", amount: 950, want: "Rp950" },
    { name: "exactly a thousand", amount: 1_000, want: "Rp1.000" },
    { name: "the QRIS ceiling", amount: 10_000_000, want: "Rp10.000.000" },
    { name: "a negative amount", amount: -1_250_000, want: "-Rp1.250.000" },
    {
      name: "the largest safe integer",
      amount: Number.MAX_SAFE_INTEGER,
      want: "Rp9.007.199.254.740.991",
    },
  ])("formats $name", ({ amount, want }) => {
    expect(formatRupiah(amount)).toBe(want);
  });

  it("has no space or non-breaking space after Rp", () => {
    expect(formatRupiah(1_250_000)).not.toMatch(/\s/);
  });

  it.each([
    { name: "a fraction", amount: 1250.5 },
    { name: "NaN", amount: Number.NaN },
    { name: "Infinity", amount: Number.POSITIVE_INFINITY },
    { name: "an unsafe integer", amount: Number.MAX_SAFE_INTEGER + 1 },
  ])("throws on $name", ({ amount }) => {
    expect(() => formatRupiah(amount)).toThrow(RangeError);
  });
});

describe("parseRupiahInput", () => {
  it.each([
    { name: "dotted thousands", input: "1.250.000", want: 1_250_000 },
    { name: "plain digits", input: "1250000", want: 1_250_000 },
    {
      name: "a pasted formatted amount",
      input: "Rp 1.250.000",
      want: 1_250_000,
    },
    { name: "zero", input: "0", want: 0 },
    { name: "leading zeros", input: "007", want: 7 },
    {
      name: "the largest safe integer",
      input: "9007199254740991",
      want: Number.MAX_SAFE_INTEGER,
    },
  ])("reads $name", ({ input, want }) => {
    expect(parseRupiahInput(input)).toBe(want);
  });

  it.each([
    { name: "an empty string", input: "" },
    { name: "only spaces", input: "   " },
    { name: "no digits", input: "Rp" },
    { name: "a decimal comma", input: "1.250.000,00" },
    {
      name: "a value beyond the safe integer range",
      input: "9007199254740992",
    },
    { name: "a very long number", input: "9".repeat(400) },
  ])("returns null for $name", ({ input }) => {
    expect(parseRupiahInput(input)).toBeNull();
  });
});
