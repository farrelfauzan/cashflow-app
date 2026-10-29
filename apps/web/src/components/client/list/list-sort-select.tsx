"use client";

import { useId } from "react";

import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import type { ListUrl, SortOptions } from "./type";
import { SORT_LABEL, SORT_PARAM, sortParam } from "./utils";

type ListSortSelectProps = {
  // The list's choices; the first is its default, which stays out of the URL.
  options: SortOptions;
  // The sort in the URL (already one of the options).
  value: string;
  onChange: ListUrl["change"];
  className?: string;
};

// "Urutkan": how the list is ordered. A change goes into the URL (and back to page 1), and the
// API sorts. The caller passes its own options, so each list offers what its API accepts.
export function ListSortSelect({
  options,
  value,
  onChange,
  className,
}: ListSortSelectProps) {
  const id = useId();
  return (
    <div className={cn("flex items-center gap-2.5 lg:w-[310px]", className)}>
      <Label
        htmlFor={id}
        className="shrink-0 text-[15px] font-semibold text-muted-foreground md:text-base"
      >
        {SORT_LABEL}
      </Label>
      <Select
        value={value}
        onValueChange={(next) =>
          onChange({ [SORT_PARAM]: sortParam(next, options) })
        }
      >
        <SelectTrigger
          id={id}
          className="min-w-0 flex-1 rounded-xl border-[1.5px] bg-card px-3.5 text-[15px] data-[size=default]:h-12 md:text-[17px]"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent position="popper">
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
    </div>
  );
}
