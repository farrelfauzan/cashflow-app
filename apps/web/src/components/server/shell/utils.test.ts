import { describe, expect, it } from "vitest";

import type { Capability } from "@/lib/api/generated/model";
import { createAppAbility } from "@/lib/capabilities";

import { ORG_MENU, roleSummary, visibleOrgMenu } from "./utils";

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
    expect(
      visibleOrgMenu(createAppAbility(all)).map((item) => item.key),
    ).toEqual(["home", "properties", "tenants", "members"]);
  });

  it("shows an item allowed on some records", () => {
    const limited = capabilities({
      "property:read": "conditional",
      "tenant:read": "conditional",
      "member:read": "deny",
    });
    expect(
      visibleOrgMenu(createAppAbility(limited)).map((item) => item.key),
    ).toEqual(["home", "properties", "tenants"]);
  });

  it("shows only the home to a member with no matching action", () => {
    expect(
      visibleOrgMenu(createAppAbility([])).map((item) => item.key),
    ).toEqual(["home"]);
  });

  it("names a catalog action for every gated item", () => {
    for (const item of ORG_MENU) {
      if (item.permission !== null) {
        expect(item.permission.join(" ")).toMatch(/^[a-z]+ [a-z]+$/);
      }
    }
  });
});

describe("roleSummary", () => {
  it("joins role names in the API's order", () => {
    expect(
      roleSummary([
        { key: "karyawan", name: "Karyawan", builtin: true },
        { key: "admin", name: "Admin", builtin: true, propertyId: "p-1" },
      ]),
    ).toBe("Karyawan, Admin");
  });

  it("shows a role held for several properties once", () => {
    expect(
      roleSummary([
        { key: "admin", name: "Admin", builtin: true, propertyId: "p-1" },
        { key: "admin", name: "Admin", builtin: true, propertyId: "p-2" },
      ]),
    ).toBe("Admin");
  });

  it("is empty without roles", () => {
    expect(roleSummary([])).toBe("");
  });
});
