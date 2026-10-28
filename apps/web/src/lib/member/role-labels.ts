import type { Property, RoleRef } from "@/lib/api/generated/model";

// How a member's roles read on screen: one chip per role ("Admin", "Karyawan · Kos Melati"),
// and one sentence for the "Anggota ditambahkan" note. The role name comes from the API; the
// property name from listProperties. Display only: what a member may do comes from
// capabilities.

// Shown when the property of a limited role is not in the list the viewer may read.
export const UNKNOWN_PROPERTY = "properti lain";

export type RoleChip = {
  // Unique within one member, for React keys.
  id: string;
  // The role key styles the chip; never used to decide what anyone may do.
  key: string;
  label: string;
  property: string | null;
};

export type PropertyNames = ReadonlyMap<string, string>;

export function propertyNames(properties: readonly Property[]): PropertyNames {
  return new Map(properties.map((p) => [p.id, p.name]));
}

export function roleChips(
  roles: readonly RoleRef[],
  names: PropertyNames,
): RoleChip[] {
  return roles.map((role) => ({
    id: `${role.key}:${role.propertyId ?? ""}`,
    key: role.key,
    label: role.name,
    property:
      role.propertyId === undefined
        ? null
        : (names.get(role.propertyId) ?? UNKNOWN_PROPERTY),
  }));
}

// "Karyawan", "Admin di Kos Melati dan Karyawan", "A, B dan C".
export function describeRoles(
  roles: readonly RoleRef[],
  names: PropertyNames,
): string {
  const parts = roleChips(roles, names).map((chip) =>
    chip.property === null ? chip.label : `${chip.label} di ${chip.property}`,
  );
  if (parts.length <= 1) return parts.join("");
  return `${parts.slice(0, -1).join(", ")} dan ${parts.at(-1)}`;
}
