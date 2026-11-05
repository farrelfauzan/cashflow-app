"use client";

import { Button } from "@/components/ui/button";

import type { PaymentPageMessageView } from "./type";
import { RETRY_TEXT } from "./utils";

type PaymentPageMessageProps = {
  message: PaymentPageMessageView;
  onRetry: () => void;
};

// A page that did not load: the link does not work, too many requests, or no connection.
export function PaymentPageMessage({
  message,
  onRetry,
}: PaymentPageMessageProps) {
  return (
    <section className="flex flex-col items-start gap-3 pt-6">
      <h1 className="text-2xl leading-tight font-bold md:text-[28px]">
        {message.title}
      </h1>
      <p className="text-sm leading-normal text-muted-foreground md:text-base">
        {message.text}
      </p>
      {message.retry && (
        <Button
          type="button"
          size="lg"
          className="w-full font-semibold"
          onClick={onRetry}
        >
          {RETRY_TEXT}
        </Button>
      )}
    </section>
  );
}
