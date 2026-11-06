"use client";

import { FormAlert } from "@/components/client/form/form-alert";
import type { PaymentPage } from "@/lib/api/generated/model";

import { BankTransferMethod } from "./bank-transfer-method";
import { PaymentMethodTabs } from "./payment-method-tabs";
import { QrisAnnouncer } from "./qris-announcer";
import { QrisChecking } from "./qris-checking";
import { QrisPanel } from "./qris-panel";
import type { QrisOfferView, TransferView } from "./type";
import { useQrisPayment } from "./use-qris-payment";
import {
  QRIS_UNAVAILABLE_TEXT,
  qrisAnnouncement,
  qrisCheckingRows,
} from "./utils";

type PaymentMethodsProps = {
  token: string;
  offer: QrisOfferView;
  transfer: TransferView | null;
  noTransferText: string;
  refetchPage: () => Promise<PaymentPage | undefined>;
  onLinkGone: () => void;
};

// How to pay when the organization has set up QRIS (boards "Bayar QRIS", "QR kedaluwarsa",
// "Sedang dicek", "QRIS gangguan"): the QRIS and Transfer bank tabs, QRIS first. While a QR is
// being checked, that card takes the place of the tabs.
export function PaymentMethods({
  token,
  offer,
  transfer,
  noTransferText,
  refetchPage,
  onLinkGone,
}: PaymentMethodsProps) {
  const qris = useQrisPayment({ token, offer, refetchPage, onLinkGone });
  const { card } = qris.model;

  return (
    <>
      <QrisAnnouncer text={qrisAnnouncement(card)} />
      {card.kind === "unavailable" && (
        <FormAlert message={QRIS_UNAVAILABLE_TEXT} />
      )}
      {card.kind === "checking" ? (
        <QrisChecking rows={qrisCheckingRows(card)} />
      ) : (
        <PaymentMethodTabs
          tab={qris.model.tab}
          onTabChange={qris.selectTab}
          qris={<QrisPanel token={token} offer={offer} qris={qris} />}
          transfer={
            <BankTransferMethod
              transfer={transfer}
              noTransferText={noTransferText}
            />
          }
        />
      )}
    </>
  );
}
