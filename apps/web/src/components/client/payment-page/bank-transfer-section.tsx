"use client";

import { CopyAccountNumberButton } from "./copy-account-number-button";
import type { TransferView } from "./type";
import {
  PARTIAL_ALLOWED_TEXT,
  RECORDED_BY_ADMIN_TEXT,
  TRANSFER_TITLE,
  TRANSFER_TO_LABEL,
} from "./utils";

type BankTransferSectionProps = {
  transfer: TransferView;
};

// "Transfer bank": the organization's account, the copy button, and what to write in the
// transfer note. An admin records the payment once the money arrives.
export function BankTransferSection({ transfer }: BankTransferSectionProps) {
  return (
    <section
      aria-labelledby="bank-transfer-title"
      className="flex flex-col gap-3 rounded-xl border bg-card px-4 py-3.5 shadow-xs md:px-5 md:py-4"
    >
      <h2 id="bank-transfer-title" className="text-base font-semibold">
        {TRANSFER_TITLE}
      </h2>
      <div className="flex flex-col gap-0.5">
        <p className="text-sm text-muted-foreground md:text-base">
          {TRANSFER_TO_LABEL}
        </p>
        <p className="text-sm font-semibold md:text-base">
          {transfer.bankName}
        </p>
        <p className="text-2xl font-bold tracking-wide break-all">
          {transfer.accountNumber}
        </p>
        <p className="text-sm md:text-base">{transfer.holderLine}</p>
      </div>
      <CopyAccountNumberButton accountNumber={transfer.accountNumber} />
      <ul className="flex flex-col gap-1 text-sm leading-normal md:text-base">
        <li>{transfer.noteLine}</li>
        <li>{PARTIAL_ALLOWED_TEXT}</li>
        <li className="text-muted-foreground">{RECORDED_BY_ADMIN_TEXT}</li>
      </ul>
    </section>
  );
}
