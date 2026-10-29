"use client";

import { cn } from "@/lib/utils";
import type { SummaryTileView } from "./type";

type ImportSummaryTileProps = {
  tile: SummaryTileView;
};

// One count in the check's summary ("48 Baris"); "Perlu diperbaiki" above 0 in the warning style.
export function ImportSummaryTile({ tile }: ImportSummaryTileProps) {
  return (
    <div
      className={cn(
        "flex flex-col gap-0.5 rounded-xl border px-3.5 py-3",
        tile.warning ? "border-warning/40 bg-warning/10" : "bg-card",
      )}
    >
      <span
        className={cn(
          "text-2xl leading-tight font-bold md:text-[28px]",
          tile.warning && "text-warning",
        )}
      >
        {tile.value}
      </span>
      <span
        className={cn(
          "text-[15px] md:text-base",
          tile.warning ? "text-warning" : "text-muted-foreground",
        )}
      >
        {tile.label}
      </span>
    </div>
  );
}
