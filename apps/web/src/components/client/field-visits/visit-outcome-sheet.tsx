"use client";

import { useAbility } from "@casl/react";
import { XIcon } from "lucide-react";
import { useEffect, useId } from "react";

import { CashSaveFooter } from "@/components/client/field-cash/cash-save-footer";
import {
  CLOSE_LABEL,
  canRecordCash,
  sheetSubtitle,
} from "@/components/client/field-cash/utils";
import { canCreatePromise } from "@/components/client/billing/utils";
import { owedText, placeText } from "@/components/client/field-today/utils";
import { FormSheet } from "@/components/client/form-sheet/form-sheet";
import { FormSheetContent } from "@/components/client/form-sheet/form-sheet-content";
import { Button } from "@/components/ui/button";
import {
  SheetClose,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import type { FieldVisit } from "@/lib/api/generated/model";
import type { AppAbility } from "@/lib/capabilities";

import { SAVE_TEXT, SAVING_TEXT, SHEET_TITLE } from "./utils";

import { useRecordVisit } from "./use-record-visit";
import { VisitOutcomeForm } from "./visit-outcome-form";

type VisitOutcomeSheetProps = {
  orgId: string;
  // The tenant's row as the list holds it now.
  visit: FieldVisit;
  // This opening's requestId; the screen mounts the sheet afresh for each one.
  requestId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // Whether this opening may have recorded the outcome without the app knowing (its answer was
  // lost): the screen then reopens this same sheet instead of a fresh one.
  onUnsureChange: (unsure: boolean) => void;
  // Open "Terima tunai" or "Catat janji bayar" over this sheet, which stays open beneath with
  // everything chosen so far.
  onReceiveCash: () => void;
  onRecordPromise: () => void;
};

// "Catat hasil kunjungan" (board HP-Kunjungan, columns 2 to 5): a bottom sheet on the phone, the
// right-hand drawer from md up. It cannot close while the outcome is saved. On a phone, closing
// with something chosen or typed asks first. Saving raises the toast and closes it.
export function VisitOutcomeSheet({
  orgId,
  visit,
  requestId,
  open,
  onOpenChange,
  onUnsureChange,
  onReceiveCash,
  onRecordPromise,
}: VisitOutcomeSheetProps) {
  const ability = useAbility<AppAbility>();
  const formId = `${useId()}-form`;
  const record = useRecordVisit({
    orgId,
    visit,
    requestId,
    onSaved: () => onOpenChange(false),
  });
  useEffect(() => {
    onUnsureChange(record.locked);
  }, [record.locked, onUnsureChange]);

  function changeOpen(next: boolean) {
    if (!next) record.finish();
    onOpenChange(next);
  }

  return (
    <FormSheet
      open={open}
      onOpenChange={changeOpen}
      // After a lost answer the opening is kept (locked): the row's button brings it back with
      // its requestId, so closing loses nothing and does not ask "Tutup tanpa menyimpan?".
      dirty={record.dirty && !record.locked}
      busy={record.saving}
    >
      <FormSheetContent showCloseButton={false}>
        <SheetHeader className="flex-row items-start gap-3 border-b px-5 pt-5 pb-3 md:px-6 md:pt-6 md:pb-4">
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <SheetTitle className="text-xl leading-tight font-bold md:text-2xl">
              {SHEET_TITLE}
            </SheetTitle>
            <SheetDescription className="text-sm text-muted-foreground md:text-base">
              {sheetSubtitle(visit.tenantName, placeText(visit.units))}
            </SheetDescription>
          </div>
          <SheetClose asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={CLOSE_LABEL}
              disabled={record.saving}
              className="-mt-1.5 -mr-2 rounded-full"
            >
              <XIcon aria-hidden="true" className="size-5" />
            </Button>
          </SheetClose>
        </SheetHeader>
        <VisitOutcomeForm
          formId={formId}
          visit={record}
          owed={owedText(visit)}
          canReceiveCash={canRecordCash(ability)}
          canPromise={canCreatePromise(ability)}
          onReceiveCash={onReceiveCash}
          onRecordPromise={onRecordPromise}
        />
        <CashSaveFooter
          formId={formId}
          label={record.saving ? SAVING_TEXT : SAVE_TEXT}
          canSave={record.canSave}
          saving={record.saving}
        />
      </FormSheetContent>
    </FormSheet>
  );
}
