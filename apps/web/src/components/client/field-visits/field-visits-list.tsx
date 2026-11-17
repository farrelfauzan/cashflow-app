"use client";

import { useId } from "react";

import type { FieldVisitRowView, FieldVisitSections } from "./type";
import {
  ALL_DONE_TEXT,
  DONE_LIST_LABEL,
  DONE_TITLE,
  EMPTY_TEXT,
  LIST_LABEL,
} from "./utils";

import { FieldVisitDoneRow } from "./field-visit-done-row";
import { FieldVisitRow } from "./field-visit-row";

type FieldVisitsListProps = {
  sections: FieldVisitSections;
  // "dan 3 penyewa lainnya" when the API holds more than one page; null otherwise.
  moreText: string | null;
  canRecord: boolean;
  onRecord: (view: FieldVisitRowView) => void;
};

// The visits still open, in the API's order (the oldest visit first), then "Sudah dicatat hari
// ini". Nothing at all: one calm line.
export function FieldVisitsList({
  sections,
  moreText,
  canRecord,
  onRecord,
}: FieldVisitsListProps) {
  const doneTitleId = useId();
  const { open, done } = sections;
  if (open.length === 0 && done.length === 0) {
    return (
      <p className="rounded-[14px] border bg-card px-4 py-4 text-base">
        {EMPTY_TEXT}
      </p>
    );
  }
  return (
    <div className="flex flex-col gap-2.5 md:gap-3">
      {open.length === 0 ? (
        <p className="rounded-[14px] border bg-card px-4 py-4 text-base">
          {ALL_DONE_TEXT}
        </p>
      ) : (
        <ul aria-label={LIST_LABEL} className="flex flex-col gap-2.5 md:gap-3">
          {open.map((view) => (
            <FieldVisitRow
              key={view.tenantId}
              view={view}
              canRecord={canRecord}
              onRecord={onRecord}
            />
          ))}
        </ul>
      )}
      {done.length > 0 && (
        <section aria-labelledby={doneTitleId} className="flex flex-col gap-2">
          <h2
            id={doneTitleId}
            className="pt-1 text-sm font-semibold text-muted-foreground md:text-base"
          >
            {DONE_TITLE}
          </h2>
          <ul aria-label={DONE_LIST_LABEL} className="flex flex-col gap-2.5">
            {done.map((view) => (
              <FieldVisitDoneRow key={view.tenantId} view={view} />
            ))}
          </ul>
        </section>
      )}
      {moreText !== null && (
        <p className="text-sm text-muted-foreground md:text-base">{moreText}</p>
      )}
    </div>
  );
}
