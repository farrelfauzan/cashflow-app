"use client";

import { useId, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import type { Unit } from "@/lib/api/generated/model";
import { jakartaToday } from "@/lib/format/date";
import type { UnitPricesPanelState } from "./type";
import { TERMS_LEGEND, UNIT_PRICES_TITLE, unitPriceTermViews } from "./utils";

import { UnitPriceHistory } from "./unit-price-history";
import { UnitPriceTermRow } from "./unit-price-term-row";

type UnitPricesPanelProps = {
  orgId: string;
  propertyName: string;
  panel: UnitPricesPanelState;
  units: readonly Unit[];
  onPanelChange: (panel: UnitPricesPanelState) => void;
};

// "Harga unit", read-only, for a member who may read units but not change them (the Supervisor):
// the unit's terms with their prices, then "Riwayat harga". Same side panel as the unit's form,
// from the right on iPad and the whole screen on the phone.
export function UnitPricesPanel({
  orgId,
  propertyName,
  panel,
  units,
  onPanelChange,
}: UnitPricesPanelProps) {
  const id = useId();
  // The last unit shown stays rendered while the sheet slides away.
  const [shownId, setShownId] = useState<string | null>(null);
  const [today, setToday] = useState(() => jakartaToday(new Date()));
  if (panel.kind === "open" && panel.unitId !== shownId) {
    setShownId(panel.unitId);
    setToday(jakartaToday(new Date()));
  }

  const unit = units.find((candidate) => candidate.id === shownId);
  const open = panel.kind === "open" && unit !== undefined;

  return (
    <Sheet
      open={open}
      onOpenChange={(next) => {
        if (!next) onPanelChange({ kind: "closed" });
      }}
    >
      <SheetContent
        side="right"
        className="w-full gap-0 bg-card p-0 sm:max-w-none md:w-[470px]"
      >
        <SheetHeader className="gap-1 border-b px-6 pt-6 pr-16 pb-4">
          <SheetTitle className="text-2xl font-bold">
            {UNIT_PRICES_TITLE}
          </SheetTitle>
          <SheetDescription className="text-base text-muted-foreground">
            {unit === undefined
              ? propertyName
              : `${unit.label} · ${propertyName}`}
          </SheetDescription>
        </SheetHeader>
        {unit !== undefined && (
          <div className="flex min-h-0 flex-1 flex-col gap-[22px] overflow-y-auto px-6 py-5">
            <section
              aria-labelledby={`${id}-terms`}
              className="flex flex-col gap-2"
            >
              <h3
                id={`${id}-terms`}
                className="text-[15px] leading-normal font-semibold md:text-base"
              >
                {TERMS_LEGEND}
              </h3>
              <ul
                aria-label={TERMS_LEGEND}
                className="flex flex-col rounded-xl border-[1.5px] bg-card px-3.5"
              >
                {unitPriceTermViews(unit).map((term) => (
                  <UnitPriceTermRow key={term.months} term={term} />
                ))}
              </ul>
            </section>
            <UnitPriceHistory orgId={orgId} unitId={unit.id} today={today} />
          </div>
        )}
        <div className="flex border-t px-6 py-4">
          <SheetClose asChild>
            <Button
              type="button"
              variant="outline"
              className="h-13 flex-1 rounded-xl border-[1.5px] bg-card text-[17px] font-semibold"
            >
              Tutup
            </Button>
          </SheetClose>
        </div>
      </SheetContent>
    </Sheet>
  );
}
