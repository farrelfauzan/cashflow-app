"use client";

import { Button } from "@/components/ui/button";
import { SheetClose } from "@/components/ui/sheet";
import { cn } from "@/lib/utils";

import { CLOSE_LABEL, SAVING_LABEL } from "./utils";

type CheckSheetFooterProps = {
  // Read-only (the Supervisor): one "Tutup", no action.
  readOnly: boolean;
  secondaryLabel: string;
  // Without it, the secondary button closes the drawer.
  onSecondary?: () => void;
  primaryLabel: string;
  // The form the primary button submits; without it, onPrimary runs.
  formId?: string;
  onPrimary?: () => void;
  // The request went out (or succeeded): the primary says "Menyimpan…" and is disabled.
  saving: boolean;
  // The request is running: nothing in the footer can be pressed.
  pending: boolean;
  // The primary cannot go yet (a photo still being scanned), whatever else is the case.
  primaryDisabled?: boolean;
  // A long secondary label ("Bukan pembayaran"): on the phone both buttons share the width
  // equally, so neither label breaks.
  balanced?: boolean;
};

// The drawer's footer: a secondary button and the one primary action, or "Tutup" alone for a
// member who may only read.
export function CheckSheetFooter({
  readOnly,
  secondaryLabel,
  onSecondary,
  primaryLabel,
  formId,
  onPrimary,
  saving,
  pending,
  primaryDisabled = false,
  balanced = false,
}: CheckSheetFooterProps) {
  const secondaryClass =
    "h-13 flex-1 rounded-xl border-[1.5px] bg-card px-3 text-base font-semibold whitespace-nowrap md:text-[17px]";
  if (readOnly) {
    return (
      <SheetClose asChild>
        <Button type="button" variant="outline" className={secondaryClass}>
          {CLOSE_LABEL}
        </Button>
      </SheetClose>
    );
  }
  const secondary = (
    <Button
      type="button"
      variant="outline"
      disabled={pending}
      onClick={onSecondary}
      className={secondaryClass}
    >
      {secondaryLabel}
    </Button>
  );
  return (
    <>
      {onSecondary === undefined ? (
        <SheetClose asChild>{secondary}</SheetClose>
      ) : (
        secondary
      )}
      <Button
        type={formId === undefined ? "button" : "submit"}
        form={formId}
        onClick={onPrimary}
        disabled={saving || primaryDisabled}
        className={cn(
          "h-13 rounded-xl px-3 text-base font-semibold whitespace-normal md:text-[17px]",
          balanced ? "flex-1 md:flex-[1.6]" : "flex-[1.6]",
        )}
      >
        {saving ? SAVING_LABEL : primaryLabel}
      </Button>
    </>
  );
}
