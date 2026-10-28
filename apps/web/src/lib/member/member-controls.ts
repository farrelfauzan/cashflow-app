import type { Capability, Member } from "@/lib/api/generated/model";
import { canSee } from "@/lib/organization/org-menu";

// Which member controls show, from the signed-in member's capabilities (getOrgMe), never from
// role names. "conditional" shows the control and lets the API decide per member. Hiding is
// visibility only; the API checks every request.

export type MemberPermissions = {
  // "Tambah anggota" and "Kirim ulang undangan".
  canInvite: boolean;
  // "Atur" (roles and status).
  canManage: boolean;
};

export function memberPermissions(
  capabilities: readonly Capability[],
): MemberPermissions {
  return {
    canInvite: canSee(capabilities, "member:invite"),
    canManage: canSee(capabilities, "member:manage"),
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
