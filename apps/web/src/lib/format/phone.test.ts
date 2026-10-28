import { describe, expect, it } from "vitest";

import { formatPhone } from "./phone";

describe("formatPhone", () => {
  it.each([
    {
      name: "a 12-digit mobile",
      e164: "+6281200090001",
      want: "0812-0009-0001",
    },
    {
      name: "an 11-digit mobile",
      e164: "+628123456789",
      want: "0812-3456-789",
    },
    {
      name: "a 13-digit mobile",
      e164: "+6281234567890",
      want: "0812-3456-7890",
    },
  ])("formats $name", ({ e164, want }) => {
    expect(formatPhone(e164)).toBe(want);
  });

  it.each([
    { name: "another country", value: "+6591234567" },
    { name: "a number without a plus", value: "081200090001" },
    { name: "letters", value: "+62abc" },
    { name: "an empty string", value: "" },
  ])("returns $name unchanged", ({ value }) => {
    expect(formatPhone(value)).toBe(value);
  });
});
