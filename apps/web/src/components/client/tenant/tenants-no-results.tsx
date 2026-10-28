"use client";

import { Button } from "@/components/ui/button";

type TenantsNoResultsProps = {
  onReset: () => void;
};

// The search and filters match nobody. "Hapus pencarian" takes all of them off.
export function TenantsNoResults({ onReset }: TenantsNoResultsProps) {
  return (
    <div className="flex flex-col items-center gap-2.5 rounded-[14px] border bg-card px-5 py-8 text-center md:gap-3 md:rounded-2xl md:px-7 md:py-11">
      <h2 className="text-xl font-bold md:text-[22px]">
        Tidak ada penyewa yang cocok
      </h2>
      <p className="max-w-[440px] text-[15px] leading-normal text-muted-foreground md:text-[17px]">
        Coba kata lain, atau pilih Semua properti dan Semua status.
      </p>
      <Button
        type="button"
        variant="outline"
        onClick={onReset}
        className="h-12 w-full rounded-xl border-[1.5px] bg-card px-[18px] text-base font-semibold md:w-auto md:text-[17px]"
      >
        Hapus pencarian
      </Button>
    </div>
  );
}
