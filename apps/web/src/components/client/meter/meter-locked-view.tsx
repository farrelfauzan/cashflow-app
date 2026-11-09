"use client";

import { Lock } from "lucide-react";

import { TenantChip } from "@/components/client/tenant/tenant-chip";
import { Button } from "@/components/ui/button";
import type { LockedMeterView } from "./type";
import { LOCKED_CHIP } from "./utils";

type MeterLockedViewProps = {
  view: LockedMeterView;
  // "Koreksi tagihan November 2026"; null hides it (no correction:create).
  onCorrect: (() => void) | null;
  // "Kembali": back to the form (not "Tutup", which is the panel's own close button).
  onClose: () => void;
};

// "Meteran sudah ditagihkan": a billed amount, read only, with the invoice to correct instead.
export function MeterLockedView({
  view,
  onCorrect,
  onClose,
}: MeterLockedViewProps) {
  return (
    <>
      <div className="flex min-h-0 flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-5">
        <div className="flex flex-col items-start gap-2.5">
          <TenantChip tone="neutral">
            <Lock aria-hidden="true" className="mr-1.5 size-4" />
            {LOCKED_CHIP}
          </TenantChip>
          <h3 className="text-xl font-bold">{view.heading}</h3>
        </div>
        <dl className="flex flex-col gap-3 rounded-[14px] border bg-card px-4 py-3.5">
          {view.rows.map((row) => (
            <div
              key={row.label}
              className="flex flex-wrap items-baseline justify-between gap-x-3"
            >
              <dt className="text-[15px] font-semibold text-muted-foreground md:text-base">
                {row.label}
              </dt>
              <dd className="text-base font-bold md:text-[17px]">
                {row.value}
              </dd>
            </div>
          ))}
        </dl>
        <p className="text-[15px] leading-normal md:text-base">{view.text}</p>
      </div>
      <div className="flex gap-2.5 border-t px-6 py-4">
        {onCorrect !== null && (
          <Button
            type="button"
            variant="caution-outline"
            onClick={onCorrect}
            className="h-13 flex-1 rounded-xl border-[1.5px] bg-card px-3 text-base font-semibold whitespace-normal md:text-[17px]"
          >
            {view.correctLabel}
          </Button>
        )}
        <Button
          type="button"
          onClick={onClose}
          className="h-13 flex-1 rounded-xl text-base font-semibold md:text-[17px]"
        >
          Kembali
        </Button>
      </div>
    </>
  );
}
