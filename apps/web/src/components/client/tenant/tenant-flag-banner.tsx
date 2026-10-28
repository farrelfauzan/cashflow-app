"use client";

import { TriangleAlert } from "lucide-react";

import { Button } from "@/components/ui/button";
import { flagBannerTitle } from "./utils";

type TenantFlagBannerProps = {
  count: number;
  // The list is narrowed to the flagged tenants.
  showing: boolean;
  onShow: () => void;
};

// The admin's to-do: tenants whose collector was disabled. "Tampilkan" narrows the list to them.
export function TenantFlagBanner({
  count,
  showing,
  onShow,
}: TenantFlagBannerProps) {
  return (
    <div
      role="status"
      className="flex flex-col gap-3 rounded-[14px] border border-warning/40 bg-warning/10 px-4 py-3.5 md:flex-row md:items-center md:gap-3.5 md:px-[18px]"
    >
      <TriangleAlert
        aria-hidden="true"
        className="hidden size-6 shrink-0 text-warning md:block"
      />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <span className="text-base font-bold md:text-[17px]">
          {flagBannerTitle(count)}
        </span>
        <p className="text-[15px] leading-normal text-foreground/85 md:text-base">
          Penagih mereka sudah dinonaktifkan. Buka sewa penyewa itu, lalu pilih
          penagih baru.
        </p>
      </div>
      <Button
        type="button"
        variant="outline"
        aria-label="Tampilkan penyewa yang belum punya penagih"
        aria-pressed={showing}
        onClick={onShow}
        className="self-start rounded-xl border-[1.5px] bg-card px-[18px] text-[15px] font-semibold md:self-auto"
      >
        Tampilkan
      </Button>
    </div>
  );
}
