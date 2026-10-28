import type { RoleRef } from "@/lib/api/generated/model";

// The roles under the organization's name in the menu ("Admin", "Karyawan, Admin"). Display
// only: what the member may do comes from capabilities, never from these names. A role held
// for several properties shows once.
export function roleSummary(roles: readonly RoleRef[]): string {
  return [...new Set(roles.map((role) => role.name))].join(", ");
}
