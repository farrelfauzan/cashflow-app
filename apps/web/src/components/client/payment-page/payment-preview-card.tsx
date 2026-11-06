"use client";

import { PaymentPreviewRow } from "./payment-preview-row";
import type { PreviewView } from "./type";
import { PREVIEW_NOTE, PREVIEW_TITLE } from "./utils";

type PaymentPreviewCardProps = {
  preview: PreviewView;
  // A newer amount is being read: the card dims until it answers.
  loading: boolean;
};

// "Uang ini dipakai untuk": each invoice the amount reaches, oldest first, as the API answered,
// and what would still be owed.
export function PaymentPreviewCard({
  preview,
  loading,
}: PaymentPreviewCardProps) {
  return (
    <section
      aria-label={PREVIEW_TITLE}
      aria-busy={loading}
      className="flex flex-col gap-3 rounded-xl border bg-card px-4 py-3.5 shadow-xs transition-opacity aria-busy:opacity-60 md:px-5 md:py-4"
    >
      <h2 className="text-sm font-semibold md:text-base">{PREVIEW_TITLE}</h2>
      <ul className="flex flex-col gap-2.5">
        {preview.rows.map((row) => (
          <PaymentPreviewRow key={row.id} row={row} />
        ))}
      </ul>
      {preview.outstandingAfterLine !== null && (
        <>
          <div aria-hidden="true" className="h-px bg-border" />
          <p className="text-sm font-semibold md:text-base">
            {preview.outstandingAfterLine}
          </p>
        </>
      )}
      <p className="text-sm text-muted-foreground md:text-base">
        {PREVIEW_NOTE}
      </p>
    </section>
  );
}
