import { describe, expect, it } from "vitest";

import { parseRupiahInput } from "./rupiah";
import {
  groupRupiahDigits,
  MAX_RUPIAH_DIGITS,
  rupiahDigits,
  rupiahFieldText,
} from "./rupiah-input";

describe("rupiahDigits", () => {
  it.each([
    { name: "plain digits", input: "1250000", want: "1250000" },
    { name: "a formatted amount", input: "Rp 1.250.000", want: "1250000" },
    { name: "letters and signs", input: "-1a2,5b", want: "125" },
    { name: "leading zeros", input: "0001500", want: "1500" },
    { name: "only zeros", input: "000", want: "" },
    { name: "nothing", input: "", want: "" },
    {
      name: "more digits than a safe integer holds",
      input: "1".repeat(20),
      want: "1".repeat(MAX_RUPIAH_DIGITS),
    },
  ])("keeps $name as digits", ({ input, want }) => {
    expect(rupiahDigits(input)).toBe(want);
  });

  it("always gives an integer parseRupiahInput accepts", () => {
    const widest = rupiahDigits("9".repeat(30));
    expect(parseRupiahInput(widest)).toBe(Number(widest));
  });
});

describe("groupRupiahDigits", () => {
  it.each([
    { digits: "", want: "" },
    { digits: "5", want: "5" },
    { digits: "950", want: "950" },
    { digits: "1000", want: "1.000" },
    { digits: "1250000", want: "1.250.000" },
    { digits: "10000000", want: "10.000.000" },
  ])("groups $digits as $want", ({ digits, want }) => {
    expect(groupRupiahDigits(digits)).toBe(want);
  });
});

describe("rupiahFieldText", () => {
  it("turns typed text into the grouped digits the field shows", () => {
    expect(rupiahFieldText("1500000")).toBe("1.500.000");
    expect(rupiahFieldText("1.500.0000")).toBe("15.000.000");
    expect(rupiahFieldText("abc")).toBe("");
  });

  it("round-trips through the parser", () => {
    expect(parseRupiahInput(rupiahFieldText("1500000"))).toBe(1_500_000);
  });
});
