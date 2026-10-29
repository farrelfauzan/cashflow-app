import { describe, expect, it } from "vitest";

import { parseRupiahInput } from "./rupiah";
import {
  caretAfterDigits,
  groupRupiahDigits,
  MAX_RUPIAH_DIGITS,
  rupiahDigits,
  rupiahDigitsBefore,
  rupiahFieldText,
} from "./rupiah-input";

describe("rupiahDigits", () => {
  it.each([
    { name: "plain digits", input: "1250000", want: "1250000" },
    { name: "a formatted amount", input: "Rp 1.250.000", want: "1250000" },
    { name: "letters and signs", input: "-1a2,5b", want: "125" },
    { name: "leading zeros", input: "0001500", want: "1500" },
    { name: "only zeros", input: "000", want: "0" },
    { name: "one zero", input: "0", want: "0" },
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

describe("rupiahDigitsBefore", () => {
  it.each([
    { name: "caret at the start", input: "1.250.000", caret: 0, want: 0 },
    { name: "caret after a dot", input: "1.250.000", caret: 2, want: 1 },
    { name: "caret in a group", input: "1.2350.000", caret: 4, want: 3 },
    { name: "caret at the end", input: "1.250.000", caret: 9, want: 7 },
    {
      name: "a letter before the caret",
      input: "1.25a0.000",
      caret: 5,
      want: 3,
    },
    {
      name: "a dot left after a Backspace",
      input: ".250.000",
      caret: 0,
      want: 0,
    },
    { name: "a leading zero typed", input: "01.250", caret: 1, want: 0 },
    { name: "only zeros typed", input: "000", caret: 3, want: 1 },
  ])("$name", ({ input, caret, want }) => {
    expect(rupiahDigitsBefore(input, caret)).toBe(want);
  });
});

describe("caretAfterDigits", () => {
  it.each([
    { name: "no digits", text: "1.250.000", count: 0, want: 0 },
    { name: "after the first digit", text: "1.250.000", count: 1, want: 1 },
    {
      name: "after the second digit, past the dot",
      text: "1.250.000",
      count: 2,
      want: 3,
    },
    { name: "after the last digit", text: "1.250.000", count: 7, want: 9 },
    { name: "more digits than the text has", text: "250", count: 5, want: 3 },
    { name: "an empty field", text: "", count: 0, want: 0 },
  ])("$name", ({ text, count, want }) => {
    expect(caretAfterDigits(text, count)).toBe(want);
  });
});
