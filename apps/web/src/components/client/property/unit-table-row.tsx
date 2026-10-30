"use client";

import { Button } from "@/components/ui/button";
import type { Unit } from "@/lib/api/generated/model";
import type { UnitRowAction } from "./type";
import {
  PRICES_BUTTON_TEXT,
  pricesButtonLabel,
  unitRentText,
  unitOccupancyView,
} from "./utils";

import { UnitTenant } from "./unit-tenant";

type UnitTableRowProps = {
  unit: Unit;
  action: UnitRowAction;
  onEdit: (unitId: string) => void;
  onShowPrices: (unitId: string) => void;
};

// One unit on iPad: label, usual rent, who is in it, and "Ubah" (or "Harga" for a member who may
// only read units).
export function UnitTableRow({
  unit,
  action,
  onEdit,
  onShowPrices,
}: UnitTableRowProps) {
  return (
    <li className="grid min-h-14 grid-cols-[minmax(0,1fr)_180px_290px_96px] items-center gap-4 border-t px-5 py-1.5 first:border-t-0">
      <span className="truncate text-[17px] font-semibold">{unit.label}</span>
      <span className="text-base text-muted-foreground">
        {unitRentText(unit)}
      </span>
      <div className="min-w-0">
        <UnitTenant view={unitOccupancyView(unit)} className="text-base" />
      </div>
      <div className="flex justify-end">
        {action === "edit" && (
          <Button
            type="button"
            variant="outline"
            aria-label={`Ubah ${unit.label}`}
            onClick={() => onEdit(unit.id)}
            className="rounded-xl border-[1.5px] bg-card px-[18px] text-[15px] font-semibold md:text-base"
          >
            Ubah
          </Button>
        )}
        {action === "prices" && (
          <Button
            type="button"
            variant="outline"
            aria-label={pricesButtonLabel(unit.label)}
            onClick={() => onShowPrices(unit.id)}
            className="rounded-xl border-[1.5px] bg-card px-[18px] text-[15px] font-semibold md:text-base"
          >
            {PRICES_BUTTON_TEXT}
          </Button>
        )}
      </div>
    </li>
  );
}
