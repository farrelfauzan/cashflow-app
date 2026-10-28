"use client";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import type { FilterOption } from "./type";

type FilterSelectProps = {
  // The accessible name: "Filter properti".
  label: string;
  value: string;
  // The first option, which means no filter: "Semua properti".
  allValue: string;
  allLabel: string;
  options: readonly FilterOption[];
  onValueChange: (value: string) => void;
  className?: string;
};

// One filter of the tenant list: "Semua properti" and the choices.
export function FilterSelect({
  label,
  value,
  allValue,
  allLabel,
  options,
  onValueChange,
  className,
}: FilterSelectProps) {
  return (
    <Select value={value} onValueChange={onValueChange}>
      <SelectTrigger
        aria-label={label}
        className={cn(
          "w-full rounded-xl border-[1.5px] bg-card px-3.5 text-[15px] data-[size=default]:h-12 md:text-[17px]",
          className,
        )}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent position="popper">
        <SelectItem value={allValue} className="min-h-11 text-base">
          {allLabel}
        </SelectItem>
        {options.map((option) => (
          <SelectItem
            key={option.value}
            value={option.value}
            className="min-h-11 text-base"
          >
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
