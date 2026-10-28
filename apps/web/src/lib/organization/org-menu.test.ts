import { describe, expect, it } from "vitest";

import type { Capability } from "@/lib/api/generated/model";

import { canSee, ORG_MENU, orgPath, visibleOrgMenu } from "./org-menu";

function capabilities(
  entries: Record<string, Capability["access"]>,
): Capability[] {
  return Object.entries(entries).map(([action, access]) => ({
    action,
    access,
  }));
}

describe("visibleOrgMenu", () => {
  it("shows every item to a member allowed everything", () => {
    const all = capabilities({
      "property:read": "allow",
      "tenant:read": "allow",
      "member:read": "allow",
    });
    expect(visibleOrgMenu(all).map((item) => item.key)).toEqual([
      "home",
      "properties",
      "tenants",
      "members",
    ]);
  });

  it("shows an item allowed on some records", () => {
    const limited = capabilities({
      "property:read": "conditional",
      "tenant:read": "conditional",
      "member:read": "deny",
    });
    expect(visibleOrgMenu(limited).map((item) => item.key)).toEqual([
      "home",
      "properties",
      "tenants",
    ]);
  });

  it("shows only the home to a member with no matching action", () => {
    expect(visibleOrgMenu([]).map((item) => item.key)).toEqual(["home"]);
  });

  it("treats an action missing from the list as denied", () => {
    expect(
      canSee(capabilities({ "tenant:read": "allow" }), "member:read"),
    ).toBe(false);
  });

  it("names a catalog action for every gated item", () => {
    for (const item of ORG_MENU) {
      if (item.action !== null) expect(item.action).toMatch(/^[a-z]+:[a-z]+$/);
    }
  });
});

describe("orgPath", () => {
  it("builds the home and a section path", () => {
    expect(orgPath("org-1")).toBe("/o/org-1");
    expect(orgPath("org-1", "members")).toBe("/o/org-1/members");
  });

  it("encodes the id", () => {
    expect(orgPath("a/b?c")).toBe("/o/a%2Fb%3Fc");
  });
});
