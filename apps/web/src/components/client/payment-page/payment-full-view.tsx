"use client";

import type { ReactNode } from "react";

import { PaymentPageFooter } from "./payment-page-footer";
import { PaymentPageHeader } from "./payment-page-header";
import { PaymentSummaryCard } from "./payment-summary-card";
import type { OpenPaymentPageView } from "./type";
import { ALLOCATION_NOTE } from "./utils";

type PaymentFullViewProps = {
  view: OpenPaymentPageView;
  // How to pay: the QRIS and Transfer bank tabs, or bank transfer alone.
  methods: ReactNode;
};

// Everything the tenant owes (boards "Bayar · Pilih", "Bayar · Transfer" and the QRIS boards):
// what is open, the total, and how to pay it.
export function PaymentFullView({ view, methods }: PaymentFullViewProps) {
  return (
    <>
      <PaymentPageHeader header={view.header} />
      {view.linkInvoicePaidNote !== null && (
        <p className="rounded-xl bg-accent px-4 py-3 text-sm font-semibold md:text-base">
          {view.linkInvoicePaidNote}
        </p>
      )}
      <PaymentSummaryCard
        rows={view.rows}
        total={view.total}
        credit={view.credit}
      />
      <p className="text-sm leading-normal md:text-base">{ALLOCATION_NOTE}</p>
      {methods}
      <PaymentPageFooter />
    </>
  );
}
