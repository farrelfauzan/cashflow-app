"use client";

import { useState } from "react";

import type { PaymentPage } from "@/lib/api/generated/model";

import { ChosenAmountCard } from "./chosen-amount-card";
import { PartialEntry } from "./partial-entry";
import { PaymentFullView } from "./payment-full-view";
import { PaymentMethods } from "./payment-methods";
import { PaymentPageFooter } from "./payment-page-footer";
import { PaymentPageHeader } from "./payment-page-header";
import { QrisAnnouncer } from "./qris-announcer";
import type {
  OpenPaymentPageView,
  PartialPayMode,
  QrisCard,
  QrisOfferView,
} from "./type";
import { useQrisPayment } from "./use-qris-payment";
import {
  FULL_MODE,
  PARTIAL_TITLE,
  canChangePartialAmount,
  changePartialAmount,
  choosePartial,
  chosenAmountView,
  openPartialEntry,
  partialModeAfterCard,
  qrisAnnouncement,
} from "./utils";

type QrisPaymentFlowProps = {
  view: OpenPaymentPageView;
  offer: QrisOfferView;
  token: string;
  refetchPage: () => Promise<PaymentPage | undefined>;
  onLinkGone: () => void;
};

// The page when QRIS is set up, in three views that share one QRIS state: the whole total
// (today's page), "Bayar sebagian" (the amount and its preview), and the chosen amount with its
// QR. A QR of a chosen amount that gets paid brings the tenant back to the whole page.
export function QrisPaymentFlow({
  view,
  offer,
  token,
  refetchPage,
  onLinkGone,
}: QrisPaymentFlowProps) {
  const qris = useQrisPayment({ token, offer, refetchPage, onLinkGone });
  const { card } = qris.model;
  const [mode, setMode] = useState<PartialPayMode>(FULL_MODE);

  // Follows the card from render to render (React's "previous props" pattern): a paid QR of a
  // chosen amount leaves the chosen view.
  const [previousCard, setPreviousCard] = useState<QrisCard>(card);
  if (previousCard !== card) {
    setPreviousCard(card);
    const next = partialModeAfterCard(mode, previousCard, qris.model);
    if (next !== mode) setMode(next);
  }

  const partialHeader = { ...view.header, title: PARTIAL_TITLE };

  function body() {
    switch (mode.kind) {
      case "entry":
        return (
          <PartialEntry
            header={partialHeader}
            offer={offer}
            token={token}
            text={mode.text}
            onTextChange={(text) => setMode({ kind: "entry", text })}
            onChoose={(preview) => {
              setMode(choosePartial(mode, preview));
              qris.start(false, preview.amount);
            }}
            onBack={() => setMode(FULL_MODE)}
            onLinkGone={onLinkGone}
          />
        );
      case "chosen":
        return (
          <>
            <PaymentPageHeader header={partialHeader} />
            {card.kind !== "checking" && (
              <ChosenAmountCard
                {...chosenAmountView(mode.amount, mode.outstandingAfter, card)}
                onChange={
                  canChangePartialAmount(card)
                    ? () => setMode(changePartialAmount(mode))
                    : null
                }
              />
            )}
            <PaymentMethods
              token={token}
              offer={offer}
              qris={qris}
              chosen={mode.amount}
              onPartial={null}
              transfer={view.transfer}
              noTransferText={view.noTransferText}
            />
            <PaymentPageFooter />
          </>
        );
      default:
        return (
          <PaymentFullView
            view={view}
            methods={
              <PaymentMethods
                token={token}
                offer={offer}
                qris={qris}
                chosen={null}
                onPartial={() => setMode(openPartialEntry())}
                transfer={view.transfer}
                noTransferText={view.noTransferText}
              />
            }
          />
        );
    }
  }

  return (
    <>
      <QrisAnnouncer text={qrisAnnouncement(card)} />
      {body()}
    </>
  );
}
