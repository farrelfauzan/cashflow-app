"use client";

import { QrCode } from "lucide-react";

import { FormAlert } from "@/components/client/form/form-alert";
import { Button } from "@/components/ui/button";

import { PartialAmountField } from "./partial-amount-field";
import { PaymentPageHeader } from "./payment-page-header";
import { PaymentPreviewCard } from "./payment-preview-card";
import { QuickAmountGrid } from "./quick-amount-grid";
import type { PaymentPageHeaderView, PreviewView, QrisOfferView } from "./type";
import { usePaymentPreview } from "./use-payment-preview";
import {
  PARTIAL_BACK_TEXT,
  partialAmountCheck,
  partialFieldText,
  partialPayLabel,
} from "./utils";

type PartialEntryProps = {
  // The page's header with the "Bayar sebagian" title.
  header: PaymentPageHeaderView;
  offer: QrisOfferView;
  token: string;
  text: string;
  onTextChange: (text: string) => void;
  // The primary: pay the previewed amount by QRIS.
  onChoose: (preview: PreviewView) => void;
  onBack: () => void;
  onLinkGone: () => void;
};

// "Bayar sebagian" (board "Bayar sebagian"): the amount, quick amounts, what it would pay, and
// "Bayar QRIS Rp…" once the API has previewed exactly that amount.
export function PartialEntry({
  header,
  offer,
  token,
  text,
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
          <QrCode aria-hidden="true" />
          {partialPayLabel(amount)}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="lg"
          className="w-full font-semibold"
          onClick={onBack}
        >
          {PARTIAL_BACK_TEXT}
        </Button>
      </div>
    </>
  );
}
