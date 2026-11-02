"use client";

import { ChevronLeft, ChevronRight } from "lucide-react";

import { Button } from "@/components/ui/button";
import { NEXT_MONTH_LABEL, PREVIOUS_MONTH_LABEL } from "./utils";

type MonthStepperProps = {
  // "November 2026".
  label: string;
  // "Bulan berikutnya" is disabled on this month.
  canGoNext: boolean;
  onPrevious: () => void;
  onNext: () => void;
};

// "‹ November 2026 ›": the month the list shows, with a button for the month before and the one
// after.
export function MonthStepper({
  label,
  canGoNext,
  onPrevious,
  onNext,
}: MonthStepperProps) {
  return (
    <div className="flex items-center gap-1">
      <Button
        type="button"
        variant="outline"
        size="icon"
        aria-label={PREVIOUS_MONTH_LABEL}
        onClick={onPrevious}
        className="rounded-xl border-[1.5px] bg-card"
      >
        <ChevronLeft aria-hidden="true" className="size-5" />
      </Button>
      <span
        aria-live="polite"
        className="min-w-[150px] px-2 text-center text-[17px] font-semibold"
      >
        {label}
      </span>
      <Button
        type="button"
        variant="outline"
        size="icon"
        aria-label={NEXT_MONTH_LABEL}
        disabled={!canGoNext}
        onClick={onNext}
        className="rounded-xl border-[1.5px] bg-card"
      >
        <ChevronRight aria-hidden="true" className="size-5" />
      </Button>
    </div>
  );
}
