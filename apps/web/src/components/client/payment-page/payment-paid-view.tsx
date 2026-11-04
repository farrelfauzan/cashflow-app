"use client";

import { CircleCheck } from "lucide-react";

import { PaymentPageHeader } from "./payment-page-header";
import type { PaidPaymentPageView } from "./type";
import {
  CREDIT_LABEL,
  CREDIT_NOTE,
  PAID_DETAILS_LABEL,
  PAID_TEXT,
  PAID_TITLE,
} from "./utils";

type PaymentPaidViewProps = {
  view: PaidPaymentPageView;
};

// The tenant owes nothing (board "Bayar · Lunas"): the link's invoice and any credit. The API
// gives no method, time or receipt, so none is shown.
export function PaymentPaidView({ view }: PaymentPaidViewProps) {
  return (
    <>
      <PaymentPageHeader header={view.header} />
      <section
        aria-label={PAID_TITLE}
        className="flex flex-col items-center gap-2.5 rounded-xl bg-primary px-5 py-7 text-center text-primary-foreground"
      >
        <CircleCheck aria-hidden="true" className="size-16" strokeWidth={2} />
        <p className="text-[26px] font-bold">{PAID_TITLE}</p>
        <p className="text-sm md:text-base">{PAID_TEXT}</p>
      </section>
      <section
        aria-label={PAID_DETAILS_LABEL}
        className="flex flex-col rounded-xl border bg-card px-4 py-1.5 shadow-xs md:px-5"
      >
        <div className="flex items-center justify-between gap-3 py-3">
          <span className="text-sm text-muted-foreground md:text-base">
            {view.invoiceName}
          </span>
          <span className="text-sm font-semibold whitespace-nowrap md:text-base">
            {view.amount}
          </span>
        </div>
        {view.credit !== null && (
          <div className="flex flex-col gap-0.5 border-t py-3">
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm text-muted-foreground md:text-base">
                {CREDIT_LABEL}
              </span>
              <span className="text-sm font-semibold whitespace-nowrap md:text-base">
                {view.credit}
              </span>
            </div>
            <p className="text-sm text-muted-foreground md:text-base">
              {CREDIT_NOTE}
            </p>
          </div>
        )}
      </section>
    </>
  );
}
