"use client";

import { useState } from "react";

import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import type { PropertyKind, Unit } from "@/lib/api/generated/model";
import { jakartaToday } from "@/lib/format/date";
import { cn } from "@/lib/utils";
import type { OpenUnitPanel, UnitPanelState, UnitPanelView } from "./type";
import { priceSavedNotice } from "./utils";

import { UnitForm } from "./unit-form";
import { UnitPriceForm } from "./unit-price-form";

type UnitPanelProps = {
  orgId: string;
  propertyId: string;
  propertyName: string;
  propertyKind: PropertyKind;
  panel: UnitPanelState;
  units: readonly Unit[];
  onPanelChange: (panel: UnitPanelState) => void;
  onNotice: (notice: string) => void;
};

// The side panel for "Tambah unit" and "Ubah unit" (board Properti-Unit-Form.html): from the right
// on iPad, the whole screen on the phone. Its save button is its primary action. "Ubah harga"
// replaces the unit's form inside the same panel; the unit's form stays mounted, hidden, so
// "Kembali" finds what was typed there.
export function UnitPanel({
  orgId,
  propertyId,
  propertyName,
  propertyKind,
  panel,
  units,
  onPanelChange,
  onNotice,
}: UnitPanelProps) {
  // The last panel that was open stays rendered while the sheet slides away.
  const [shown, setShown] = useState<OpenUnitPanel | null>(null);
  const [view, setView] = useState<UnitPanelView>({ kind: "unit" });
  const [priceNotice, setPriceNotice] = useState("");
  const [today, setToday] = useState(() => jakartaToday(new Date()));
  if (panel.kind !== "closed" && panel !== shown) {
    setShown(panel);
    setView({ kind: "unit" });
    setPriceNotice("");
    setToday(jakartaToday(new Date()));
  }

  const editing =
    shown?.kind === "edit"
      ? units.find((unit) => unit.id === shown.unitId)
      : undefined;
  const open =
    panel.kind !== "closed" && (panel.kind !== "edit" || editing !== undefined);
  const adding = shown?.kind === "add";
  const pricing = view.kind === "price" && editing !== undefined;
  const close = () => onPanelChange({ kind: "closed" });

  return (
    <Sheet
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
    >
      <SheetContent
        side="right"
        className="w-full gap-0 bg-card p-0 sm:max-w-none md:w-[470px]"
      >
        <SheetHeader className="gap-1 border-b px-6 pt-6 pr-16 pb-4">
          <SheetTitle className="text-2xl font-bold">
            {pricing ? "Ubah harga" : adding ? "Tambah unit" : "Ubah unit"}
          </SheetTitle>
          <SheetDescription className="text-base text-muted-foreground">
            {pricing ? `${editing.label} · ${propertyName}` : propertyName}
          </SheetDescription>
        </SheetHeader>
        {(adding || editing !== undefined) && (
          <div
            className={cn(
              "min-h-0 flex-1 flex-col",
              pricing ? "hidden" : "flex",
            )}
          >
            <UnitForm
              key={editing?.id ?? "add"}
              orgId={orgId}
              propertyId={propertyId}
              propertyName={propertyName}
              propertyKind={propertyKind}
              unit={editing}
              today={today}
              notice={priceNotice}
              onSaved={(unit) => {
                onNotice(
                  adding
                    ? `${unit.label} sudah ditambahkan.`
                    : `${unit.label} sudah disimpan.`,
                );
                close();
              }}
              onChangePrice={(months) => {
                setPriceNotice("");
                setView({ kind: "price", months });
              }}
            />
          </div>
        )}
        {pricing && (
          <UnitPriceForm
            orgId={orgId}
            propertyId={propertyId}
            unit={editing}
            months={view.months}
            today={today}
            onSaved={(price) => {
              setPriceNotice(priceSavedNotice(price));
              setView({ kind: "unit" });
            }}
            onBack={() => setView({ kind: "unit" })}
          />
        )}
      </SheetContent>
    </Sheet>
  );
}
