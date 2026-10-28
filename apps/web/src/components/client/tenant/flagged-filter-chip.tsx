"use client";

import { X } from "lucide-react";

type FlaggedFilterChipProps = {
  onClear: () => void;
};

// Shows that the list is narrowed to the tenants without a collector, and takes that off.
export function FlaggedFilterChip({ onClear }: FlaggedFilterChipProps) {
  return (
    <button
      type="button"
      aria-label="Hanya yang belum punya penagih, hapus penyaring"
      onClick={onClear}
      className="inline-flex min-h-11 items-center gap-2 self-start rounded-xl border-[1.5px] border-warning bg-warning/10 px-3.5 text-[15px] font-semibold md:text-base"
    >
      Hanya yang belum punya penagih
      <X aria-hidden="true" className="size-4" strokeWidth={2.4} />
    </button>
  );
}
