"use client";

import { ChevronLeft, Plus } from "lucide-react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import type { Property } from "@/lib/api/generated/model";
import { orgPath } from "@/lib/org-path";
import { propertySummary } from "./utils";

type PropertyUnitsHeaderProps = {
  orgId: string;
  property: Property;
  // property:manage (allow or conditional): "Ubah properti".
  canEditProperty: boolean;
  // unit:manage (allow or conditional): "Tambah unit", the screen's one primary action.
  canAddUnit: boolean;
  onEditProperty: () => void;
  onAddUnit: () => void;
};

// The back link, the property's name and its summary line, "Ubah properti" and "Tambah unit". On
// iPad the buttons sit to the right of the title, "Tambah unit" on the right; on the phone they
// are full-width under it, "Tambah unit" on top. It comes first in the markup at both sizes, so
// the focus order starts with the screen's primary action.
export function PropertyUnitsHeader({
  orgId,
  property,
  canEditProperty,
  canAddUnit,
  onEditProperty,
  onAddUnit,
}: PropertyUnitsHeaderProps) {
  const showButtons = canEditProperty || canAddUnit;

  return (
    <header className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between lg:gap-5">
      <div className="flex flex-col gap-1.5">
        <Link
          href={orgPath(orgId, "properties")}
          className="-ml-1 inline-flex min-h-11 items-center gap-1 self-start rounded-md px-1 text-[15px] font-semibold text-primary hover:underline md:text-base"
        >
          <ChevronLeft aria-hidden="true" className="size-[18px]" />
          Properti
        </Link>
        <h1 className="-mt-1.5 text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
          {property.name}
        </h1>
        <p className="text-[15px] leading-normal text-muted-foreground md:text-[17px]">
          {propertySummary(property)}
        </p>
      </div>
      {showButtons && (
        <div className="flex shrink-0 flex-col gap-2.5 lg:flex-row-reverse lg:items-center lg:gap-2">
          {canAddUnit && (
            <Button
              type="button"
              onClick={onAddUnit}
              className="h-13 w-full rounded-xl text-[17px] font-semibold lg:h-12 lg:w-auto"
            >
              <Plus aria-hidden="true" className="size-5" strokeWidth={2.2} />
              Tambah unit
            </Button>
          )}
          {canEditProperty && (
            <Button
              type="button"
              variant="outline"
              onClick={onEditProperty}
              className="h-12 w-full rounded-xl border-[1.5px] bg-card text-base font-semibold lg:w-auto lg:px-[18px] lg:text-[17px]"
            >
              Ubah properti
            </Button>
          )}
        </div>
      )}
    </header>
  );
}
