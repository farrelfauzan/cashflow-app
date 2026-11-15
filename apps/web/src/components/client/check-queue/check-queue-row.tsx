"use client";

import { ChevronRight } from "lucide-react";

import { TenantChip } from "@/components/client/tenant/tenant-chip";
import { cn } from "@/lib/utils";

import type { CheckRowView } from "./type";

type CheckQueueRowProps = {
  row: CheckRowView;
  onOpen: () => void;
};

// One waiting item: its icon tile, the kind in capitals, the main line and the detail; on the
// right its chip and when it arrived (under the text on the phone). A row that opens a drawer is
// one button at least 76px tall.
export function CheckQueueRow({ row, onOpen }: CheckQueueRowProps) {
  const Icon = row.icon;
  const content = (
    <>
      <span
        aria-hidden="true"
        className={cn(
          "flex size-11 shrink-0 items-center justify-center rounded-xl",
          row.iconTone === "warning"
            ? "bg-warning/15 text-warning"
            : "bg-accent text-primary",
        )}
      >
        <Icon className="size-5" />
      </span>
      <span className="flex min-w-0 flex-1 flex-col gap-1 md:flex-row md:items-center md:gap-4">
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="text-sm font-semibold tracking-wide text-muted-foreground uppercase md:text-base">
            {row.kindLabel}
          </span>
          <span className="text-base font-semibold md:text-[17px]">
            {row.title}
          </span>
          <span className="text-sm text-muted-foreground md:text-base">
            {row.detail}
          </span>
        </span>
        <span className="flex flex-wrap items-center gap-x-3 gap-y-1 md:flex-col md:items-end md:gap-1">
          <TenantChip tone={row.chip.tone}>{row.chip.label}</TenantChip>
          <span className="text-sm text-muted-foreground md:text-base">
            {row.arrived}
          </span>
        </span>
      </span>
    </>
  );
  const layout =
    "flex min-h-[76px] w-full items-start gap-3 px-4 py-3.5 text-left md:items-center md:gap-4 md:px-5";

  return (
    <li className="border-t first:border-t-0">
      {row.opens ? (
        <button
          type="button"
          onClick={onOpen}
          className={cn(
            layout,
            "transition-colors hover:bg-accent/60 focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none focus-visible:ring-inset",
          )}
        >
          {content}
          <ChevronRight
            aria-hidden="true"
            className="mt-3 size-5 shrink-0 text-muted-foreground md:mt-0"
          />
        </button>
      ) : (
        <div className={layout}>{content}</div>
      )}
    </li>
  );
}
