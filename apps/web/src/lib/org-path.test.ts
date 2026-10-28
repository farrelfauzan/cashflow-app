import { describe, expect, it } from "vitest";

import { orgPath } from "./org-path";

describe("orgPath", () => {
  it("builds the home and a section path", () => {
    expect(orgPath("org-1")).toBe("/o/org-1");
    expect(orgPath("org-1", "members")).toBe("/o/org-1/members");
  });

  it("encodes the id", () => {
    expect(orgPath("a/b?c")).toBe("/o/a%2Fb%3Fc");
  });
});
