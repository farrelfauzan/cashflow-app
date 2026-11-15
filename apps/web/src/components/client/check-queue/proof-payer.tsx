"use client";

import type { ProofPayerView } from "./type";
import { PAYER_LABEL } from "./utils";

type ProofPayerProps = {
  payer: ProofPayerView;
};

// "Pembayaran dari": whose payment the proof becomes, always the proof's own tenant, with their
// unit and what they still owe.
export function ProofPayer({ payer }: ProofPayerProps) {
  return (
    <section aria-label={PAYER_LABEL} className="flex flex-col gap-2">
      <h3 className="text-[15px] leading-normal font-semibold md:text-base">
        {PAYER_LABEL}
      </h3>
      <div className="flex flex-col gap-0.5 rounded-xl border-[1.5px] bg-card px-3.5 py-3">
        <span className="text-base font-semibold md:text-[17px]">
          {payer.name}
        </span>
        {payer.detail !== null && (
          <span className="text-sm text-muted-foreground md:text-base">
            {payer.detail}
          </span>
        )}
      </div>
    </section>
  );
}
