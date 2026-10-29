import { ApiError } from "@/lib/api/errors";
import { toFormError, type FormError } from "@/lib/api/form-error";
import { formatTimestampDate } from "@/lib/format/date";
import type { Member, RoleGrant, RoleRef } from "@/lib/api/generated/model";
import type { AppAbility } from "@/lib/capabilities";

import type {
  MemberPermissions,
  MemberStatusTone,
  MemberStatusView,
  ResendError,
  RoleChip,
  RoleFormErrors,
  RoleFormState,
  ScopedRoleKey,
  ScopedRoleState,
} from "./type";

export const EMPTY_NAME_MESSAGE = "Isi nama anggota.";
export const EMPTY_PHONE_MESSAGE = "Isi nomor WhatsApp.";
// The API's limit (InviteMember.name maxLength).
export const NAME_MAX_LENGTH = 100;

export const INPUT_CLASS =
  "h-13 rounded-xl border-[1.5px] bg-card px-3.5 text-[17px] aria-invalid:border-2";

export const RESENT_MESSAGE = "Undangan sudah dikirim ulang.";

// The chip's surface by role key, from the theme tokens. Styling only; what a member may do
// comes from capabilities. A custom role gets the plain surface.
export const ROLE_CHIP_TONE: Record<string, string> = {
  supervisor: "bg-secondary text-foreground",
  admin: "bg-accent text-primary",
  karyawan: "bg-warning/15 text-foreground",
};

export const SCOPED_ROLE_LABELS: Record<ScopedRoleKey, string> = {
  admin: "Admin",
  karyawan: "Karyawan",
};

// Each row is a whole label, so the touch target is at least 44px tall.
export const ROLE_OPTION_CLASS =
  "min-h-11 gap-2.5 text-base leading-normal font-normal";

// Member controls: which controls show, from the member's ability (getOrgMe's capabilities),
// never from role names. "conditional" shows the control and lets the API decide per member.
// Hiding is visibility only; the API checks every request.
export function memberPermissions(ability: AppAbility): MemberPermissions {
  return {
    canInvite: ability.can("invite", "member"),
    canManage: ability.can("manage", "member"),
  };
}

// Nobody changes their own roles or status, so the own row never has "Atur".
export function canManageMember(
  member: Member,
  ownMembershipId: string,
  permissions: MemberPermissions,
): boolean {
  return permissions.canManage && member.id !== ownMembershipId;
}

// An invited member who has not opened the organization yet, and is not disabled. Founders
// were never invited and have nothing to resend.
export function canResendInvitation(
  member: Member,
  permissions: MemberPermissions,
): boolean {
  return (
    permissions.canInvite &&
    member.status === "active" &&
    member.invitation !== null &&
    member.firstSignInAt === null
  );
}

// Member errors: where each members panel shows a failed request. The message is always the
// API's own (already Bahasa Indonesia); only its place differs, decided by the error's code and
// field.
// "Tambah anggota": invalid_phone and invalid_input on name, phone or roles go under that
// control. already_member names the phone field too, but it is not a typing mistake, so it goes
// in the alert at the top of the panel.
export function inviteFormError(error: unknown): FormError {
  if (error instanceof ApiError && error.code === "already_member") {
    return { field: null, message: error.message };
  }
  return toFormError(error, ["name", "phone", "roles"]);
}

// "Atur akses": invalid_input on roles goes under the roles; everything else (last_admin, 403,
// a member removed meanwhile) in the alert.
export function accessFormError(error: unknown): FormError {
  return toFormError(error, ["roles"]);
}

export const UNDELIVERABLE_HINT = "Periksa nomornya bersama anggota ini.";

export function resendError(error: unknown): ResendError {
  const { message } = toFormError(error, []);
  if (error instanceof ApiError && error.code === "whatsapp_undeliverable") {
    return { message: `${message} ${UNDELIVERABLE_HINT}`, canRetry: false };
  }
  return { message, canRetry: true };
}

// Member status: whether the member can work in the organization and, for someone who never
// opened it, whether their invitation went out. A disabled member stays the collector of their
// leases until an admin picks another one (the user's decision), so the list flags how many
// tenants are left without an active collector.
export function memberStatusView(member: Member): MemberStatusView {
  if (member.status === "disabled") {
    const note =
      member.collectorLeaseCount > 0
        ? {
            text: `${member.collectorLeaseCount} penyewa belum punya penagih`,
            tone: "warning" as const,
          }
        : null;
    return { label: "Nonaktif", tone: "disabled", note };
  }
  if (member.firstSignInAt !== null) {
    return { label: "Aktif", tone: "active", note: null };
  }

  const pending = { label: "Belum pernah masuk", tone: "pending" } as const;
  const { invitation } = member;
  // A founder is never invited; there is nothing to say about an invitation.
  if (invitation === null) return { ...pending, note: null };
  if (invitation.state === "not_sent") {
    return {
      ...pending,
      note: { text: "Undangan belum terkirim", tone: "warning" },
    };
  }
  const text =
    invitation.lastSentAt === undefined
      ? "Undangan terkirim"
      : `Undangan terkirim ${formatTimestampDate(invitation.lastSentAt)}`;
  return { ...pending, note: { text, tone: "muted" } };
}

// Role form: how the roles part of the panels maps to the API's RoleGrant list. Admin and
// Karyawan can each cover the whole organization or some properties; the API takes one grant
// per property for the second ("give the same role once per property"). Supervisor is only set
// when the organization is created, so the form never adds or removes it: a member's Supervisor
// grants are sent back unchanged.
export const SCOPED_ROLES: readonly ScopedRoleKey[] = ["admin", "karyawan"];

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

// Role labels: how a member's roles read on screen: one chip per role ("Admin", "Karyawan · Kos
// Melati"), and one sentence for the "Anggota ditambahkan" note. The role name and the property
// name both come from the API (RoleRef). Display only: what a member may do comes from
// capabilities.
// Shown if the API ever sends a limited role without its property's name.
export const UNKNOWN_PROPERTY = "properti lain";

export function roleChips(roles: readonly RoleRef[]): RoleChip[] {
  return roles.map((role) => ({
    id: `${role.key}:${role.propertyId ?? ""}`,
    key: role.key,
    label: role.name,
    property:
      role.propertyId === undefined
        ? null
        : (role.propertyName ?? UNKNOWN_PROPERTY),
  }));
}

// "Karyawan", "Admin di Kos Melati dan Karyawan", "A, B dan C".
export function describeRoles(roles: readonly RoleRef[]): string {
  const parts = roleChips(roles).map((chip) =>
    chip.property === null ? chip.label : `${chip.label} di ${chip.property}`,
  );
  if (parts.length <= 1) return parts.join("");
  return `${parts.slice(0, -1).join(", ")} dan ${parts.at(-1)}`;
}

// Status tone: the text color of each member status, from the theme tokens.
export const STATUS_TONE_CLASS: Record<MemberStatusTone, string> = {
  active: "text-primary",
  pending: "text-foreground/85",
  disabled: "text-muted-foreground",
};
