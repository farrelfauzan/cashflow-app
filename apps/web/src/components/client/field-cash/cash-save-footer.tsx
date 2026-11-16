"use client";

import { Button } from "@/components/ui/button";
import { SheetClose } from "@/components/ui/sheet";

import { CANCEL_TEXT } from "./utils";

type CashSaveFooterProps = {
  formId: string;
  // "Simpan Rp1.000.000", "Menunggu foto…" or "Menyimpan…".
  label: string;
  canSave: boolean;
  // recordCashPayment is running: Batal waits too.
  saving: boolean;
};

// The sheet's footer: Batal, which stays possible while the photo is sent (nothing is recorded),
// and the one primary, Simpan, which works only once the photo is sent.
export function CashSaveFooter({
  formId,
  label,
  canSave,
  saving,
}: CashSaveFooterProps) {
  return (
    <div
      data-slot="form-sheet-footer"
      className="flex gap-2.5 border-t px-4 py-3 md:px-6 md:py-4"
    >
      <SheetClose asChild>
        <Button
          type="button"
          variant="outline"
          disabled={saving}
          className="h-12 flex-1 rounded-xl border-[1.5px] bg-card text-base font-semibold md:h-13 md:text-[17px]"
        >
          {CANCEL_TEXT}
        </Button>
      </SheetClose>
      <Button
        type="submit"
        form={formId}
        disabled={!canSave}
        className="h-12 flex-2 rounded-xl px-3 text-base font-bold tabular-nums md:h-13 md:text-[17px]"
      >
        {label}
      </Button>
    </div>
  );
}
