"use client";

import type { CheckQueueKind } from "@/lib/api/generated/model";
import { cn } from "@/lib/utils";

import type { CheckFilterOption } from "./type";
import { FILTERS_LABEL } from "./utils";

type CheckQueueFiltersProps = {
  options: readonly CheckFilterOption[];
  // The kind shown; undefined for every kind ("Semua").
  selected: CheckQueueKind | undefined;
  onSelect: (kind: CheckQueueKind | null) => void;
};

// "Semua 6", "Bukti transfer 2", ...: toggle buttons (aria-pressed), each at least 44px tall,
// with how many items of that kind wait once the API answered. They wrap on the phone.
export function CheckQueueFilters({
  options,
  selected,
  onSelect,
}: CheckQueueFiltersProps) {
  return (
    <div
      role="group"
      aria-label={FILTERS_LABEL}
      className="flex flex-wrap gap-2"
    >
      {options.map((option) => {
        const pressed = (option.kind ?? undefined) === selected;
        return (
          <button
            key={option.kind ?? "all"}
            type="button"
            aria-pressed={pressed}
            onClick={() => onSelect(option.kind)}
            className={cn(
              "inline-flex min-h-11 items-center gap-2 rounded-full border-[1.5px] px-4 text-[15px] font-semibold transition-colors focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none md:text-base",
              pressed
                ? "border-primary bg-primary text-primary-foreground"
                : "bg-card text-foreground hover:bg-accent",
            )}
          >
            {option.label}
            {option.count !== undefined && (
              <span
                className={cn(
                  "tabular-nums",
                  pressed ? "text-primary-foreground" : "text-muted-foreground",
                )}
              >
                {option.count}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
}
