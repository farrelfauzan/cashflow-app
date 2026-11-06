"use client";

import type { PaymentPage } from "@/lib/api/generated/model";

import { BankTransferMethod } from "./bank-transfer-method";
import { PaymentFullView } from "./payment-full-view";
import { QrisPaymentFlow } from "./qris-payment-flow";
import type { OpenPaymentPageView } from "./type";

type PaymentOpenViewProps = {
  view: OpenPaymentPageView;
  token: string;
  refetchPage: () => Promise<PaymentPage | undefined>;
  onLinkGone: () => void;
};

// The tenant owes something. With QRIS set up, the QRIS flow (the whole total, or "Bayar
// sebagian"); without it, bank transfer only, as before.
export function PaymentOpenView({
  view,
  token,
  refetchPage,
  onLinkGone,
}: PaymentOpenViewProps) {
  if (view.qris === null) {
    return (
      <PaymentFullView
        view={view}
        methods={
          <BankTransferMethod
            transfer={view.transfer}
            noTransferText={view.noTransferText}
          />
        }
      />
    );
  }
  return (
    <QrisPaymentFlow
      view={view}
      offer={view.qris}
      token={token}
      refetchPage={refetchPage}
      onLinkGone={onLinkGone}
    />
  );
}
