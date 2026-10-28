import { describe, expect, it } from "vitest";

import { isCompleteLoginCode, loginCodeDigits } from "./login-code";

describe("loginCodeDigits", () => {
  it.each([
    { name: "plain digits", input: "123456", want: "123456" },
    { name: "a partial code", input: "123", want: "123" },
    { name: "empty", input: "", want: "" },
    { name: "a code with a space", input: "123 456", want: "123456" },
    { name: "a code with a dash", input: "123-456", want: "123456" },
    { name: "letters", input: "12a3b4", want: "1234" },
    { name: "more than 6 digits", input: "12345678", want: "123456" },
    {
      name: "a pasted message",
      input: "Kode masuk Anda: 654321.",
      want: "654321",
    },
  ])("keeps the digits of $name", ({ input, want }) => {
    expect(loginCodeDigits(input)).toBe(want);
  });
});

describe("isCompleteLoginCode", () => {
  it.each([
    { name: "6 digits", input: "000123", want: true },
    { name: "5 digits", input: "12345", want: false },
    { name: "7 digits", input: "1234567", want: false },
    { name: "empty", input: "", want: false },
    { name: "a letter", input: "12345a", want: false },
    { name: "a space", input: "123 45", want: false },
  ])("$name is $want", ({ input, want }) => {
    expect(isCompleteLoginCode(input)).toBe(want);
  });
});
