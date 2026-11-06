"use client";

import type { PaymentPage } from "@/lib/api/generated/model";

import { PaymentFlow } from "./payment-flow";
import type { OpenPaymentPageView, QrisOfferView } from "./type";
import { useQrisPayment } from "./use-qris-payment";

type QrisPaymentFlowProps = {
  view: OpenPaymentPageView;
  offer: QrisOfferView;
  token: string;
  refetchPage: () => Promise<PaymentPage | undefined>;
  onLinkGone: () => void;
};

// The open page when QRIS is set up: one QRIS state for the whole visit, handed to every view of
// the flow, so moving between them never loses a QR.
export function QrisPaymentFlow({
  view,
  offer,
  token,
  refetchPage,
  onLinkGone,
}: QrisPaymentFlowProps) {
  const qris = useQrisPayment({ token, offer, refetchPage, onLinkGone });
  return (
    <PaymentFlow
      view={view}
      token={token}
      qris={qris}
      refetchPage={refetchPage}
      onLinkGone={onLinkGone}
    />
  );
}
