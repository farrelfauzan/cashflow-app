"use client";

import { useState } from "react";

import { FormSheet } from "@/components/client/form-sheet/form-sheet";
import { FormSheetContent } from "@/components/client/form-sheet/form-sheet-content";
import {
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import type { Property } from "@/lib/api/generated/model";
import type { OpenPropertyPanel, PropertyPanelState } from "./type";

import { PropertyForm } from "./property-form";

type PropertyPanelProps = {
  orgId: string;
  organizationName: string;
  panel: PropertyPanelState;
  properties: readonly Property[];
  onPanelChange: (panel: PropertyPanelState) => void;
  onNotice: (notice: string) => void;
};

// The side panel for "Tambah properti" and "Ubah properti" (board Properti-Form.html): from the
// right on iPad and desktop, a sheet from the bottom on the phone (board HP-Lembar). Its save button is its primary action.
export function PropertyPanel({
  orgId,
  organizationName,
  panel,
  properties,
  onPanelChange,
  onNotice,
}: PropertyPanelProps) {
  // The last panel that was open stays rendered while the sheet slides away.
  const [shown, setShown] = useState<OpenPropertyPanel | null>(null);
  if (panel.kind !== "closed" && panel !== shown) setShown(panel);

  const editing =
    shown?.kind === "edit"
      ? properties.find((property) => property.id === shown.propertyId)
      : undefined;
  const open =
    panel.kind !== "closed" && (panel.kind !== "edit" || editing !== undefined);
  const adding = shown?.kind === "add";
  const close = () => onPanelChange({ kind: "closed" });

  return (
    <FormSheet
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
    >
      <FormSheetContent>
        <SheetHeader className="gap-1 border-b px-6 pt-6 pr-16 pb-4">
          <SheetTitle className="text-2xl font-bold">
            {adding ? "Tambah properti" : "Ubah properti"}
          </SheetTitle>
          <SheetDescription className="text-base text-muted-foreground">
            {organizationName}
          </SheetDescription>
        </SheetHeader>
        {(adding || editing !== undefined) && (
          <PropertyForm
            key={editing?.id ?? "add"}
            orgId={orgId}
            property={editing}
            onSaved={(property) => {
              onNotice(
                adding
                  ? `${property.name} sudah ditambahkan.`
                  : `${property.name} sudah disimpan.`,
              );
              close();
            }}
          />
        )}
      </FormSheetContent>
    </FormSheet>
  );
}
