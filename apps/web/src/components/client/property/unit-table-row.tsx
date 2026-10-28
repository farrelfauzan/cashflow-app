"use client";

import { Button } from "@/components/ui/button";
import type { Unit } from "@/lib/api/generated/model";
import { defaultRentText, unitOccupancyView } from "./utils";

import { UnitTenant } from "./unit-tenant";

type UnitTableRowProps = {
  unit: Unit;
  canManage: boolean;
  onEdit: (unitId: string) => void;
};

// One unit on iPad: label, usual rent, who is in it, and "Ubah".
export function UnitTableRow({ unit, canManage, onEdit }: UnitTableRowProps) {
  return (
    <li className="grid min-h-14 grid-cols-[minmax(0,1fr)_180px_290px_96px] items-center gap-4 border-t px-5 py-1.5 first:border-t-0">
      <span className="truncate text-[17px] font-semibold">{unit.label}</span>
      <span className="text-base text-muted-foreground">
        {defaultRentText(unit)}
      </span>
      <div className="min-w-0">
        <UnitTenant view={unitOccupancyView(unit)} className="text-base" />
      </div>
      <div className="flex justify-end">
        {canManage && (
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
      </div>
    </li>
  );
}
