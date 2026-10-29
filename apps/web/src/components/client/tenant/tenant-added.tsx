"use client";

import { Check } from "lucide-react";

import { Button } from "@/components/ui/button";
import { TENANT_ADDED_LATER_TEXT, tenantAddedText } from "./utils";

type TenantAddedProps = {
  name: string;
  // lease:manage: "Sewakan unit" continues in the same panel.
  canManageLeases: boolean;
  onLater: () => void;
  onRent: () => void;
};

// "Penyewa ditambahkan" (board Penyewa-Tambah.dc.html): the tenant is saved without a lease;
// "Sewakan unit" goes on to the lease form, "Nanti saja" closes the panel.
export function TenantAdded({
  name,
  canManageLeases,
  onLater,
  onRent,
}: TenantAddedProps) {
  return (
    <>
      <div className="flex min-h-0 flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-7">
        <div
          role="status"
          className="flex items-start gap-3.5 rounded-[14px] bg-accent p-4"
        >
          <span
            aria-hidden="true"
            className="flex size-10 shrink-0 items-center justify-center rounded-full bg-primary text-primary-foreground"
          >
            <Check className="size-[22px]" strokeWidth={2.4} />
          </span>
          <div className="flex flex-col gap-1">
            <p className="text-lg font-bold text-primary">
              Penyewa ditambahkan
            </p>
            <p className="text-base leading-normal text-foreground">
              {tenantAddedText(name, canManageLeases)}
            </p>
          </div>
        </div>
        {canManageLeases && (
          <p className="text-base leading-[1.55] text-foreground/85">
            {TENANT_ADDED_LATER_TEXT}
          </p>
        )}
      </div>
      <div className="flex gap-2.5 border-t px-6 py-4">
        {canManageLeases ? (
          <>
            <Button
              type="button"
              variant="outline"
              onClick={onLater}
              className="h-13 flex-1 rounded-xl border-[1.5px] bg-card text-[17px] font-semibold"
            >
              Nanti saja
            </Button>
            <Button
              type="button"
              onClick={onRent}
              className="h-13 flex-2 rounded-xl px-3 text-[17px] font-semibold whitespace-normal"
            >
              Sewakan unit
            </Button>
          </>
        ) : (
          <Button
            type="button"
            onClick={onLater}
            className="h-13 flex-1 rounded-xl text-[17px] font-semibold"
          >
            Selesai
          </Button>
        )}
      </div>
    </>
  );
}
