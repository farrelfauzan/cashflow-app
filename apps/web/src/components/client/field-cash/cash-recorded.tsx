"use client";

import { cn } from "cn";
import { Check } from "lucide-react";

import { Button } from "@/components/ui/button";

import type { CashRecordedView, ChosenPhoto } from "./type";
import { DONE_TEXT, PAID_LIST_LABEL, PAID_TITLE } from "./utils";

import { CashPaidLine } from "./cash-paid-line";
import { ReceiptPhotoThumb } from "./receipt-photo-thumb";

type CashRecordedProps = {
  view: CashRecordedView;
  // The photo on this phone that went with the cash.
  photo: ChosenPhoto | null;
  onDone: () => void;
};

// "Tersimpan" (board HP-Tagihan, 3), in the sheet that recorded it: the amount, the tenant's
// receipt, the invoices it paid oldest first and what is still owed, and the photo, whose scan is
// not waited for. Every figure is a field of the API's answer.
export function CashRecorded({ view, photo, onDone }: CashRecordedProps) {
  return (
    <>
      <div
        data-slot="form-sheet-body"
        className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-5 py-5 md:px-6"
      >
        <div
          role="status"
          className="flex flex-col items-center gap-2 pt-2 pb-1 text-center"
        >
          <span
            aria-hidden="true"
            className="flex size-13 items-center justify-center rounded-full bg-accent text-primary"
          >
            <Check className="size-[26px]" strokeWidth={2.4} />
          </span>
          <p className="text-[22px] leading-tight font-bold tabular-nums">
            {view.headline}
          </p>
          <p className="text-[15px] text-muted-foreground md:text-base">
            {view.receiptLine}
          </p>
        </div>

        <section className="flex flex-col gap-1.5 rounded-xl border px-3.5 py-3">
          <h3 className="text-[15px] font-semibold md:text-base">
            {PAID_TITLE}
          </h3>
          {view.paidEmptyText !== null ? (
            <p className="text-[15px] md:text-base">{view.paidEmptyText}</p>
          ) : (
            <ul aria-label={PAID_LIST_LABEL} className="flex flex-col gap-1.5">
              {view.paid.map((line) => (
                <CashPaidLine key={line.invoiceId} line={line} />
              ))}
            </ul>
          )}
          <dl className="mt-1 flex flex-col gap-1.5 border-t pt-2">
            {view.totals.map((total) => (
              <div
                key={total.label}
                className="flex flex-wrap items-baseline justify-between gap-x-3"
              >
                <dt className="text-[15px] text-muted-foreground md:text-base">
                  {total.label}
                </dt>
                <dd className="text-[15px] font-bold tabular-nums md:text-base">
                  {total.value}
                </dd>
              </div>
            ))}
          </dl>
        </section>

        <div
          className={cn(
            "flex items-center gap-3 rounded-xl border px-3 py-2.5",
            view.photo.refused && "border-warning",
          )}
        >
          <ReceiptPhotoThumb photo={photo} />
          <div className="flex min-w-0 flex-col">
            <span
              className={cn(
                "text-[15px] font-semibold md:text-base",
                view.photo.refused && "text-warning-strong",
              )}
            >
              {view.photo.title}
            </span>
            {view.photo.detail !== null && (
              <span className="text-sm text-muted-foreground md:text-base">
                {view.photo.detail}
              </span>
            )}
          </div>
        </div>
      </div>
      <div
        data-slot="form-sheet-footer"
        className="flex gap-2.5 border-t px-4 py-3 md:px-6 md:py-4"
      >
        <Button
          type="button"
          variant="outline"
          onClick={onDone}
          className="h-12 flex-1 rounded-xl border-[1.5px] bg-card text-base font-semibold md:h-13 md:text-[17px]"
        >
          {DONE_TEXT}
        </Button>
      </div>
    </>
  );
}
