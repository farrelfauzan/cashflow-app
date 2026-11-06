"use client";

import { FormAlert } from "@/components/client/form/form-alert";

import { BankTransferMethod } from "./bank-transfer-method";
import { PaymentMethodTabs } from "./payment-method-tabs";
import { QrisChecking } from "./qris-checking";
import { QrisPanel } from "./qris-panel";
import type { QrisOfferView, QrisPayment, TransferView } from "./type";
import { QRIS_UNAVAILABLE_TEXT, qrisCheckingRows } from "./utils";

type PaymentMethodsProps = {
  token: string;
  offer: QrisOfferView;
  // The QRIS state, owned by QrisPaymentFlow so it survives switching views.
  qris: QrisPayment;
  // The amount of "Bayar sebagian", or null for the total.
  chosen: number | null;
  // Opens "Bayar sebagian"; null where it is not offered.
  onPartial: (() => void) | null;
  transfer: TransferView | null;
  noTransferText: string;
};

// How to pay when the organization has set up QRIS (boards "Bayar QRIS", "QR kedaluwarsa",
// "Sedang dicek", "QRIS gangguan"): the QRIS and Transfer bank tabs, QRIS first. While a QR is
// being checked, that card takes the place of the tabs.
export function PaymentMethods({
  token,
  offer,
  qris,
  chosen,
  onPartial,
  transfer,
  noTransferText,
}: PaymentMethodsProps) {
  const { card } = qris.model;

  return (
    <>
      {card.kind === "unavailable" && (
        <FormAlert message={QRIS_UNAVAILABLE_TEXT} />
      )}
      {card.kind === "checking" ? (
        <QrisChecking rows={qrisCheckingRows(card)} />
      ) : (
        <PaymentMethodTabs
          tab={qris.model.tab}
          onTabChange={qris.selectTab}
          qris={
            <QrisPanel
              token={token}
              offer={offer}
              qris={qris}
              chosen={chosen}
              onPartial={onPartial}
            />
          }
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
