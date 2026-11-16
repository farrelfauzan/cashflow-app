"use client";

import { TriangleAlert } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

import type { ProofPayerView } from "./type";
import { PAYER_CHANGE, PAYER_LABEL } from "./utils";

type ProofPayerProps = {
  payer: ProofPayerView;
  // "Bukan pengirim bukti (…). Tagihan … tidak berubah." when the payment is not the sender's;
  // null when it is.
  notice: string | null;
  // Opens "Pembayaran untuk siapa?"; without it there is no "Ganti" (the member may not search
  // tenants, or a save may already have recorded the payment for this payee).
  onChange?: () => void;
};

// "Pembayaran dari": whose payment the proof becomes, with their unit and what they still owe, and
// "Ganti" to make it another tenant's (board Admin-Cek-Bukti-Lain). Someone other than the sender
// gets a warning frame and the line saying the sender's invoices do not change.
export function ProofPayer({ payer, notice, onChange }: ProofPayerProps) {
  return (
    <section aria-label={PAYER_LABEL} className="flex flex-col gap-2">
      <h3 className="text-[15px] leading-normal font-semibold md:text-base">
        {PAYER_LABEL}
      </h3>
      <div
        className={cn(
          "flex items-center gap-3 rounded-xl border-[1.5px] bg-card py-1.5 pl-3.5",
          notice === null ? "border-border" : "border-warning",
          onChange === undefined ? "pr-3.5" : "pr-1.5",
        )}
      >
        <div className="flex min-w-0 flex-1 flex-col gap-0.5 py-1.5">
          <span className="text-base font-semibold md:text-[17px]">
            {payer.name}
          </span>
          {payer.detail !== null && (
            <span className="text-sm text-muted-foreground md:text-base">
              {payer.detail}
            </span>
          )}
        </div>
        {onChange !== undefined && (
          <Button
            type="button"
            variant="ghost"
            onClick={onChange}
            className="h-11 min-w-11 shrink-0 rounded-lg px-3 text-base font-semibold text-primary md:text-[17px]"
          >
            {PAYER_CHANGE}
          </Button>
        )}
      </div>
      {notice !== null && (
        <p className="flex items-start gap-2 text-sm leading-normal text-warning md:text-base">
          <TriangleAlert
            aria-hidden="true"
            className="mt-0.5 size-4 shrink-0"
          />
          {notice}
        </p>
      )}
    </section>
  );
}
