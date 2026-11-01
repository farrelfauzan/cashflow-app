"use client";

import { cn } from "@/lib/utils";
import type { AccountTileView } from "./type";

type AccountTileProps = {
  tile: AccountTileView;
  className?: string;
};

// One summary tile above the lists: its label, the figure and the small lines under it.
export function AccountTile({ tile, className }: AccountTileProps) {
  return (
    <div
      role="group"
      aria-label={tile.label}
      className={cn(
        "flex min-w-0 flex-col gap-1 rounded-[14px] border bg-card p-4 md:rounded-2xl md:px-5 md:py-[18px]",
        className,
      )}
    >
      <span className="text-sm text-muted-foreground md:text-base">
        {tile.label}
      </span>
      <span
        className={cn(
          "text-[22px] leading-tight font-bold md:text-[26px]",
          tile.tone === "warning" && "text-warning",
        )}
      >
        {tile.value}
      </span>
      {tile.notes.map((note) => (
        <span
          key={note}
          className="text-sm leading-[1.4] text-muted-foreground md:text-base"
        >
          {note}
        </span>
      ))}
    </div>
  );
}
