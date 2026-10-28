import type { Member } from "@/lib/api/generated/model";
import { formatTimestampDate } from "@/lib/format/date";

// The status column of the members list (board Members.dc.html): whether the member can work in
// the organization and, for someone who never opened it, whether their invitation went out.

export type MemberStatusTone = "active" | "pending" | "disabled";

export type MemberStatusView = {
  label: string;
  tone: MemberStatusTone;
  // A second line under the status; "warning" when the invitation did not go out.
  note: { text: string; tone: "muted" | "warning" } | null;
};

export function memberStatusView(member: Member): MemberStatusView {
  if (member.status === "disabled") {
    return { label: "Nonaktif", tone: "disabled", note: null };
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
