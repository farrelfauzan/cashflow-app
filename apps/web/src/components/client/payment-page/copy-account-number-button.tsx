"use client";

import { Copy } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";

import type { CopyStatus } from "./type";
import { COPY_BUTTON_TEXT, COPY_STATUS_TEXT, copyText } from "./utils";

type CopyAccountNumberButtonProps = {
  accountNumber: string;
};

// The page's one primary action: copies the account number, then says whether it worked in a
// live region, so a screen reader hears it too.
export function CopyAccountNumberButton({
  accountNumber,
}: CopyAccountNumberButtonProps) {
  const [status, setStatus] = useState<CopyStatus>("idle");

  async function copy() {
    setStatus(await copyText(accountNumber, navigator.clipboard));
  }

  return (
    <div className="flex flex-col gap-2">
      <Button
        type="button"
        size="lg"
        className="w-full font-semibold"
        onClick={() => void copy()}
      >
        <Copy aria-hidden="true" />
        {COPY_BUTTON_TEXT}
      </Button>
      <p
        role="status"
        aria-live="polite"
        className={
          status === "failed"
            ? "text-sm text-destructive md:text-base"
            : "text-sm text-primary md:text-base"
        }
      >
        {COPY_STATUS_TEXT[status]}
      </p>
    </div>
  );
}
