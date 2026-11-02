"use client";

import { AccountTile } from "@/components/client/billing/account-tile";
import type { AccountTileView } from "@/components/client/billing/type";

type InvoiceTilesProps = {
  tiles: readonly AccountTileView[];
};

// "Terkumpul", "Sisa belum dibayar" and "Tunggakan tertua": three tiles side by side on iPad,
// stacked on the phone.
export function InvoiceTiles({ tiles }: InvoiceTilesProps) {
  return (
    <div className="grid grid-cols-1 gap-2.5 md:grid-cols-3 md:gap-3">
      {tiles.map((tile) => (
        <AccountTile key={tile.label} tile={tile} />
      ))}
    </div>
  );
}
