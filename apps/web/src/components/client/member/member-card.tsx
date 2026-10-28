"use client";

import { cn } from "@/lib/utils";
import type { Member } from "@/lib/api/generated/model";
import { formatPhone } from "@/lib/format/phone";
import { memberStatusView, roleChips, STATUS_TONE_CLASS } from "./utils";
import type { PropertyNames } from "./type";

import { ManageMemberButton } from "./manage-member-button";
import { OwnMemberChip } from "./own-member-chip";
import { ResendInvitationButton } from "./resend-invitation-button";
import { RoleChip } from "./role-chip";

type MemberCardProps = {
  orgId: string;
  member: Member;
  isSelf: boolean;
  canManage: boolean;
  canResend: boolean;
  propertyNames: PropertyNames;
  onManage: (memberId: string) => void;
};

// One member on the phone (board Members-Phone.dc.html): name, phone and status, "Atur", the
// roles, and the invitation with "Kirim ulang".
export function MemberCard({
  orgId,
  member,
  isSelf,
  canManage,
  canResend,
  propertyNames,
  onManage,
}: MemberCardProps) {
  const status = memberStatusView(member);
  const showFooter = status.note !== null || canResend;

  return (
    <li
      className={cn(
        "flex flex-col gap-2.5 rounded-[14px] border bg-card px-4 py-3.5",
        status.tone === "disabled" && "text-muted-foreground",
      )}
    >
      <div className="flex items-start justify-between gap-2.5">
        <div className="flex min-w-0 flex-col gap-0.5">
          <span className="text-[17px] font-semibold">
            {member.name}
            {isSelf && <OwnMemberChip />}
          </span>
          <span className="text-sm text-muted-foreground">
            {formatPhone(member.phone)} ·{" "}
            <span
              className={cn("font-semibold", STATUS_TONE_CLASS[status.tone])}
            >
              {status.label}
            </span>
          </span>
        </div>
        {canManage && (
          <ManageMemberButton member={member} onManage={onManage} />
        )}
      </div>
      <div className="flex flex-wrap gap-1.5">
        {roleChips(member.roles, propertyNames).map((chip) => (
          <RoleChip key={chip.id} chip={chip} />
        ))}
      </div>
      {showFooter && (
        <div className="flex flex-wrap items-center justify-between gap-x-2 border-t pt-2">
          {status.note !== null && (
            <span
              className={cn(
                "text-sm",
                status.note.tone === "warning"
                  ? "font-semibold text-warning"
                  : "text-muted-foreground",
              )}
            >
              {status.note.text}
            </span>
          )}
          {canResend && (
            <ResendInvitationButton
              orgId={orgId}
              member={member}
              label="Kirim ulang"
              className="-my-1"
            />
          )}
        </div>
      )}
    </li>
  );
}
