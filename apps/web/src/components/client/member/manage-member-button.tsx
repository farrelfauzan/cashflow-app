"use client";

import { Button } from "@/components/ui/button";
import type { Member } from "@/lib/api/generated/model";

type ManageMemberButtonProps = {
  member: Member;
  onManage: (memberId: string) => void;
};

// "Atur": opens the member's "Atur akses" panel.
export function ManageMemberButton({
  member,
  onManage,
}: ManageMemberButtonProps) {
  return (
    <Button
      type="button"
      variant="outline"
      aria-label={`Atur akses ${member.name}`}
      onClick={() => onManage(member.id)}
      className="shrink-0 rounded-[10px] border-[1.5px] bg-card px-4 text-[15px] font-semibold"
    >
      Atur
    </Button>
  );
}
