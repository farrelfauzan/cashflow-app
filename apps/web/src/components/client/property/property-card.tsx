"use client";

import Link from "next/link";

import { Button } from "@/components/ui/button";
import type { Property } from "@/lib/api/generated/model";
import { orgPath } from "@/lib/organization/org-menu";
import { occupancyText, unitCountText } from "./utils";

import { PropertyKindChip } from "./property-kind-chip";

type PropertyCardProps = {
  orgId: string;
  property: Property;
  canEdit: boolean;
  onEdit: (propertyId: string) => void;
};

// One property on the phone: name, address and kind, how many units are taken, "Buka unit" and
// "Ubah".
export function PropertyCard({
  orgId,
  property,
  canEdit,
  onEdit,
}: PropertyCardProps) {
  const href = orgPath(orgId, `properties/${property.id}`);

  return (
    <li className="flex flex-col gap-2.5 rounded-[14px] border bg-card px-4 py-3.5">
      <div className="flex items-start justify-between gap-2.5">
        <div className="flex min-w-0 flex-col gap-0.5">
          <Link href={href} className="text-[17px] font-semibold">
            {property.name}
          </Link>
          <span className="text-sm text-muted-foreground">
            {property.address}
          </span>
        </div>
        <PropertyKindChip kind={property.kind} />
      </div>
      <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
        <span className="text-[15px] font-semibold">
          {unitCountText(property)}
        </span>
        <span className="text-sm text-muted-foreground">
          {occupancyText(property)}
        </span>
      </div>
      <div className="flex gap-2">
        <Button
          asChild
          variant="outline"
          className="flex-1 rounded-xl border-[1.5px] bg-card text-[15px] font-semibold"
        >
          <Link href={href} aria-label={`Buka unit ${property.name}`}>
            Buka unit
          </Link>
        </Button>
        {canEdit && (
          <Button
            type="button"
            variant="outline"
            aria-label={`Ubah ${property.name}`}
            onClick={() => onEdit(property.id)}
            className="rounded-xl border-[1.5px] bg-card px-5 text-[15px] font-semibold"
          >
            Ubah
          </Button>
        )}
      </div>
    </li>
  );
}
