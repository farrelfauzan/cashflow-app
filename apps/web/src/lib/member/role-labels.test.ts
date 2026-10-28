import { describe, expect, it } from "vitest";

import type { RoleRef } from "@/lib/api/generated/model";

import { describeRoles, roleChips, UNKNOWN_PROPERTY } from "./role-labels";

const KOS_MELATI = "11111111-1111-4111-8111-111111111111";
const MAWAR = "22222222-2222-4222-8222-222222222222";
const GONE = "99999999-9999-4999-8999-999999999999";

const names = new Map([
  [KOS_MELATI, "Kos Melati"],
  [MAWAR, "Kontrakan Mawar"],
]);

const admin: RoleRef = { key: "admin", name: "Admin", builtin: true };
const karyawan: RoleRef = { key: "karyawan", name: "Karyawan", builtin: true };

describe("roleChips", () => {
  it("names the property of a limited role", () => {
    expect(
      roleChips([karyawan, { ...admin, propertyId: MAWAR }], names),
    ).toEqual([
      { id: "karyawan:", key: "karyawan", label: "Karyawan", property: null },
      {
        id: `admin:${MAWAR}`,
        key: "admin",
        label: "Admin",
        property: "Kontrakan Mawar",
      },
    ]);
  });

  it("falls back when the property is not in the list", () => {
    expect(
      roleChips([{ ...admin, propertyId: GONE }], names)[0]?.property,
    ).toBe(UNKNOWN_PROPERTY);
  });

  it("uses the API's role name, also for a custom role", () => {
    expect(
      roleChips([{ key: "penagih", name: "Penagih", builtin: false }], names)[0]
        ?.label,
    ).toBe("Penagih");
  });
});

describe("describeRoles", () => {
  it.each<{ name: string; roles: RoleRef[]; want: string }>([
    { name: "no role", roles: [], want: "" },
    { name: "one role", roles: [karyawan], want: "Karyawan" },
    {
      name: "a limited role",
      roles: [{ ...karyawan, propertyId: MAWAR }],
      want: "Karyawan di Kontrakan Mawar",
    },
    {
      name: "two roles",
      roles: [{ ...admin, propertyId: KOS_MELATI }, karyawan],
      want: "Admin di Kos Melati dan Karyawan",
    },
    {
      name: "three roles",
      roles: [
        { ...admin, propertyId: KOS_MELATI },
        { ...admin, propertyId: MAWAR },
        karyawan,
      ],
      want: "Admin di Kos Melati, Admin di Kontrakan Mawar dan Karyawan",
    },
  ])("describes $name", ({ roles, want }) => {
    expect(describeRoles(roles, names)).toBe(want);
  });
});
