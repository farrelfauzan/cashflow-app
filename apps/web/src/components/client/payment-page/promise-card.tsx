"use client";

import { PROMISE_CARD_LABEL, RECORDED_REMINDER_TEXT } from "./utils";

type PromiseCardProps = {
  // "Selasa, 1 Desember 2026".
  date: string;
  // "Rp500.000", or "Semua sisa tagihan".
  amount: string;
};

// The promise on Janji tercatat: when, how much, and what the reminders do until then.
export function PromiseCard({ date, amount }: PromiseCardProps) {
  return (
    <section
      aria-label={PROMISE_CARD_LABEL}
      className="flex flex-col gap-1.5 rounded-xl border bg-card px-4 py-3.5 shadow-xs md:px-5 md:py-4"
    >
      <p className="text-sm text-muted-foreground md:text-base">
        {PROMISE_CARD_LABEL}
      </p>
      <p className="text-lg font-bold md:text-xl">{date}</p>
      <p className="text-base font-semibold md:text-[17px]">{amount}</p>
      <p className="text-sm leading-normal text-muted-foreground md:text-base">
        {RECORDED_REMINDER_TEXT}
      </p>
    </section>
  );
}
