"use client";

import type { Property } from "@/lib/api/generated/model";

import { PropertyCard } from "./property-card";
import { PropertyTableRow } from "./property-table-row";

type PropertyListProps = {
  orgId: string;
  properties: readonly Property[];
  canEdit: boolean;
  onEdit: (propertyId: string) => void;
};

// Every property the member may see, in the API's order: a table on iPad, cards on the phone
// (boards Properti.html and Properti-Phone.html). Only one of the two is displayed.
export function PropertyList({
  orgId,
  properties,
  canEdit,
  onEdit,
}: PropertyListProps) {
  return (
    <>
      <section className="hidden overflow-hidden rounded-2xl border bg-card lg:block">
        <div
          aria-hidden="true"
          className="grid min-h-11 grid-cols-[minmax(0,1fr)_100px_80px_200px_160px] items-center gap-4 border-b bg-muted/40 px-5 text-base font-semibold text-muted-foreground"
        >
          <span>Nama</span>
          <span>Jenis</span>
          <span>Unit</span>
          <span>Terisi</span>
          <span />
        </div>
        <ul aria-label="Daftar properti">
          {properties.map((property) => (
            <PropertyTableRow
              key={property.id}
              orgId={orgId}
              property={property}
              canEdit={canEdit}
              onEdit={onEdit}
            />
          ))}
        </ul>
      </section>

      <ul
        aria-label="Daftar properti"
        className="flex flex-col gap-2.5 lg:hidden"
      >
        {properties.map((property) => (
          <PropertyCard
            key={property.id}
            orgId={orgId}
            property={property}
            canEdit={canEdit}
            onEdit={onEdit}
          />
        ))}
      </ul>
    </>
  );
}
