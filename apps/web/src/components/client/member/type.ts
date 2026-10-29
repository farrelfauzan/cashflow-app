import type { Member, RoleGrant } from "@/lib/api/generated/model";

export type TextFieldErrors = Partial<Record<"name" | "phone", string>>;

export type OpenPanel = Exclude<MemberPanelState, { kind: "closed" }>;

export type Outcome = { message: string; failed: boolean };

// Member controls: which controls show, from the signed-in member's capabilities.
export type MemberPermissions = {
  // "Tambah anggota" and "Kirim ulang undangan".
  canInvite: boolean;
  // "Atur" (roles and status).
  canManage: boolean;
};

// Member errors: where a failed request shows, "Kirim ulang undangan". A number that cannot
// receive WhatsApp (422 whatsapp_undeliverable) will fail again whatever happens, and every try
// spends one of the number's few sends, so the row stops offering the resend and asks the admin
// to check the number instead. Rate limits and a WhatsApp outage can be tried again later.
export type ResendError = {
  message: string;
  canRetry: boolean;
};

// Member status: the status column of the members list.
export type MemberStatusTone = "active" | "pending" | "disabled";

export type MemberStatusView = {
  label: string;
  tone: MemberStatusTone;
  // A second line under the status; "warning" when the invitation did not go out.
  note: { text: string; tone: "muted" | "warning" } | null;
};

// Panel state: the side panel of the members screen; one at a time.
export type MemberPanelState =
  | { kind: "closed" }
  // "Atur akses" for one member. Only the id is kept: the member shown is always the one in
  // the latest list, so a change made here shows at once.
  | { kind: "access"; memberId: string }
  | { kind: "invite" }
  // "Anggota ditambahkan": the member inviteMember returned.
  | { kind: "invited"; member: Member }
  | { kind: "roles" };

// Role form: the roles part of the "Atur akses" and "Tambah anggota" panels.
export type RoleScope = "organization" | "properties";

export type ScopedRoleKey = "admin" | "karyawan";

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

// Role labels: how a member's roles read on screen.
export type RoleChip = {
  // Unique within one member, for React keys.
  id: string;
  // The role key styles the chip; never used to decide what anyone may do.
  key: string;
  label: string;
  property: string | null;
};
