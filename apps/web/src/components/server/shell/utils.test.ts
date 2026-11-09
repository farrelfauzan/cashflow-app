import { describe, expect, it } from "vitest";

import type { Capability } from "@/lib/api/generated/model";
import { createAppAbility } from "@/lib/capabilities";

import {
  moreMenuRoleLine,
  moreMenuSections,
  ORG_MENU,
  phoneTabs,
  roleSummary,
  visibleOrgMenu,
} from "./utils";

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
      "invoice:read": "allow",
      "member:read": "allow",
    });
    expect(
      visibleOrgMenu(createAppAbility(all)).map((item) => item.key),
    ).toEqual(["home", "properties", "tenants", "billing", "members"]);
  });

  it("shows an item allowed on some records", () => {
    const limited = capabilities({
      "property:read": "conditional",
      "tenant:read": "conditional",
      "invoice:read": "conditional",
      "member:read": "deny",
    });
    expect(
      visibleOrgMenu(createAppAbility(limited)).map((item) => item.key),
    ).toEqual(["home", "properties", "tenants", "billing"]);
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

describe("phoneTabs and moreMenuSections", () => {
  it("puts the whole menu in the tab bar's order, and Anggota tim in Lainnya", () => {
    expect(phoneTabs(ORG_MENU).map((item) => item.label)).toEqual([
      "Beranda",
      "Penyewa",
      "Penagihan",
      "Properti",
    ]);
    expect(moreMenuSections(ORG_MENU).map((item) => item.label)).toEqual([
      "Anggota tim",
    ]);
  });

  it("shows only the sections the member may open", () => {
    const visible = ORG_MENU.filter((item) =>
      ["home", "tenants", "members"].includes(item.key),
    );
    expect(phoneTabs(visible).map((item) => item.key)).toEqual([
      "home",
      "tenants",
    ]);
    expect(moreMenuSections(visible).map((item) => item.key)).toEqual([
      "members",
    ]);
  });
});

describe("moreMenuRoleLine", () => {
  it.each([
    { roles: "Admin", want: "Admin di Griya Sejahtera" },
    { roles: "Karyawan, Admin", want: "Karyawan, Admin di Griya Sejahtera" },
    { roles: "", want: "Griya Sejahtera" },
  ])("says $want", ({ roles, want }) => {
    expect(moreMenuRoleLine(roles, "Griya Sejahtera")).toBe(want);
  });
});
