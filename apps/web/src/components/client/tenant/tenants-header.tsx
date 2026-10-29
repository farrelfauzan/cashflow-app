"use client";

import { Plus } from "lucide-react";

import { Button } from "@/components/ui/button";
import { tenantsSubtitle } from "./utils";

type TenantsHeaderProps = {
  organizationName: string;
  // tenant:manage for the whole organization, and the list is not empty (the empty state then
  // carries the button): the screen's one primary action.
  canAdd: boolean;
  onAdd: () => void;
};

// The title, what the screen is for, and "Tambah penyewa" (boards Penyewa.dc.html and
// Penyewa-Tambah.dc.html). On iPad the button sits to the right of the title; on the phone it is
// a full-width button under it. The import button comes with the import screen.
export function TenantsHeader({
  organizationName,
  canAdd,
  onAdd,
}: TenantsHeaderProps) {
  return (
    <header className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between lg:gap-5">
      <div className="flex flex-col gap-1.5">
        <h1 className="text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
          Penyewa
        </h1>
        <p className="text-[15px] leading-normal text-muted-foreground md:text-[17px]">
          {tenantsSubtitle(organizationName)}
        </p>
      </div>
      {canAdd && (
        <Button
          type="button"
          onClick={onAdd}
          className="h-13 w-full shrink-0 rounded-xl text-[17px] font-semibold lg:h-12 lg:w-auto"
        >
          <Plus aria-hidden="true" className="size-5" strokeWidth={2.2} />
          Tambah penyewa
        </Button>
      )}
    </header>
  );
}
