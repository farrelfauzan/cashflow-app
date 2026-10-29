"use client";

import { cn } from "@/lib/utils";
import type { FilterChipView, RowFilter } from "./type";

type ImportRowFiltersProps = {
  chips: readonly FilterChipView[];
  active: RowFilter;
  onChange: (filter: RowFilter) => void;
};

// "Semua 48", "Perlu diperbaiki 3", "Ada catatan 5": which rows the list shows.
export function ImportRowFilters({
  chips,
  active,
  onChange,
}: ImportRowFiltersProps) {
  return (
    <div
      role="group"
      aria-label="Tampilkan baris"
      className="flex gap-2 overflow-x-auto lg:flex-wrap"
    >
      {chips.map((chip) => {
        const pressed = chip.filter === active;
        return (
          <button
            key={chip.filter}
            type="button"
            aria-pressed={pressed}
            onClick={() => onChange(chip.filter)}
            className={cn(
              "inline-flex min-h-11 shrink-0 items-center rounded-[10px] border-[1.5px] px-4 text-base font-semibold whitespace-nowrap focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none",
              pressed
                ? "border-primary bg-primary text-primary-foreground"
                : "border-border bg-card text-foreground",
            )}
          >
            {chip.label}
          </button>
        );
      })}
    </div>
  );
}
