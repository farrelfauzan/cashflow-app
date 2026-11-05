"use client";

import { BankTransferSection } from "./bank-transfer-section";
import { NoTransferNote } from "./no-transfer-note";
import { PaymentPageHeader } from "./payment-page-header";
import { PaymentSummaryCard } from "./payment-summary-card";
import { QrisSoonNote } from "./qris-soon-note";
import type { OpenPaymentPageView } from "./type";
import { ALLOCATION_NOTE, FOOTER_TEXT } from "./utils";

type PaymentOpenViewProps = {
  view: OpenPaymentPageView;
};

// The tenant owes something (boards "Bayar · Pilih" and "Bayar · Transfer"): what is open, the
// total, and how to pay it by bank transfer.
export function PaymentOpenView({ view }: PaymentOpenViewProps) {
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
      {view.transfer === null ? (
        <NoTransferNote text={view.noTransferText} />
      ) : (
        <BankTransferSection transfer={view.transfer} />
      )}
      <QrisSoonNote />
      <p className="mt-auto rounded-xl bg-accent px-4 py-3 text-sm text-foreground md:text-base">
        {FOOTER_TEXT}
      </p>
    </>
  );
}
