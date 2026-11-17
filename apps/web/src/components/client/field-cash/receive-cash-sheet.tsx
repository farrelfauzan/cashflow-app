"use client";

import { XIcon } from "lucide-react";
import { useEffect } from "react";

import { FormSheet } from "@/components/client/form-sheet/form-sheet";
import { FormSheetContent } from "@/components/client/form-sheet/form-sheet-content";
import { Button } from "@/components/ui/button";
import {
  SheetClose,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";

import type { ReceiveCashTenant } from "./type";
import {
  CLOSE_LABEL,
  RECEIVE_CASH_TITLE,
  cashRecordedView,
  sheetSubtitle,
} from "./utils";

import { CashRecorded } from "./cash-recorded";
import { ReceiveCashForm } from "./receive-cash-form";
import { useReceiveCash } from "./use-receive-cash";

type ReceiveCashSheetProps = {
  orgId: string;
  tenant: ReceiveCashTenant;
  // The row's place line ("Kos Melati 11"), repeated in the subtitle.
  placeLabel: string;
  // This opening's requestId; the caller mounts the sheet afresh for each one.
  requestId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // Whether this opening may have recorded the cash without the app knowing (its answer was
  // lost): the caller then reopens this same sheet instead of a fresh one, so Simpan resends the
  // identical request and the cash is recorded once.
  onUnsureChange: (unsure: boolean) => void;
};

// "Terima tunai" (boards HP-Tagihan and HP-Foto): a bottom sheet on the phone, the right-hand
// drawer from md up. It cannot close while the cash is recorded; Batal while the photo is sent
// stops the upload and records nothing. On a phone, closing with an amount typed or a photo
// chosen asks first. Once recorded it shows "Tersimpan" until "Selesai".
export function ReceiveCashSheet({
  orgId,
  tenant,
  placeLabel,
  requestId,
  open,
  onOpenChange,
  onUnsureChange,
}: ReceiveCashSheetProps) {
  const cash = useReceiveCash({ orgId, tenantId: tenant.tenantId, requestId });
  useEffect(() => {
    onUnsureChange(cash.locked);
  }, [cash.locked, onUnsureChange]);
  const subtitle = sheetSubtitle(tenant.tenantName, placeLabel);
  const recordedPhoto =
    cash.photo.state.kind === "sent" ? cash.photo.state.photo : null;

  function changeOpen(next: boolean) {
    if (!next) cash.finish();
    onOpenChange(next);
  }

  return (
    <FormSheet
      open={open}
      onOpenChange={changeOpen}
      // After a lost answer the opening is kept (locked): the row's button brings it back with
      // its requestId, so closing loses nothing and does not ask "Tutup tanpa menyimpan?".
      dirty={cash.dirty && !cash.locked}
      busy={cash.saving}
    >
      <FormSheetContent
        showCloseButton={false}
        onOpenAutoFocus={(event) => {
          if (cash.result !== null) return;
          event.preventDefault();
          cash.form.setFocus("amount");
        }}
      >
        <SheetHeader className="flex-row items-start gap-3 border-b px-5 pt-5 pb-3 md:px-6 md:pt-6 md:pb-4">
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <SheetTitle className="text-xl leading-tight font-bold md:text-2xl">
              {RECEIVE_CASH_TITLE}
            </SheetTitle>
            <SheetDescription className="text-sm text-muted-foreground md:text-base">
              {subtitle}
            </SheetDescription>
          </div>
          <SheetClose asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={CLOSE_LABEL}
              disabled={cash.saving}
              className="-mt-1.5 -mr-2 rounded-full"
            >
              <XIcon aria-hidden="true" className="size-5" />
            </Button>
          </SheetClose>
        </SheetHeader>
        {cash.result === null ? (
          <ReceiveCashForm cash={cash} tenant={tenant} subtitle={subtitle} />
        ) : (
          <CashRecorded
            view={cashRecordedView(cash.result, tenant.tenantName)}
            photo={recordedPhoto}
            onDone={() => changeOpen(false)}
          />
        )}
      </FormSheetContent>
    </FormSheet>
  );
}
