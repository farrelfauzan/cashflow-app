"use client";

import { useEffect, useId, useRef } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  CLOSE_TEXT,
  COPY_AGAIN_TEXT,
  PAY_LINK_FALLBACK_HINT,
  PAY_LINK_FALLBACK_LABEL,
} from "./utils";

type PayLinkFallbackProps = {
  // The link just made; shown only here, once.
  url: string;
  // "Salin": tries the clipboard again.
  onCopyAgain: () => void;
  // "Tutup": the link is dropped.
  onClose: () => void;
};

// When the browser could not copy a new payment link: the link in a read-only field, selected so
// it can be copied by hand, with "Salin" to try again and "Tutup". Opens selected.
export function PayLinkFallback({
  url,
  onCopyAgain,
  onClose,
}: PayLinkFallbackProps) {
  const field = useRef<HTMLInputElement>(null);
  const hintId = useId();
  const fieldId = useId();

  useEffect(() => {
    field.current?.focus();
    field.current?.select();
  }, [url]);

  return (
    <section
      aria-label={PAY_LINK_FALLBACK_LABEL}
      className="flex flex-col gap-3 rounded-[14px] border bg-card p-4 md:rounded-2xl md:px-6 md:py-5"
    >
      <label htmlFor={fieldId} className="text-base font-semibold">
        {PAY_LINK_FALLBACK_LABEL}
      </label>
      <p
        id={hintId}
        className="text-[15px] leading-normal text-muted-foreground md:text-base"
      >
        {PAY_LINK_FALLBACK_HINT}
      </p>
      <Input
        ref={field}
        id={fieldId}
        readOnly
        value={url}
        aria-describedby={hintId}
        onFocus={(event) => event.currentTarget.select()}
        className="h-12 bg-background font-mono"
      />
      <div className="flex flex-col-reverse gap-2.5 md:flex-row md:justify-end">
        <Button
          type="button"
          variant="outline"
          onClick={onClose}
          className="h-12 w-full rounded-xl border-[1.5px] bg-card text-base font-semibold md:w-auto md:px-[18px]"
        >
          {CLOSE_TEXT}
        </Button>
        <Button
          type="button"
          variant="outline"
          onClick={() => {
            field.current?.select();
            onCopyAgain();
          }}
          className="h-12 w-full rounded-xl border-[1.5px] bg-card text-base font-semibold md:w-auto md:px-[18px]"
        >
          {COPY_AGAIN_TEXT}
        </Button>
      </div>
    </section>
  );
}
