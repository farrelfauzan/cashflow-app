"use client";

import type { ReactNode } from "react";

import { cn } from "@/lib/utils";
import { CHIP_TONE_CLASS } from "./utils";
import type { ChipTone } from "./type";

type TenantChipProps = {
  tone: ChipTone;
  children: ReactNode;
};

// A small label: a status ("Aktif"), the warning "Belum punya penagih", or an extra charge.
export function TenantChip({ tone, children }: TenantChipProps) {
  return (
    <span
      className={cn(
        "inline-flex min-h-7 items-center rounded-lg px-2.5 text-sm font-semibold whitespace-nowrap md:text-base",
        CHIP_TONE_CLASS[tone],
      )}
    >
      {children}
    </span>
  );
}
