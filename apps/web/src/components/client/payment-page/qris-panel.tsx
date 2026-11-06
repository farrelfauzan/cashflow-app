"use client";

import { FormAlert } from "@/components/client/form/form-alert";
import { Skeleton } from "@/components/ui/skeleton";

import { QrisExpired } from "./qris-expired";
import { QrisLive } from "./qris-live";
import { QrisStart } from "./qris-start";
import type { QrisOfferView, QrisPayment } from "./type";
import { qrisAmountText, qrisImageAlt, qrisImageUrl } from "./utils";

type QrisPanelProps = {
  token: string;
  offer: QrisOfferView;
  qris: QrisPayment;
};

// The QRIS tab: when more than one QR can take is owed, a note that the rest is paid later; what
// the card shows in its current state; and the alert under its buttons.
export function QrisPanel({ token, offer, qris }: QrisPanelProps) {
  const { card, alert, checkRunning } = qris.model;

  function body() {
    switch (card.kind) {
      case "resuming":
        return <Skeleton className="h-12 w-full rounded-md bg-muted" />;
      case "pending":
        return (
          <QrisLive
            imageUrl={qrisImageUrl(token, card.expiresAt)}
            imageAlt={qrisImageAlt(card.amount)}
            downloadName={offer.downloadName}
            amount={qrisAmountText(card.amount)}
            expiresAt={card.expiresAt}
            checkRunning={checkRunning}
            onCheck={qris.check}
            onNewQr={() => qris.start(false)}
            onPoll={qris.pollNow}
          />
        );
      case "expired":
        return (
          <QrisExpired
            amount={qrisAmountText(card.amount)}
            onNewQr={() => qris.start(true)}
          />
        );
      // Checking replaces the tabs (PaymentMethods), so it never reaches this tab.
      case "checking":
        return null;
      // Idle, and after an outage: "Bayar QRIS" again.
      default:
        return (
          <QrisStart
            label={offer.startLabel}
            creating={card.kind === "creating" || card.kind === "waiting"}
            onStart={() => qris.start(false)}
          />
        );
    }
  }

  return (
    <>
      {offer.limitNote !== null && card.kind !== "resuming" && (
        <p className="rounded-xl border bg-card px-4 py-3.5 text-sm leading-normal md:px-5 md:py-4 md:text-base">
          {offer.limitNote}
        </p>
      )}
      {body()}
      {alert !== null && <FormAlert message={alert} />}
    </>
  );
}
