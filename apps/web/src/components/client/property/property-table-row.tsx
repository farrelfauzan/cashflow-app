"use client";

import Link from "next/link";

import { Button } from "@/components/ui/button";
import type { Property } from "@/lib/api/generated/model";
import { orgPath } from "@/lib/org-path";
import { occupancyText, unitCountText } from "./utils";

import { PropertyKindChip } from "./property-kind-chip";

type PropertyTableRowProps = {
  orgId: string;
  property: Property;
  canEdit: boolean;
  onEdit: (propertyId: string) => void;
};

// One property on iPad: name and address, kind, unit count, occupancy, "Buka" and "Ubah".
export function PropertyTableRow({
  orgId,
  property,
  canEdit,
  onEdit,
}: PropertyTableRowProps) {
  const href = orgPath(orgId, `properties/${property.id}`);

  return (
    <li className="grid min-h-[72px] grid-cols-[minmax(0,1fr)_100px_80px_200px_160px] items-center gap-4 border-t px-5 py-2 first:border-t-0">
      <div className="flex min-w-0 flex-col gap-0.5">
        <Link
          href={href}
          className="truncate text-[17px] font-semibold hover:underline"
        >
          {property.name}
        </Link>
        <span className="truncate text-base text-muted-foreground">
          {property.address}
        </span>
      </div>
      <div>
        <PropertyKindChip kind={property.kind} />
      </div>
      <span className="text-base">{unitCountText(property)}</span>
      <span className="text-base text-muted-foreground">
        {occupancyText(property)}
      </span>
      <div className="flex items-center justify-end gap-1">
        <Button
          asChild
          variant="outline"
          className="rounded-xl border-[1.5px] bg-card px-4 text-base font-semibold"
        >
          <Link href={href} aria-label={`Buka unit ${property.name}`}>
            Buka
          </Link>
        </Button>
        {canEdit && (
          <Button
            type="button"
            variant="ghost"
            aria-label={`Ubah ${property.name}`}
            onClick={() => onEdit(property.id)}
            className="rounded-xl px-2.5 text-base font-semibold text-primary"
          >
            Ubah
          </Button>
        )}
      </div>
    </li>
  );
}
