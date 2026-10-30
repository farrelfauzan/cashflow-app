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

type UnitCardProps = {
  unit: Unit;
  action: UnitRowAction;
  onEdit: (unitId: string) => void;
  onShowPrices: (unitId: string) => void;
};

// One unit on the phone: label, usual rent, who is in it, and "Ubah" (or "Harga" for a member who
// may only read units).
export function UnitCard({
  unit,
  action,
  onEdit,
  onShowPrices,
}: UnitCardProps) {
  return (
    <li className="flex items-center justify-between gap-2.5 rounded-[14px] border bg-card py-3 pr-3.5 pl-4">
      <div className="flex min-w-0 flex-col gap-1">
        <span className="text-[17px] font-semibold">{unit.label}</span>
        <span className="text-sm text-muted-foreground">
          Sewa biasa {unitRentText(unit)}
        </span>
        <div>
          <UnitTenant view={unitOccupancyView(unit)} className="text-[15px]" />
        </div>
      </div>
      {action === "edit" && (
        <Button
          type="button"
          variant="outline"
          aria-label={`Ubah ${unit.label}`}
          onClick={() => onEdit(unit.id)}
          className="rounded-xl border-[1.5px] bg-card px-[18px] text-[15px] font-semibold"
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
          className="rounded-xl border-[1.5px] bg-card px-[18px] text-[15px] font-semibold"
        >
          {PRICES_BUTTON_TEXT}
        </Button>
      )}
    </li>
  );
}
