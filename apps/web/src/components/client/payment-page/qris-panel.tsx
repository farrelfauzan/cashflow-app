"use client";

import { FormAlert } from "@/components/client/form/form-alert";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

import { QrisExpired } from "./qris-expired";
import { QrisLive } from "./qris-live";
import { QrisStart } from "./qris-start";
import type { QrisOfferView, QrisPayment } from "./type";
import {
  PARTIAL_OPEN_TEXT,
  partialPayLabel,
  qrisAmountText,
  qrisImageAlt,
  qrisImageUrl,
  qrisPartialAmount,
} from "./utils";

type QrisPanelProps = {
  token: string;
  offer: QrisOfferView;
  qris: QrisPayment;
  // The amount of "Bayar sebagian", or null for the total. With an amount, the card above the
  // tabs shows it, so the QR card leaves out its own "Jumlah" row.
  chosen: number | null;
  // Opens "Bayar sebagian" (under "Bayar QRIS" while no QR exists); null where not offered.
  onPartial: (() => void) | null;
};

// The QRIS tab: when more than one QR can take is owed, a note that the rest is paid later; what
// the card shows in its current state; and the alert under its buttons.
export function QrisPanel({
  token,
  offer,
  qris,
  chosen,
  onPartial,
}: QrisPanelProps) {
  const { card, alert, checkRunning } = qris.model;
  const showsQrAmount = chosen === null;

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
            amount={showsQrAmount ? qrisAmountText(card.amount) : null}
            expiresAt={card.expiresAt}
            checkRunning={checkRunning}
            onCheck={qris.check}
            onNewQr={() => qris.start(false, chosen)}
            onPoll={qris.pollNow}
          />
        );
      case "expired":
        return (
          <QrisExpired
            amount={showsQrAmount ? qrisAmountText(card.amount) : null}
            onNewQr={() => qris.start(true, chosen)}
          />
        );
      // Checking replaces the tabs (PaymentMethods), so it never reaches this tab.
      case "checking":
        return null;
      // Idle, and after an outage: "Bayar QRIS" again.
      default:
        return (
          <>
            <QrisStart
              label={
                chosen === null
                  ? offer.startLabel
                  : partialPayLabel(
                      qrisPartialAmount(chosen, offer.outstanding),
                    )
              }
              creating={card.kind === "creating" || card.kind === "waiting"}
              onStart={() => qris.start(false, chosen)}
            />
            {onPartial !== null && card.kind === "idle" && (
              <Button
                type="button"
                variant="link"
                className="w-full font-semibold md:text-[17px]"
                onClick={onPartial}
              >
                {PARTIAL_OPEN_TEXT}
              </Button>
            )}
          </>
        );
    }
  }

  return (
    <>
      {offer.limitNote !== null &&
        chosen === null &&
        card.kind !== "resuming" && (
          <p className="rounded-xl border bg-card px-4 py-3.5 text-sm leading-normal md:px-5 md:py-4 md:text-base">
            {offer.limitNote}
          </p>
        )}
      {body()}
      {alert !== null && <FormAlert message={alert} />}
    </>
  );
}
