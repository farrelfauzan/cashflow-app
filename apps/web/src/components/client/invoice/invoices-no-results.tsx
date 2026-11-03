"use client";

import { Button } from "@/components/ui/button";
import { CLEAR_FILTERS_LABEL, NO_MATCH_HINT, NO_MATCH_TITLE } from "./utils";

type InvoicesNoResultsProps = {
  onReset: () => void;
};

// The search and filters match no invoice. "Hapus filter" takes all of them off.
export function InvoicesNoResults({ onReset }: InvoicesNoResultsProps) {
  return (
    <div className="flex flex-col items-center gap-2.5 rounded-[14px] border bg-card px-5 py-8 text-center md:gap-3 md:rounded-2xl md:px-7 md:py-11">
      <h2 className="text-xl font-bold md:text-[22px]">{NO_MATCH_TITLE}</h2>
      <p className="max-w-[440px] text-[15px] leading-normal text-muted-foreground md:text-[17px]">
        {NO_MATCH_HINT}
      </p>
      <Button
        type="button"
        variant="outline"
        onClick={onReset}
        className="h-12 w-full rounded-xl border-[1.5px] bg-card px-[18px] text-base font-semibold md:w-auto md:text-[17px]"
      >
        {CLEAR_FILTERS_LABEL}
      </Button>
    </div>
  );
}
