"use client";

import { Check } from "lucide-react";
import { useId } from "react";

import { ProofsWaitingRow } from "./proofs-waiting-row";
import type { ProofRowView } from "./type";
import { PROOFS_NOTE, PROOFS_TITLE, PROOF_SENT_TEXT } from "./utils";

type ProofsWaitingSectionProps = {
  rows: readonly ProofRowView[];
  // A proof was sent in this visit: "Bukti terkirim. Admin akan mengeceknya." above the list.
  sent: boolean;
};

// Under the bill's summary (board "Kirim bukti transfer", 3): the line that says a proof was just
// received, and "Bukti transfer terkirim", the API's proofs still waiting for the admin (at most
// 5, newest first), also when the tenant opens the link again later. No photo is shown.
export function ProofsWaitingSection({
  rows,
  sent,
}: ProofsWaitingSectionProps) {
  const titleId = useId();
  return (
    <>
      {/* Always on the page and out of the layout, so a screen reader hears the line when it
          appears; the banner below is what the eye reads. */}
      <p role="status" aria-live="polite" className="sr-only">
        {sent ? PROOF_SENT_TEXT : ""}
      </p>
      {sent && (
        <p
          aria-hidden="true"
          className="flex items-center gap-2.5 rounded-xl bg-accent px-4 py-3 text-sm font-semibold md:text-base"
        >
          <Check
            aria-hidden="true"
            className="size-5 shrink-0 text-primary"
            strokeWidth={2.4}
          />
          {PROOF_SENT_TEXT}
        </p>
      )}
      {rows.length > 0 && (
        <section
          aria-labelledby={titleId}
          className="flex flex-col gap-2 rounded-xl border border-primary bg-card px-4 py-3.5 shadow-xs md:px-5 md:py-4"
        >
          <h2 id={titleId} className="text-base font-semibold">
            {PROOFS_TITLE}
          </h2>
          <ul className="flex flex-col divide-y">
            {rows.map((row) => (
              <ProofsWaitingRow key={row.id} row={row} />
            ))}
          </ul>
          <p className="text-sm leading-normal text-muted-foreground md:text-base">
            {PROOFS_NOTE}
          </p>
        </section>
      )}
    </>
  );
}
