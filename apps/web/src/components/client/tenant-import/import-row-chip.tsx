"use client";

import type { ReactNode } from "react";

import { cn } from "@/lib/utils";
import type { RowChipTone } from "./type";
import { ROW_CHIP_TONE_CLASS } from "./utils";

type ImportRowChipProps = {
  tone: RowChipTone;
  children: ReactNode;
};

// A small label on a checked row: its status ("Siap") or what it will create ("Penyewa baru").
export function ImportRowChip({ tone, children }: ImportRowChipProps) {
  return (
    <span
      className={cn(
        "inline-flex min-h-7 items-center rounded-lg px-2.5 text-sm font-semibold whitespace-nowrap md:text-base",
        ROW_CHIP_TONE_CLASS[tone],
      )}
    >
      {children}
    </span>
  );
}
