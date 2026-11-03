"use client";

import { TriangleAlert } from "lucide-react";

import { Button } from "@/components/ui/button";
import { METER_FLAG_HINT, METER_SHOW_LABEL } from "./utils";

type MeterFlagBoxProps = {
  // "Meteran Oktober 2026 belum dicatat untuk 3 sewa".
  title: string;
  onShow: () => void;
};

// The admin's to-do on Penagihan: leases whose meter amount for last month is not recorded yet.
// It says the invoices go out anyway. "Tampilkan" opens the list of those leases.
export function MeterFlagBox({ title, onShow }: MeterFlagBoxProps) {
  return (
    <div
      role="status"
      className="flex flex-col gap-3 rounded-[14px] border border-warning/40 bg-warning/10 px-4 py-3.5 md:flex-row md:items-center md:gap-3.5 md:px-[18px]"
    >
      <TriangleAlert
        aria-hidden="true"
        className="size-6 shrink-0 text-warning"
      />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <span className="text-base font-bold md:text-[17px]">{title}</span>
        <p className="text-[15px] leading-normal text-foreground/85 md:text-base">
          {METER_FLAG_HINT}
        </p>
      </div>
      <Button
        type="button"
        variant="outline"
        onClick={onShow}
        className="h-11 w-full rounded-xl border-[1.5px] bg-card px-[18px] text-[15px] font-semibold md:w-auto"
      >
        {METER_SHOW_LABEL}
      </Button>
    </div>
  );
}
