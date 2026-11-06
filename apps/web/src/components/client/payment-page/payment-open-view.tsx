"use client";

import type { PaymentPage } from "@/lib/api/generated/model";

import { PaymentFlow } from "./payment-flow";
import { QrisPaymentFlow } from "./qris-payment-flow";
import type { OpenPaymentPageView } from "./type";

type PaymentOpenViewProps = {
  view: OpenPaymentPageView;
  token: string;
  refetchPage: () => Promise<PaymentPage | undefined>;
  onLinkGone: () => void;
};

// The tenant owes something: the choice (pay it all, a part, or promise), with the QRIS state
// when QRIS is set up, bank transfer alone when it is not.
export function PaymentOpenView({
  view,
  token,
  refetchPage,
  onLinkGone,
}: PaymentOpenViewProps) {
  if (view.qris === null) {
    return (
      <PaymentFlow
        view={view}
        token={token}
        qris={null}
        refetchPage={refetchPage}
        onLinkGone={onLinkGone}
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
