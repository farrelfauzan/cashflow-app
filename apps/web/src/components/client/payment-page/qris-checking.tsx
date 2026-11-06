"use client";

import { Clock } from "lucide-react";

import type { QrisDetailRow } from "./type";
import {
  PAID_DETAILS_LABEL,
  QRIS_CHECKING_CHIP,
  QRIS_CHECKING_TEXT,
  QRIS_CHECKING_TITLE,
} from "./utils";

type QrisCheckingProps = {
  rows: readonly QrisDetailRow[];
};

// The tenant said they paid and no answer has come (board "Sedang dicek"). It takes the place
// of the tabs: there is nothing to do but wait, so there is no button.
export function QrisChecking({ rows }: QrisCheckingProps) {
  return (
    <>
      <section
        aria-labelledby="qris-checking-title"
        className="flex flex-col items-center gap-3 rounded-xl border bg-card px-4 py-5 text-center shadow-xs md:px-5"
      >
        <span className="flex size-14 items-center justify-center rounded-full bg-accent text-primary">
          <Clock aria-hidden="true" className="size-7" />
        </span>
        <h2 id="qris-checking-title" className="text-xl font-bold">
          {QRIS_CHECKING_TITLE}
        </h2>
        <p className="text-sm leading-normal md:text-base">
          {QRIS_CHECKING_TEXT}
        </p>
        <span className="inline-flex min-h-7 items-center rounded-md bg-warning/15 px-2.5 text-sm font-semibold text-foreground">
          {QRIS_CHECKING_CHIP}
        </span>
      </section>
      <section
        aria-label={PAID_DETAILS_LABEL}
        className="flex flex-col divide-y rounded-xl border bg-card px-4 py-1.5 shadow-xs md:px-5"
      >
        {rows.map((row) => (
          <div
            key={row.label}
            className="flex items-center justify-between gap-3 py-3"
          >
            <span className="text-sm text-muted-foreground md:text-base">
              {row.label}
            </span>
            <span className="text-sm font-semibold whitespace-nowrap md:text-base">
              {row.value}
            </span>
          </div>
        ))}
      </section>
    </>
  );
}
