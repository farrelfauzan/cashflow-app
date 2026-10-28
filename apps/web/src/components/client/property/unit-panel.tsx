"use client";

import { useState } from "react";

import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import type { Unit } from "@/lib/api/generated/model";
import type { OpenUnitPanel, UnitPanelState } from "./type";

import { UnitForm } from "./unit-form";

type UnitPanelProps = {
  orgId: string;
  propertyId: string;
  propertyName: string;
  panel: UnitPanelState;
  units: readonly Unit[];
  onPanelChange: (panel: UnitPanelState) => void;
  onNotice: (notice: string) => void;
};

// The side panel for "Tambah unit" and "Ubah unit" (board Properti-Unit-Form.html): from the right
// on iPad, the whole screen on the phone. Its save button is its primary action.
export function UnitPanel({
  orgId,
  propertyId,
  propertyName,
  panel,
  units,
  onPanelChange,
  onNotice,
}: UnitPanelProps) {
  // The last panel that was open stays rendered while the sheet slides away.
  const [shown, setShown] = useState<OpenUnitPanel | null>(null);
  if (panel.kind !== "closed" && panel !== shown) setShown(panel);

  const editing =
    shown?.kind === "edit"
      ? units.find((unit) => unit.id === shown.unitId)
      : undefined;
  const open =
    panel.kind !== "closed" && (panel.kind !== "edit" || editing !== undefined);
  const adding = shown?.kind === "add";
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
            {adding ? "Tambah unit" : "Ubah unit"}
          </SheetTitle>
          <SheetDescription className="text-base text-muted-foreground">
            {propertyName}
          </SheetDescription>
        </SheetHeader>
        {(adding || editing !== undefined) && (
          <UnitForm
            key={editing?.id ?? "add"}
            orgId={orgId}
            propertyId={propertyId}
            propertyName={propertyName}
            unit={editing}
            onSaved={(unit) => {
              onNotice(
                adding
                  ? `${unit.label} sudah ditambahkan.`
                  : `${unit.label} sudah disimpan.`,
              );
              close();
            }}
          />
        )}
      </SheetContent>
    </Sheet>
  );
}
