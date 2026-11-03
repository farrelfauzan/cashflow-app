"use client";

import { Button } from "@/components/ui/button";
import { CORRECT_BUTTON_TEXT } from "./utils";

type CorrectButtonProps = {
  // Names the record for screen readers: "Koreksi tagihan November 2026".
  label: string;
  onClick: () => void;
};

// The small "Koreksi" action of an invoice or payment row, a 44px touch target.
export function CorrectButton({ label, onClick }: CorrectButtonProps) {
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      aria-label={label}
      onClick={onClick}
      className="rounded-[10px] border-[1.5px] bg-card px-3.5 text-[15px] font-semibold md:text-base"
    >
      {CORRECT_BUTTON_TEXT}
    </Button>
  );
}
