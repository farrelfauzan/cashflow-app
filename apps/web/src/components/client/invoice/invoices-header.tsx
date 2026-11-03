"use client";

import { ALL_MONTHS_TEXT } from "./utils";

import { MonthStepper } from "./month-stepper";

type InvoicesHeaderProps = {
  // "November 2026".
  monthLabel: string;
  // The status is "Terlambat": debt has no month, so the stepper gives way to a line.
  allMonths: boolean;
  canGoNext: boolean;
  onPrevious: () => void;
  onNext: () => void;
};

// The title and the month under it, with the stepper that moves it. The screen has no primary
// action.
export function InvoicesHeader({
  monthLabel,
  allMonths,
  canGoNext,
  onPrevious,
  onNext,
}: InvoicesHeaderProps) {
  return (
    <header className="flex flex-col gap-1.5">
      <h1 className="text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
        Penagihan
      </h1>
      {allMonths ? (
        <p className="flex min-h-11 items-center text-[15px] text-muted-foreground md:text-[17px]">
          {ALL_MONTHS_TEXT}
        </p>
      ) : (
        <MonthStepper
          label={monthLabel}
          canGoNext={canGoNext}
          onPrevious={onPrevious}
          onNext={onNext}
        />
      )}
    </header>
  );
}
