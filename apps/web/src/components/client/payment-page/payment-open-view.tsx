"use client";

import type { PaymentPage } from "@/lib/api/generated/model";

import { BankTransferMethod } from "./bank-transfer-method";
import { PaymentMethods } from "./payment-methods";
import { PaymentPageHeader } from "./payment-page-header";
import { PaymentSummaryCard } from "./payment-summary-card";
import type { OpenPaymentPageView } from "./type";
import { ALLOCATION_NOTE, FOOTER_TEXT } from "./utils";

type PaymentOpenViewProps = {
  view: OpenPaymentPageView;
  token: string;
  refetchPage: () => Promise<PaymentPage | undefined>;
  onLinkGone: () => void;
};

// The tenant owes something (boards "Bayar · Pilih", "Bayar · Transfer" and the QRIS boards):
// what is open, the total, and how to pay it. Without QRIS set up, bank transfer only, as before.
export function PaymentOpenView({
  view,
  token,
  refetchPage,
  onLinkGone,
}: PaymentOpenViewProps) {
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
      {view.qris === null ? (
        <BankTransferMethod
          transfer={view.transfer}
          noTransferText={view.noTransferText}
        />
      ) : (
        <PaymentMethods
          token={token}
          offer={view.qris}
          transfer={view.transfer}
          noTransferText={view.noTransferText}
          refetchPage={refetchPage}
          onLinkGone={onLinkGone}
        />
      )}
      <p className="mt-auto rounded-xl bg-accent px-4 py-3 text-sm text-foreground md:text-base">
        {FOOTER_TEXT}
      </p>
    </>
  );
}
