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

import { PAGE_SIZE_LABEL, PAGE_SIZES, toPageSize } from "./utils";

type PageSizeSelectProps = {
  pageSize: number;
  onPageSizeChange: (pageSize: number) => void;
};

// "Baris per halaman": 20, 50 or 100 rows. The caller decides what a change does to the page.
export function PageSizeSelect({
  pageSize,
  onPageSizeChange,
}: PageSizeSelectProps) {
  const id = useId();
  return (
    <div className="flex items-center gap-2">
      <Label
        htmlFor={id}
        className="text-sm font-normal text-muted-foreground md:text-base"
      >
        {PAGE_SIZE_LABEL}
      </Label>
      <Select
        value={String(pageSize)}
        onValueChange={(value) => onPageSizeChange(toPageSize(value))}
      >
        <SelectTrigger
          id={id}
          className="min-w-20 bg-card text-sm data-[size=default]:h-11 md:text-base"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {PAGE_SIZES.map((size) => (
            <SelectItem
              key={size}
              value={String(size)}
              className="min-h-11 md:text-base"
            >
              {size}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
