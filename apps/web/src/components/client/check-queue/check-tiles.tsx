"use client";

import { cn } from "@/lib/utils";

import type { CheckTile } from "./type";

type CheckTilesProps = {
  tiles: readonly CheckTile[];
};

// The figures of an item side by side ("Total kwitansi", "Diterima admin", "Selisih"). They
// stack on the phone: a phone number does not fit in half its width.
export function CheckTiles({ tiles }: CheckTilesProps) {
  return (
    <dl
      className={cn(
        "grid gap-2.5",
        tiles.length >= 3
          ? "grid-cols-1 sm:grid-cols-3"
          : "grid-cols-1 sm:grid-cols-2",
      )}
    >
      {tiles.map((tile) => (
        <div
          key={tile.label}
          className={cn(
            "flex flex-col gap-0.5 rounded-xl px-3.5 py-3",
            tile.tone === "warning" && "border border-warning bg-warning/15",
            tile.tone === "accent" && "border border-primary bg-accent",
            tile.tone === "plain" && "bg-muted",
          )}
        >
          <dt className="text-sm text-muted-foreground md:text-base">
            {tile.label}
          </dt>
          <dd
            className={cn(
              "text-lg font-bold tabular-nums",
              tile.tone === "accent" && "text-primary",
            )}
          >
            {tile.value}
          </dd>
          {tile.sub !== null && (
            <dd className="text-sm text-muted-foreground md:text-base">
              {tile.sub}
            </dd>
          )}
        </div>
      ))}
    </dl>
  );
}
