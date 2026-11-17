"use client";

import { XIcon } from "lucide-react";

import {
  CLOSE_LABEL,
  sheetSubtitle,
} from "@/components/client/field-cash/utils";
import { FormSheet } from "@/components/client/form-sheet/form-sheet";
import { FormSheetContent } from "@/components/client/form-sheet/form-sheet-content";
import { Button } from "@/components/ui/button";
import {
  SheetClose,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";

import type { RetakePayment } from "./type";
import { RETAKE_TITLE } from "./utils";

import { RetakePhotoForm } from "./retake-photo-form";
import { useRetakePhoto } from "./use-retake-photo";

type RetakePhotoSheetProps = {
  orgId: string;
  payment: RetakePayment;
  // "Tunai Rp1.000.000 · 10.42".
  summary: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

// "Ambil ulang foto kwitansi": a bottom sheet on the phone, the right-hand drawer from md up,
// like Terima tunai. The caller mounts it afresh for each opening. It cannot close while the
// photo is attached; on a phone, closing with a photo chosen asks first. It closes itself once
// the photo is attached (with a toast), or when the API says there is nothing left to retake.
export function RetakePhotoSheet({
  orgId,
  payment,
  summary,
  open,
  onOpenChange,
}: RetakePhotoSheetProps) {
  const retake = useRetakePhoto({
    orgId,
    payment,
    onClose: () => changeOpen(false),
  });
  const subtitle = sheetSubtitle(payment.tenantName, payment.place);

  function changeOpen(next: boolean) {
    if (!next) retake.finish();
    onOpenChange(next);
  }

  return (
    <FormSheet
      open={open}
      onOpenChange={changeOpen}
      dirty={retake.dirty}
      busy={retake.saving}
    >
      <FormSheetContent showCloseButton={false}>
        <SheetHeader className="flex-row items-start gap-3 border-b px-5 pt-5 pb-3 md:px-6 md:pt-6 md:pb-4">
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <SheetTitle className="text-xl leading-tight font-bold md:text-2xl">
              {RETAKE_TITLE}
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
              disabled={retake.saving}
              className="-mt-1.5 -mr-2 rounded-full"
            >
              <XIcon aria-hidden="true" className="size-5" />
            </Button>
          </SheetClose>
        </SheetHeader>
        <RetakePhotoForm
          retake={retake}
          summary={summary}
          subtitle={subtitle}
        />
      </FormSheetContent>
    </FormSheet>
  );
}
