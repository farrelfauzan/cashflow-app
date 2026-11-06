"use client";

import { ArrowRight, QrCode } from "lucide-react";

import { FormAlert } from "@/components/client/form/form-alert";
import { Button } from "@/components/ui/button";

import { BackButton } from "./back-button";
import { PartialAmountField } from "./partial-amount-field";
import { PaymentPageHeader } from "./payment-page-header";
import { PaymentPreviewCard } from "./payment-preview-card";
import { QuickAmountGrid } from "./quick-amount-grid";
import type { PaymentPageHeaderView, PreviewView, QrisOfferView } from "./type";
import { usePaymentPreview } from "./use-payment-preview";
import {
  entryLeadsToPromise,
  entryNextLine,
  entryPrimaryLabel,
  partialAmountCheck,
  partialFieldText,
} from "./utils";

type PartialEntryProps = {
  // The page's header with the "Bayar sebagian" title.
  header: PaymentPageHeaderView;
  offer: QrisOfferView;
  token: string;
  text: string;
  // Whether a promise can be made: then an amount that leaves something owed goes on to the
  // Janji screen before its QR.
  promiseOffered: boolean;
  onTextChange: (text: string) => void;
  // The primary: the previewed amount, on to the Janji screen or straight to its QR.
  onChoose: (preview: PreviewView) => void;
  onBack: () => void;
  onLinkGone: () => void;
};

// "Bayar sebagian" (board "Bayar sebagian"): the amount, quick amounts, what it would pay, and,
// once the API has previewed exactly that amount, "Lanjut pilih tanggal pelunasan" when something
// stays owed, or "Bayar QRIS Rp…" when it pays everything.
export function PartialEntry({
  header,
  offer,
  token,
  text,
  promiseOffered,
  onTextChange,
  onChoose,
  onBack,
  onLinkGone,
}: PartialEntryProps) {
  const check = partialAmountCheck(text);
  const amount = check.kind === "valid" ? check.amount : null;
  const preview = usePaymentPreview({ token, amount, onLinkGone });

  const fieldError =
    check.kind === "invalid"
      ? check.message
      : preview.error?.kind === "field"
        ? preview.error.message
        : null;
  const alert = preview.error?.kind === "alert" ? preview.error.message : null;
  const ready = preview.ready && preview.view !== null;
  const toPromise = entryLeadsToPromise({
    amount,
    preview: preview.view,
    outstanding: offer.outstanding,
    promiseOffered,
  });
  const nextLine = entryNextLine(preview.view, toPromise);

  return (
    <>
      <PaymentPageHeader header={header} />
      <p className="text-sm font-semibold md:text-base">{offer.totalLine}</p>
      <PartialAmountField
        text={text}
        onTextChange={onTextChange}
        error={fieldError}
      />
      <QuickAmountGrid
        chips={offer.quickAmounts}
        selected={amount}
        onSelect={(chip) => onTextChange(partialFieldText(chip))}
      />
      {preview.view !== null && fieldError === null && (
        <PaymentPreviewCard preview={preview.view} loading={!preview.ready} />
      )}
      {nextLine !== null && fieldError === null && (
        <p className="text-sm font-semibold md:text-base">{nextLine}</p>
      )}
      {alert !== null && <FormAlert message={alert} />}
      <div className="mt-auto flex flex-col gap-2">
        <Button
          type="button"
          size="lg"
          className="w-full font-semibold"
          disabled={!ready}
          onClick={() => {
            if (preview.view !== null && preview.ready) onChoose(preview.view);
          }}
        >
          {!toPromise && <QrCode aria-hidden="true" />}
          {entryPrimaryLabel(amount, toPromise)}
          {toPromise && <ArrowRight aria-hidden="true" />}
        </Button>
        <BackButton onClick={onBack} />
      </div>
    </>
  );
}
