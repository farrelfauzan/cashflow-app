import { describe, expect, it } from "vitest";

import { canSee } from "./capabilities";

describe("canSee", () => {
  it("treats an action missing from the list as denied", () => {
    expect(
      canSee([{ action: "tenant:read", access: "allow" }], "member:read"),
    ).toBe(false);
  });
});
