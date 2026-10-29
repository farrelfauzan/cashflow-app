"use client";

import { FileUp, Plus } from "lucide-react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import { tenantsSubtitle } from "./utils";

type TenantsHeaderProps = {
  organizationName: string;
  // tenant:manage for the whole organization, and the list is not empty (the empty state then
  // carries the button): the screen's one primary action.
  canAdd: boolean;
  onAdd: () => void;
  // "Impor dari spreadsheet" (an outline button), to the import screen; null hides it.
  importHref: string | null;
};

// The title, what the screen is for, "Impor dari spreadsheet" and "Tambah penyewa" (boards
// Penyewa.dc.html and Penyewa-Tambah.dc.html). On iPad the buttons sit to the right of the title;
// on the phone they are full-width buttons under it.
export function TenantsHeader({
  organizationName,
  canAdd,
  onAdd,
  importHref,
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
      {(importHref !== null || canAdd) && (
        <div className="flex shrink-0 flex-col gap-2.5 lg:flex-row lg:items-center lg:gap-2">
          {importHref !== null && (
            <Button
              asChild
              variant="outline"
              className="h-12 w-full rounded-xl border-[1.5px] bg-card text-base font-semibold lg:w-auto lg:px-[18px] lg:text-[17px]"
            >
              <Link href={importHref}>
                <FileUp aria-hidden="true" className="size-5" strokeWidth={2} />
                Impor dari spreadsheet
              </Link>
            </Button>
          )}
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
        </div>
      )}
    </header>
  );
}
