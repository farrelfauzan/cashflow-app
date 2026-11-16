"use client";

import { Button } from "@/components/ui/button";

import type { VisitPromiseSummaryView } from "./type";
import { CHANGE_PROMISE_TEXT, RECORD_PROMISE_TEXT } from "./utils";

type VisitPromiseChoiceProps = {
  // The promise made for the tenant today that no outcome names yet; null without one.
  promise: VisitPromiseSummaryView | null;
  // The member may make promises (promise:create).
  canPromise: boolean;
  onRecordPromise: () => void;
  disabled: boolean;
};

// Under "Janji bayar baru" (board HP-Kunjungan, column 4): today's promise, made here or on
// Tagihan hari ini, with "Ubah janji"; without one, "Catat janji bayar". Both open that sheet
// over this one; the outcome then names the promise it saved.
export function VisitPromiseChoice({
  promise,
  canPromise,
  onRecordPromise,
  disabled,
}: VisitPromiseChoiceProps) {
  if (promise === null) {
    return canPromise ? (
      <Button
        type="button"
        variant="outline"
        disabled={disabled}
        onClick={onRecordPromise}
        className="h-12 rounded-xl border-[1.5px] bg-card text-base font-semibold"
      >
        {RECORD_PROMISE_TEXT}
      </Button>
    ) : null;
  }
  return (
    <div className="flex items-center justify-between gap-3 rounded-lg border-[1.5px] bg-card px-3.5 py-2">
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="text-sm font-semibold tabular-nums md:text-base">
          {promise.terms}
        </span>
        <span className="text-sm text-muted-foreground tabular-nums md:text-base">
          {promise.recorded}
        </span>
      </div>
      {canPromise && (
        <Button
          type="button"
          variant="link"
          disabled={disabled}
          onClick={onRecordPromise}
          className="h-11 shrink-0 px-1 text-base font-semibold text-primary underline"
        >
          {CHANGE_PROMISE_TEXT}
        </Button>
      )}
    </div>
  );
}
