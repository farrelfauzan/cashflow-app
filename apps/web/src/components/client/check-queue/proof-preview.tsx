"use client";

import { Info } from "lucide-react";

import type { ProofPreviewView } from "./type";
import { NO_RECEIPT_TEXT, PREVIEW_LEAD, PREVIEW_LOADING_TEXT } from "./utils";

type ProofPreviewProps = {
  // The API's preview of the typed amount; null before an amount is typed or while the first one
  // loads.
  view: ProofPreviewView | null;
  // A newer amount is being read.
  loading: boolean;
  // The API refused the preview (its message).
  error: string | null;
  // An amount is typed.
  typed: boolean;
};

// "Setelah disimpan:" and what the API says the typed amount would pay: which invoices become
// "Lunas", which stay partly paid and what they would still owe, and the credit it would add.
// A transfer sends no WhatsApp receipt, which the box always says.
export function ProofPreview({
  view,
  loading,
  error,
  typed,
}: ProofPreviewProps) {
  let body = null;
  if (error !== null) {
    body = <p>{error}</p>;
  } else if (view !== null) {
    body = (
      <>
        <p className="font-bold">{PREVIEW_LEAD}</p>
        <ul className="flex flex-col gap-0.5">
          {view.lines.map((line) => (
            <li key={line.id}>
              {line.lead} <b className="whitespace-nowrap">{line.result}</b>
            </li>
          ))}
          {view.credit !== null && <li>{view.credit}</li>}
        </ul>
      </>
    );
  } else if (typed) {
    body = (
      <p>
        <b>{PREVIEW_LEAD}</b> {PREVIEW_LOADING_TEXT}
      </p>
    );
  }
  return (
    <section
      aria-live="polite"
      aria-busy={loading}
      className="flex items-start gap-2.5 rounded-xl bg-accent px-3.5 py-3 text-foreground transition-opacity aria-busy:opacity-70"
    >
      <Info
        aria-hidden="true"
        className="mt-0.5 size-5 shrink-0 text-primary"
      />
      <div className="flex min-w-0 flex-col gap-1 text-[15px] leading-normal md:text-base">
        {body}
        <p>{NO_RECEIPT_TEXT}</p>
      </div>
    </section>
  );
}
