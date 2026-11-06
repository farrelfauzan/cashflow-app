"use client";

import { Download } from "lucide-react";

import { Button } from "@/components/ui/button";

import { QrisAmountRow } from "./qris-amount-row";
import { QrisImage } from "./qris-image";
import { QrisPayGuide } from "./qris-pay-guide";
import { useCountdown } from "./use-countdown";
import {
  QRIS_DOWNLOAD_TEXT,
  QRIS_HELP_TEXT,
  QRIS_NEW_TEXT,
  QRIS_PAID_CLAIM_TEXT,
  QRIS_PENDING_TEXT,
  countdownText,
  showsNewQrUnderLive,
} from "./utils";

type QrisLiveProps = {
  imageUrl: string;
  imageAlt: string;
  downloadName: string;
  // The QR's amount, formatted; null when the API left it out.
  amount: string | null;
  expiresAt: string;
  // "Saya sudah bayar" is running: both outline buttons wait.
  checkRunning: boolean;
  onCheck: () => void;
  onNewQr: () => void;
  // Reads the status once (the countdown reached zero, or the image did not load).
  onPoll: () => void;
};

// A QR that can be paid (board "Bayar QRIS"): the image, the time left, "Unduh QR" as the one
// primary action, then "Saya sudah bayar" and, in the last two minutes, "Buat QR baru", and the
// closed "Cara bayar" guide.
export function QrisLive({
  imageUrl,
  imageAlt,
  downloadName,
  amount,
  expiresAt,
  checkRunning,
  onCheck,
  onNewQr,
  onPoll,
}: QrisLiveProps) {
  const { now, seconds } = useCountdown(expiresAt, onPoll);

  return (
    <>
      <div className="flex flex-col items-center gap-3 rounded-xl border bg-card px-4 py-4 shadow-xs md:px-5">
        {amount !== null && <QrisAmountRow amount={amount} />}
        <QrisImage src={imageUrl} alt={imageAlt} onError={onPoll} />
        <span className="inline-flex min-h-7 items-center rounded-md bg-accent px-2.5 text-sm font-semibold text-accent-foreground tabular-nums">
          {countdownText(seconds)}
        </span>
        <p className="text-center text-sm leading-normal md:text-base">
          {QRIS_HELP_TEXT}
        </p>
        <Button asChild size="lg" className="w-full font-semibold">
          <a href={imageUrl} download={downloadName}>
            <Download aria-hidden="true" />
            {QRIS_DOWNLOAD_TEXT}
          </a>
        </Button>
      </div>
      <p className="flex items-center gap-2 text-sm leading-normal md:text-base">
        <span
          aria-hidden="true"
          className="size-2.5 shrink-0 animate-pulse rounded-full bg-primary"
        />
        {QRIS_PENDING_TEXT}
      </p>
      <div className="flex flex-col gap-2">
        <Button
          type="button"
          variant="outline"
          size="lg"
          className="w-full font-semibold"
          disabled={checkRunning}
          onClick={onCheck}
        >
          {QRIS_PAID_CLAIM_TEXT}
        </Button>
        {showsNewQrUnderLive(expiresAt, now) && (
          <Button
            type="button"
            variant="outline"
            size="lg"
            className="w-full font-semibold"
            disabled={checkRunning}
            onClick={onNewQr}
          >
            {QRIS_NEW_TEXT}
          </Button>
        )}
      </div>
      <QrisPayGuide />
    </>
  );
}
