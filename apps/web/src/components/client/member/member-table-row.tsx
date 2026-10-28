"use client";

import { cn } from "@/lib/utils";
import type { Member } from "@/lib/api/generated/model";
import { formatPhone } from "@/lib/format/phone";
import { memberStatusView } from "@/lib/member/member-status";
import { roleChips, type PropertyNames } from "@/lib/member/role-labels";

import { ManageMemberButton } from "./manage-member-button";
import { MemberStatusLabel } from "./member-status-label";
import { OwnMemberChip } from "./own-member-chip";
import { ResendInvitationButton } from "./resend-invitation-button";
import { RoleChip } from "./role-chip";

type MemberTableRowProps = {
  orgId: string;
  member: Member;
  isSelf: boolean;
  canManage: boolean;
  canResend: boolean;
  propertyNames: PropertyNames;
  onManage: (memberId: string) => void;
};

// One member on iPad: name and phone, roles, status, and "Atur".
export function MemberTableRow({
  orgId,
  member,
  isSelf,
  canManage,
  canResend,
  propertyNames,
  onManage,
}: MemberTableRowProps) {
  const status = memberStatusView(member);

  return (
    <li
      className={cn(
        "grid min-h-[60px] grid-cols-[240px_minmax(0,1fr)_200px_80px] items-center gap-4 border-t px-5 py-1.5 first:border-t-0",
        status.tone === "disabled" && "text-muted-foreground",
      )}
    >
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="truncate text-[17px] font-semibold">
          {member.name}
          {isSelf && <OwnMemberChip />}
        </span>
        <span className="text-[15px] text-muted-foreground">
          {formatPhone(member.phone)}
        </span>
      </div>
      <div className="flex flex-wrap gap-1.5">
        {roleChips(member.roles, propertyNames).map((chip) => (
          <RoleChip key={chip.id} chip={chip} />
        ))}
      </div>
      <div className="flex flex-col items-start">
        <MemberStatusLabel status={status} />
        {status.note !== null && (
          <span
            className={cn(
              "pl-3.5 text-sm",
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
            label="Kirim ulang undangan"
            className="-mb-1.5 ml-2"
          />
        )}
      </div>
      <div className="flex justify-end">
        {canManage && (
          <ManageMemberButton member={member} onManage={onManage} />
        )}
      </div>
    </li>
  );
}
