"use client";

import { Lock } from "lucide-react";

import { TenantChip } from "@/components/client/tenant/tenant-chip";
import { Button } from "@/components/ui/button";
import type { RecordedMeterRowView } from "./type";
import { EDIT_TEXT, LOCKED_CHIP, UNBILLED_CHIP } from "./utils";

type MeterRecordedRowProps = {
  row: RecordedMeterRowView;
  // "Ubah" on an amount not billed yet, or the whole row of a locked one.
  onOpen: () => void;
};

// One recorded amount: "Listrik · Oktober 2026 · Rp123.000" with "Belum ditagihkan" and "Ubah",
// or "Sudah ditagihkan, terkunci", where the whole row opens the locked view.
export function MeterRecordedRow({ row, onOpen }: MeterRecordedRowProps) {
  if (row.locked) {
    return (
      <li className="border-t first:border-t-0">
        <button
          type="button"
          onClick={onOpen}
          className="flex min-h-11 w-full flex-wrap items-center justify-between gap-x-3 gap-y-1.5 py-3 text-left outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
        >
          <span className="text-[15px] font-semibold md:text-base">
            {row.text}
          </span>
          <TenantChip tone="neutral">
            <Lock aria-hidden="true" className="mr-1.5 size-4" />
            {LOCKED_CHIP}
          </TenantChip>
        </button>
      </li>
    );
  }
  return (
    <li className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1.5 border-t py-2 first:border-t-0">
      <span className="text-[15px] font-semibold md:text-base">{row.text}</span>
      <span className="flex items-center gap-2">
        <TenantChip tone="warning">{UNBILLED_CHIP}</TenantChip>
        <Button
          type="button"
          variant="ghost"
          aria-label={`${EDIT_TEXT} ${row.name}`}
          onClick={onOpen}
          className="h-11 px-3 text-[15px] font-semibold text-primary md:text-base"
        >
          {EDIT_TEXT}
        </Button>
      </span>
    </li>
  );
}
