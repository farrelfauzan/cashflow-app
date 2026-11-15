"use client";

import type { ReactNode } from "react";

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
  // The links to Penagihan's sub-screens ("Setoran tunai", "Antrean cek"): right of the title on
  // iPad, full width under it on the phone.
  links: ReactNode;
};

// The title and the month under it, with the stepper that moves it, and the links to the
// sub-screens. The screen has no primary action.
export function InvoicesHeader({
  monthLabel,
  fixedLine,
  canGoNext,
  onPrevious,
  onNext,
  links,
}: InvoicesHeaderProps) {
  return (
    <header className="grid grid-cols-1 gap-1.5 lg:grid-cols-[minmax(0,1fr)_auto] lg:gap-x-5">
      <h1 className="text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
        Penagihan
      </h1>
      <div className="flex flex-col gap-2 empty:hidden max-lg:mt-1 lg:col-start-2 lg:row-span-2 lg:row-start-1 lg:flex-row lg:items-end lg:self-end">
        {links}
      </div>
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
