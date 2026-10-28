"use client";

import { cn } from "@/lib/utils";
import type { RoleChip as RoleChipData } from "@/lib/member/role-labels";

type RoleChipProps = {
  chip: RoleChipData;
};

// The chip's surface by role key, from the theme tokens. Styling only; what a member may do
// comes from capabilities. A custom role gets the plain surface.
const TONE: Record<string, string> = {
  supervisor: "bg-secondary text-foreground",
  admin: "bg-accent text-primary",
  karyawan: "bg-warning/15 text-foreground",
};

// One role of a member: "Admin", or "Admin · Kos Melati" when limited to a property.
export function RoleChip({ chip }: RoleChipProps) {
  return (
    <span
      className={cn(
        "inline-flex min-h-7 items-center gap-1.5 rounded-lg px-2.5 text-sm font-semibold lg:min-h-[30px] lg:text-[15px]",
        TONE[chip.key] ?? "border bg-card text-foreground",
      )}
    >
      {chip.label}
      {chip.property !== null && (
        <span className="font-medium">· {chip.property}</span>
      )}
    </span>
  );
}
