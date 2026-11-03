"use client";

import { ChevronLeft, Plus } from "lucide-react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import { formatPhone } from "@/lib/format/phone";
import { orgPath } from "@/lib/org-path";

type TenantDetailHeaderProps = {
  orgId: string;
  name: string;
  // E.164, as the API sends it.
  phone: string;
  // tenant:manage (allow or conditional): "Ubah penyewa".
  canEdit: boolean;
  onEdit: () => void;
  // payment:record (allow or conditional): "Catat pembayaran", the page's one primary action.
  canRecordPayment: boolean;
  onRecordPayment: () => void;
};

// The back link, the tenant's name and their number, "Catat pembayaran" (the page's one primary
// action) and "Ubah penyewa" (an outline button). The board's "WhatsApp aktif" is left
// out: the API has no such field.
export function TenantDetailHeader({
  orgId,
  name,
  phone,
  canEdit,
  onEdit,
  canRecordPayment,
  onRecordPayment,
}: TenantDetailHeaderProps) {
  return (
    <header className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between lg:gap-5">
      <div className="flex flex-col gap-1.5">
        <Link
          href={orgPath(orgId, "tenants")}
          className="-ml-1 inline-flex min-h-11 items-center gap-1 self-start rounded-md pr-2 text-[15px] font-semibold text-primary hover:underline focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none md:text-base"
        >
          <ChevronLeft
            aria-hidden="true"
            className="size-[18px]"
            strokeWidth={2.2}
          />
          Penyewa
        </Link>
        <h1 className="text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
          {name}
        </h1>
        <p className="text-[15px] leading-normal text-muted-foreground md:text-[17px]">
          {formatPhone(phone)}
        </p>
      </div>
      {(canEdit || canRecordPayment) && (
        <div className="flex shrink-0 flex-col gap-2.5 lg:flex-row-reverse">
          {canRecordPayment && (
            <Button
              type="button"
              onClick={onRecordPayment}
              className="h-12 w-full rounded-xl px-5 text-[17px] font-semibold lg:w-auto"
            >
              <Plus aria-hidden="true" className="size-5" strokeWidth={2.2} />
              Catat pembayaran
            </Button>
          )}
          {canEdit && (
            <Button
              type="button"
              variant="outline"
              onClick={onEdit}
              className="h-12 w-full rounded-xl border-[1.5px] bg-card text-base font-semibold lg:w-auto lg:px-[18px] lg:text-[17px]"
            >
              Ubah penyewa
            </Button>
          )}
        </div>
      )}
    </header>
  );
}
