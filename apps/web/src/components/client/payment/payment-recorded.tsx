"use client";

import { Check } from "lucide-react";

import { LeaseInfoNote } from "@/components/client/lease/lease-info-note";
import { Button } from "@/components/ui/button";
import type { PaymentResult } from "@/lib/api/generated/model";
import {
  ALL_CREDIT_TEXT,
  CORRECTION_HINT,
  PAID_LIST_LABEL,
  PAID_TITLE,
  paymentResultView,
  RESULT_TITLE,
} from "./utils";

import { PaymentPaidRow } from "./payment-paid-row";

type PaymentRecordedProps = {
  result: PaymentResult;
  onDone: () => void;
};

// "Pembayaran tercatat" (board "Pembayaran tercatat"), in the panel that recorded it: the
// payment, the invoices it paid oldest first, and the tenant's credit and debt after it. Every
// figure is a field of the API's answer.
export function PaymentRecorded({ result, onDone }: PaymentRecordedProps) {
  const view = paymentResultView(result);

  return (
    <>
      <div
        data-slot="form-sheet-body"
        className="flex min-h-0 flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-6"
      >
        <div
          role="status"
          className="flex items-start gap-3.5 rounded-[14px] bg-accent p-4"
        >
          <span
            aria-hidden="true"
            className="flex size-10 shrink-0 items-center justify-center rounded-full bg-primary text-primary-foreground"
          >
            <Check className="size-[22px]" strokeWidth={2.4} />
          </span>
          <div className="flex flex-col gap-1">
            <h3 className="text-lg font-bold text-primary">{RESULT_TITLE}</h3>
            <p className="text-base leading-normal text-foreground">
              {view.headline}
            </p>
          </div>
        </div>

        <section className="flex flex-col gap-3">
          <h4 className="text-base font-bold md:text-[17px]">{PAID_TITLE}</h4>
          {view.paid.length === 0 ? (
            <p className="text-[15px] md:text-base">{ALL_CREDIT_TEXT}</p>
          ) : (
            <ul aria-label={PAID_LIST_LABEL} className="flex flex-col gap-3">
              {view.paid.map((row) => (
                <PaymentPaidRow key={row.invoiceId} row={row} />
              ))}
            </ul>
          )}
        </section>

        <dl className="flex flex-col gap-3 rounded-[14px] border bg-card px-4 py-3.5">
          {view.box.map((row) => (
            <div key={row.label} className="flex flex-col gap-0.5">
              <div className="flex flex-wrap items-baseline justify-between gap-x-3">
                <dt className="text-[15px] font-semibold text-muted-foreground md:text-base">
                  {row.label}
                </dt>
                <dd className="text-lg font-bold md:text-xl">{row.value}</dd>
              </div>
              {row.note !== null && (
                <dd className="text-sm text-muted-foreground md:text-base">
                  {row.note}
                </dd>
              )}
            </div>
          ))}
        </dl>

        <LeaseInfoNote text={CORRECTION_HINT} />
      </div>
      <div
        data-slot="form-sheet-footer"
        className="flex gap-2.5 border-t px-6 py-4"
      >
        <Button
          type="button"
          onClick={onDone}
          className="h-13 flex-1 rounded-xl text-[17px] font-semibold"
        >
          Selesai
        </Button>
      </div>
    </>
  );
}
