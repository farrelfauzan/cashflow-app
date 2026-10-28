"use client";

import { cn } from "@/lib/utils";
import type { MemberStatusView } from "@/lib/member/member-status";

import { STATUS_TONE_CLASS } from "@/lib/member/status-tone";

type MemberStatusLabelProps = {
  status: MemberStatusView;
};

// The status on iPad: a dot in the status color and its label ("Aktif", "Nonaktif").
export function MemberStatusLabel({ status }: MemberStatusLabelProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 text-[15px] font-semibold",
        STATUS_TONE_CLASS[status.tone],
      )}
    >
      <span
        aria-hidden="true"
        className="size-2 shrink-0 rounded-full bg-current"
      />
      {status.label}
    </span>
  );
}
