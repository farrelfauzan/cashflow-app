import { describe, expect, it } from "vitest";

import { orgInitials } from "./utils";

describe("orgInitials", () => {
  it.each([
    { name: "Griya Sejahtera", want: "GS" },
    { name: "Kost Putri Anggrek", want: "KP" },
    { name: "  kos   melati ", want: "KM" },
    { name: "Anggrek", want: "AN" },
    { name: "", want: "" },
  ])("gives $want for '$name'", ({ name, want }) => {
    expect(orgInitials(name)).toBe(want);
  });
});
