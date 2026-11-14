"use client";

import { Button } from "@/components/ui/button";
import { SheetClose } from "@/components/ui/sheet";
import { cn } from "@/lib/utils";

import type { LeasePanelSubmitVariant } from "./type";

type LeasePanelFooterProps = {
  // The form the main button submits, through its `form` attribute.
  formId: string;
  cancelLabel: string;
  submitLabel: string;
  saving: boolean;
  // The wide panel puts both buttons at the right on iPad; the narrow one fills the row.
  wide: boolean;
  // The main button's look by meaning: "Simpan rencana keluar" is caution, "Batalkan sewa" and
  // "Catat pindah keluar" are destructive; the cancel button stays neutral.
  submitVariant?: LeasePanelSubmitVariant;
};

// The footer of a lease panel: a button that closes the panel, and the panel's one main action.
// The main button is disabled while saving, so a form is never sent twice.
export function LeasePanelFooter({
  formId,
  cancelLabel,
  submitLabel,
  saving,
  wide,
  submitVariant = "default",
}: LeasePanelFooterProps) {
  return (
    <div
      data-slot="form-sheet-footer"
      className={cn(
        "flex gap-2.5 border-t px-6 py-4",
        wide && "md:justify-end",
      )}
    >
      <SheetClose asChild>
        <Button
          type="button"
          variant="outline"
          className={cn(
            "h-13 flex-1 rounded-xl border-[1.5px] bg-card text-[17px] font-semibold",
            wide && "md:min-w-[140px] md:flex-none",
          )}
        >
          {cancelLabel}
        </Button>
      </SheetClose>
      <Button
        type="submit"
        form={formId}
        disabled={saving}
        variant={submitVariant}
        className={cn(
          "h-13 flex-2 rounded-xl px-3 text-base font-semibold whitespace-normal md:text-[17px]",
          wide && "md:min-w-60 md:flex-none",
        )}
      >
        {saving ? "Menyimpan…" : submitLabel}
      </Button>
    </div>
  );
}
