"use client";

import { useId } from "react";

import type { PastDepositView } from "./type";
import { GAP_NOTE, hasGap, HISTORY_TITLE } from "./utils";

import { PastDepositRow } from "./past-deposit-row";

type PastDepositListProps = {
  // The counted deposits of the first page, newest first.
  views: readonly PastDepositView[];
};

// "Setoran sebelumnya": the deposits an admin has counted, and when one of them had a gap, what
// happens to it.
export function PastDepositList({ views }: PastDepositListProps) {
  const titleId = useId();
  return (
    <section aria-labelledby={titleId} className="flex flex-col gap-3">
      <h2 id={titleId} className="mt-1.5 text-lg font-bold md:text-xl">
        {HISTORY_TITLE}
      </h2>
      <ul
        aria-labelledby={titleId}
        className="flex flex-col rounded-[14px] border bg-card"
      >
        {views.map((view) => (
          <PastDepositRow key={view.id} view={view} />
        ))}
      </ul>
      {hasGap(views) && (
        <p className="text-sm leading-normal text-muted-foreground md:text-base">
          {GAP_NOTE}
        </p>
      )}
    </section>
  );
}
