"use client";

import type { Unit } from "@/lib/api/generated/model";
import type { UnitRowAction } from "./type";

import { UnitCard } from "./unit-card";
import { UnitTableRow } from "./unit-table-row";

type UnitListProps = {
  units: readonly Unit[];
  action: UnitRowAction;
  onEdit: (unitId: string) => void;
  onShowPrices: (unitId: string) => void;
};

// The units of one property, in the API's order: a table on iPad, cards on the phone (boards
// Properti-Unit.html and Properti-Unit-Phone.html). Only one of the two is displayed.
export function UnitList({
  units,
  action,
  onEdit,
  onShowPrices,
}: UnitListProps) {
  return (
    <>
      <section className="hidden overflow-hidden rounded-2xl border bg-card lg:block">
        <div
          aria-hidden="true"
          className="grid min-h-11 grid-cols-[minmax(0,1fr)_180px_290px_96px] items-center gap-4 border-b bg-muted/40 px-5 text-base font-semibold text-muted-foreground"
        >
          <span>Unit</span>
          <span>Sewa biasa</span>
          <span>Penyewa sekarang</span>
          <span />
        </div>
        <ul aria-label="Daftar unit">
          {units.map((unit) => (
            <UnitTableRow
              key={unit.id}
              unit={unit}
              action={action}
              onEdit={onEdit}
              onShowPrices={onShowPrices}
            />
          ))}
        </ul>
      </section>

      <ul aria-label="Daftar unit" className="flex flex-col gap-2.5 lg:hidden">
        {units.map((unit) => (
          <UnitCard
            key={unit.id}
            unit={unit}
            action={action}
            onEdit={onEdit}
            onShowPrices={onShowPrices}
          />
        ))}
      </ul>
    </>
  );
}
