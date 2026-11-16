import { describe, expect, it } from "vitest";

import type { Capability } from "@/lib/api/generated/model";
import { createAppAbility } from "@/lib/capabilities";

import {
  isExactSection,
  landingSegment,
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

// The built-in roles' capabilities as getOrgMe answers them (internal/authz/builtin.go), only the
// actions the menu reads.
const KARYAWAN = capabilities({
  "report:read": "deny",
  "property:read": "deny",
  "tenant:read": "conditional",
  "invoice:read": "conditional",
  "cash:record": "conditional",
  "visit:read": "conditional",
  "visit:record": "conditional",
  "cash:deposit": "conditional",
  "member:read": "deny",
});
const ADMIN = capabilities({
  "report:read": "allow",
  "property:read": "allow",
  "tenant:read": "allow",
  "invoice:read": "allow",
  "cash:record": "deny",
  "visit:read": "allow",
  "visit:record": "deny",
  "cash:deposit": "deny",
  "member:read": "allow",
});

describe("visibleOrgMenu", () => {
  it("shows every item to a member allowed everything, field work first", () => {
    const all = capabilities({
      "cash:record": "allow",
      "report:read": "allow",
      "property:read": "allow",
      "tenant:read": "allow",
      "invoice:read": "allow",
      "cash:deposit": "allow",
      "visit:record": "allow",
      "member:read": "allow",
    });
    expect(
      visibleOrgMenu(createAppAbility(all)).map((item) => item.key),
    ).toEqual([
      "field",
      "visits",
      "deposit",
      "home",
      "properties",
      "tenants",
      "billing",
      "members",
    ]);
  });

  it("shows an item allowed on some records", () => {
    const limited = capabilities({
      "report:read": "conditional",
      "property:read": "conditional",
      "tenant:read": "conditional",
      "invoice:read": "conditional",
      "member:read": "deny",
    });
    expect(
      visibleOrgMenu(createAppAbility(limited)).map((item) => item.key),
    ).toEqual(["home", "properties", "tenants", "billing"]);
  });

  it("shows Tagihan hari ini, Kunjungan hari ini and Setor tunai first, and no Beranda, to a karyawan", () => {
    expect(
      visibleOrgMenu(createAppAbility(KARYAWAN)).map((item) => item.label),
    ).toEqual([
      "Tagihan hari ini",
      "Kunjungan hari ini",
      "Setor tunai",
      "Penyewa",
      "Penagihan",
    ]);
  });

  it("shows Kunjungan hari ini by visit:record, not by visit:read", () => {
    const readOnly = capabilities({ "visit:read": "allow" });
    expect(visibleOrgMenu(createAppAbility(readOnly))).toEqual([]);
    const records = capabilities({ "visit:record": "conditional" });
    expect(
      visibleOrgMenu(createAppAbility(records)).map((item) => item.segment),
    ).toEqual(["field/visits"]);
  });

  it("keeps the admin's menu, which starts at Beranda", () => {
    expect(
      visibleOrgMenu(createAppAbility(ADMIN)).map((item) => item.label),
    ).toEqual(["Beranda", "Properti", "Penyewa", "Penagihan", "Anggota tim"]);
  });

  it("hides Setor tunai from a member denied cash:deposit", () => {
    const admin = capabilities({
      "report:read": "allow",
      "invoice:read": "allow",
      "cash:deposit": "deny",
    });
    expect(
      visibleOrgMenu(createAppAbility(admin)).map((item) => item.key),
    ).toEqual(["home", "billing"]);
  });

  it("shows Beranda only to a member who may read reports", () => {
    const noReports = capabilities({
      "report:read": "deny",
      "tenant:read": "allow",
    });
    expect(
      visibleOrgMenu(createAppAbility(noReports)).map((item) => item.key),
    ).toEqual(["tenants"]);
  });

  it("shows nothing to a member with no matching action", () => {
    expect(visibleOrgMenu(createAppAbility([]))).toEqual([]);
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
  it("gives the admin Beranda, Penyewa, Penagihan and Properti, and Anggota tim in Lainnya", () => {
    const visible = visibleOrgMenu(createAppAbility(ADMIN));
    expect(phoneTabs(visible).map((item) => item.label)).toEqual([
      "Beranda",
      "Penyewa",
      "Penagihan",
      "Properti",
    ]);
    expect(moreMenuSections(visible).map((item) => item.label)).toEqual([
      "Anggota tim",
    ]);
  });

  it("gives the karyawan Tagihan, Kunjungan, Setor and Penyewa, and Penagihan in Lainnya", () => {
    const visible = visibleOrgMenu(createAppAbility(KARYAWAN));
    expect(
      phoneTabs(visible).map((item) => item.tabLabel ?? item.label),
    ).toEqual(["Tagihan", "Kunjungan", "Setor", "Penyewa"]);
    expect(moreMenuSections(visible).map((item) => item.label)).toEqual([
      "Penagihan",
    ]);
  });

  it("takes four tabs at most and moves the rest to Lainnya in the side menu's order", () => {
    expect(phoneTabs(ORG_MENU).map((item) => item.key)).toEqual([
      "field",
      "visits",
      "deposit",
      "home",
    ]);
    expect(moreMenuSections(ORG_MENU).map((item) => item.key)).toEqual([
      "properties",
      "tenants",
      "billing",
      "members",
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

describe("landingSegment", () => {
  it("keeps a member who may read reports on Beranda", () => {
    expect(landingSegment(createAppAbility(ADMIN))).toBeNull();
  });

  it("sends a karyawan to Tagihan hari ini", () => {
    expect(landingSegment(createAppAbility(KARYAWAN))).toBe("field");
  });

  it("sends a member without reports or field work to their first section", () => {
    const tenantsOnly = capabilities({ "tenant:read": "conditional" });
    expect(landingSegment(createAppAbility(tenantsOnly))).toBe("tenants");
  });

  it("keeps a member who may open nothing on the home", () => {
    expect(landingSegment(createAppAbility([]))).toBeNull();
  });
});

describe("isExactSection", () => {
  it("matches the home and Tagihan hari ini on their own page only", () => {
    expect(
      ORG_MENU.filter((item) => isExactSection(item.key)).map(
        (item) => item.key,
      ),
    ).toEqual(["field", "home"]);
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
