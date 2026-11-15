"use client";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

import type { CountAction } from "./type";
import { COUNT_BUTTON, OWN_DEPOSIT_TEXT } from "./utils";

type CountActionCellProps = {
  action: CountAction;
  staffName: string;
  onCount: () => void;
  // Full width on the phone's card; its own column on iPad.
  className?: string;
};

// "Hitung" (secondary: the drawer holds the screen's one primary action), or on the member's own
// deposit the muted "Dihitung admin lain". Nothing for a member who may only read.
export function CountActionCell({
  action,
  staffName,
  onCount,
  className,
}: CountActionCellProps) {
  if (action === "none") return null;
  if (action === "own") {
    return (
      <span
        className={cn("text-sm text-muted-foreground md:text-base", className)}
      >
        {OWN_DEPOSIT_TEXT}
      </span>
    );
  }
  return (
    <Button
      type="button"
      variant="outline"
      aria-label={`${COUNT_BUTTON} setoran ${staffName}`}
      onClick={onCount}
      className={cn(
        "h-11 rounded-xl border-[1.5px] bg-card text-base font-semibold",
        className,
      )}
    >
      {COUNT_BUTTON}
    </Button>
  );
}
