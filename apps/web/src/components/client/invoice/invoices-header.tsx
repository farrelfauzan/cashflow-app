"use client";

import { MonthStepper } from "./month-stepper";

type InvoicesHeaderProps = {
  // "November 2026".
  monthLabel: string;
  // A line in place of the stepper when the month does not drive the list: "Semua tunggakan,
  // semua bulan" for "Terlambat" (debt has no month), "Meteran Oktober 2026" for the missing
  // meter readings (always last month). Null shows the stepper.
  fixedLine: string | null;
  canGoNext: boolean;
  onPrevious: () => void;
  onNext: () => void;
};

// The title and the month under it, with the stepper that moves it. The screen has no primary
// action.
export function InvoicesHeader({
  monthLabel,
  fixedLine,
  canGoNext,
  onPrevious,
  onNext,
}: InvoicesHeaderProps) {
  return (
    <header className="flex flex-col gap-1.5">
      <h1 className="text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
        Penagihan
      </h1>
      {fixedLine !== null ? (
        <p className="flex min-h-11 items-center text-[15px] text-muted-foreground md:text-[17px]">
          {fixedLine}
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
