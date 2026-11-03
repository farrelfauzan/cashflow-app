"use client";

import type { InvoiceRowView } from "./type";
import {
  ACCOUNT_CARD_CLASS,
  INVOICES_LIST_LABEL,
  INVOICES_TITLE,
  NO_INVOICES_TEXT,
} from "./utils";

import { InvoiceRow } from "./invoice-row";

type InvoiceListProps = {
  invoices: readonly InvoiceRowView[];
  // Opens "Koreksi nominal" for one invoice, by id; none hides the action.
  onCorrect?: (invoiceId: string) => void;
};

// "Tagihan": every invoice of the tenant, newest period first.
export function InvoiceList({ invoices, onCorrect }: InvoiceListProps) {
  return (
    <section className={ACCOUNT_CARD_CLASS}>
      <h2 className="text-xl font-bold md:text-2xl">{INVOICES_TITLE}</h2>
      {invoices.length === 0 ? (
        <p className="text-[15px] text-muted-foreground md:text-base">
          {NO_INVOICES_TEXT}
        </p>
      ) : (
        <ul aria-label={INVOICES_LIST_LABEL} className="flex flex-col gap-3">
          {invoices.map((invoice) => (
            <InvoiceRow
              key={invoice.id}
              invoice={invoice}
              onCorrect={
                onCorrect === undefined
                  ? undefined
                  : () => onCorrect(invoice.id)
              }
            />
          ))}
        </ul>
      )}
    </section>
  );
}
