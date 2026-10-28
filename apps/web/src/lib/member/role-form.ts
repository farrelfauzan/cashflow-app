import type { RoleGrant, RoleRef } from "@/lib/api/generated/model";

// The roles part of the "Atur akses" and "Tambah anggota" panels, and how it maps to the API's
// RoleGrant list. Admin and Karyawan can each cover the whole organization or some properties;
// the API takes one grant per property for the second ("give the same role once per property").
// Supervisor is only set when the organization is created, so the form never adds or removes
// it: a member's Supervisor grants are sent back unchanged.

export type RoleScope = "organization" | "properties";

export type ScopedRoleKey = "admin" | "karyawan";

export const SCOPED_ROLES: readonly ScopedRoleKey[] = ["admin", "karyawan"];

export type ScopedRoleState = {
  selected: boolean;
  scope: RoleScope;
  // Used only when scope is "properties".
  propertyIds: readonly string[];
};

export type RoleFormState = {
  supervisor: readonly RoleGrant[];
  admin: ScopedRoleState;
  karyawan: ScopedRoleState;
};

// Where the form shows a problem it finds before sending: under one role, or under the roles
// as a whole.
export type RoleFormErrors = Partial<Record<ScopedRoleKey | "roles", string>>;

export const NO_ROLE_MESSAGE = "Pilih minimal satu peran.";
export const NO_PROPERTY_MESSAGE = "Pilih minimal satu properti.";

const UNSELECTED: ScopedRoleState = {
  selected: false,
  scope: "organization",
  propertyIds: [],
};

// A new member starts as Karyawan for the whole organization, as on the board.
export function inviteRoleForm(): RoleFormState {
  return {
    supervisor: [],
    admin: UNSELECTED,
    karyawan: { ...UNSELECTED, selected: true },
  };
}

function scopedRoleFromRoles(
  roles: readonly RoleRef[],
  key: ScopedRoleKey,
): ScopedRoleState {
  const held = roles.filter((role) => role.key === key);
  if (held.length === 0) return UNSELECTED;
  if (held.some((role) => role.propertyId === undefined)) {
    return { selected: true, scope: "organization", propertyIds: [] };
  }
  return {
    selected: true,
    scope: "properties",
    propertyIds: [...new Set(held.flatMap((role) => role.propertyId ?? []))],
  };
}

// The form for a member's current roles. Roles other than the three built-in ones (custom
// roles) are not part of this form.
export function roleFormFromRoles(roles: readonly RoleRef[]): RoleFormState {
  return {
    supervisor: roles
      .filter((role) => role.key === "supervisor")
      .map((role) =>
        role.propertyId === undefined
          ? { role: "supervisor" }
          : { role: "supervisor", propertyId: role.propertyId },
      ),
    admin: scopedRoleFromRoles(roles, "admin"),
    karyawan: scopedRoleFromRoles(roles, "karyawan"),
  };
}

function grantsFor(key: ScopedRoleKey, state: ScopedRoleState): RoleGrant[] {
  if (!state.selected) return [];
  if (state.scope === "organization") return [{ role: key }];
  return state.propertyIds.map((propertyId) => ({ role: key, propertyId }));
}

// The body's roles for inviteMember and setMemberRoles.
export function roleGrantsFromForm(state: RoleFormState): RoleGrant[] {
  return [
    ...state.supervisor,
    ...grantsFor("admin", state.admin),
    ...grantsFor("karyawan", state.karyawan),
  ];
}

// Friendly checks before sending. The API checks the same and more; this only saves a round
// trip for the obvious cases.
export function roleFormErrors(state: RoleFormState): RoleFormErrors {
  const errors: RoleFormErrors = {};
  for (const key of SCOPED_ROLES) {
    const role = state[key];
    if (
      role.selected &&
      role.scope === "properties" &&
      role.propertyIds.length === 0
    ) {
      errors[key] = NO_PROPERTY_MESSAGE;
    }
  }
  if (
    state.supervisor.length === 0 &&
    !state.admin.selected &&
    !state.karyawan.selected
  ) {
    errors.roles = NO_ROLE_MESSAGE;
  }
  return errors;
}

export function toggleProperty(
  propertyIds: readonly string[],
  propertyId: string,
  checked: boolean,
): string[] {
  const rest = propertyIds.filter((id) => id !== propertyId);
  return checked ? [...rest, propertyId] : rest;
}
