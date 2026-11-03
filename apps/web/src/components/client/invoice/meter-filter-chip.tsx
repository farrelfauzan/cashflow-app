"use client";

import { X } from "lucide-react";

import { METER_CHIP_CLEAR_LABEL, METER_CHIP_LABEL } from "./utils";

type MeterFilterChipProps = {
  onClear: () => void;
};

// Shows that the list is the leases with a meter amount missing, and takes that off.
export function MeterFilterChip({ onClear }: MeterFilterChipProps) {
  return (
    <button
      type="button"
      aria-label={METER_CHIP_CLEAR_LABEL}
      onClick={onClear}
      className="inline-flex min-h-11 items-center gap-2 self-start rounded-xl border-[1.5px] border-warning bg-warning/10 px-3.5 text-[15px] font-semibold md:text-base"
    >
      {METER_CHIP_LABEL}
      <X aria-hidden="true" className="size-4" strokeWidth={2.4} />
    </button>
  );
}
