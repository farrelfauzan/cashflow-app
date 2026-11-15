"use client";

import { Copy } from "lucide-react";

import { copyText } from "@/components/client/payment-page/utils";
import { notify } from "@/components/client/toast/notify";
import { Button } from "@/components/ui/button";

import {
  COPIED_TOAST,
  COPY_FAILED_TOAST,
  COPY_LABEL,
  TRANSACTION_ID_LABEL,
} from "./utils";

type TransactionIdRowProps = {
  transactionId: string;
};

// AutoGoPay's id of the QR, to look up in its dashboard, with "Salin"; a toast says whether the
// copy worked.
export function TransactionIdRow({ transactionId }: TransactionIdRowProps) {
  async function copy() {
    const status = await copyText(transactionId, navigator.clipboard);
    notify(status === "copied" ? COPIED_TOAST : COPY_FAILED_TOAST);
  }

  return (
    <div className="flex items-center gap-3 rounded-xl border px-3.5 py-3">
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="text-sm text-muted-foreground md:text-base">
          {TRANSACTION_ID_LABEL}
        </span>
        <span className="font-mono text-base break-all">{transactionId}</span>
      </div>
      <Button
        type="button"
        variant="outline"
        className="font-semibold"
        aria-label={`${COPY_LABEL} ${TRANSACTION_ID_LABEL}`}
        onClick={() => void copy()}
      >
        <Copy aria-hidden="true" />
        {COPY_LABEL}
      </Button>
    </div>
  );
}
