"use client";

import { CircleCheck } from "lucide-react";

import { Button } from "@/components/ui/button";

import { PaymentPageHeader } from "./payment-page-header";
import { PromiseCard } from "./promise-card";
import type { PaymentPageHeaderView, RecordedView } from "./type";
import { EDIT_PROMISE_TEXT, RECORDED_TITLE } from "./utils";

type PromiseRecordedViewProps = {
  header: PaymentPageHeaderView;
  view: RecordedView;
  // "Ubah janji"; null when no promise can be made now.
  onEdit: (() => void) | null;
};

// Janji tercatat (board "Janji tercatat"): the promise is saved, what this visit paid, what is
// still owed, and the promise itself. No receipt and no breakdown of the payment.
export function PromiseRecordedView({
  header,
  view,
  onEdit,
}: PromiseRecordedViewProps) {
  return (
    <>
      <PaymentPageHeader header={header} showTitle={false} />
      <section className="flex flex-col items-center gap-2.5 rounded-xl bg-primary px-5 py-7 text-center text-primary-foreground">
        <CircleCheck aria-hidden="true" className="size-16" strokeWidth={2} />
        <h1 className="text-[26px] leading-tight font-bold">
          {RECORDED_TITLE}
        </h1>
        {view.paidLine !== null && (
          <p className="text-sm md:text-base">{view.paidLine}</p>
        )}
      </section>
      {view.outstandingLine !== null && (
        <p className="text-sm font-semibold md:text-base">
          {view.outstandingLine}
        </p>
      )}
      <PromiseCard date={view.date} amount={view.amount} />
      {onEdit !== null && (
        <Button
          type="button"
          variant="outline"
          size="lg"
          className="w-full font-semibold"
          onClick={onEdit}
        >
          {EDIT_PROMISE_TEXT}
        </Button>
      )}
    </>
  );
}
