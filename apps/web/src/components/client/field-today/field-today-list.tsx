"use client";

import type { FieldTodayRowView } from "./type";
import { EMPTY_TEXT, LIST_LABEL } from "./utils";

import { FieldTodayRow } from "./field-today-row";

type FieldTodayListProps = {
  views: readonly FieldTodayRowView[];
  // "dan 3 penyewa lainnya" when the API holds more than one page; null otherwise.
  moreText: string | null;
};

// The tenants to collect from today, in the API's order (the oldest due date first, then those
// who paid in cash today). Nothing to collect: one calm line.
export function FieldTodayList({ views, moreText }: FieldTodayListProps) {
  if (views.length === 0) {
    return (
      <p className="rounded-[14px] border bg-card px-4 py-4 text-base">
        {EMPTY_TEXT}
      </p>
    );
  }
  return (
    <div className="flex flex-col gap-2.5">
      <ul aria-label={LIST_LABEL} className="flex flex-col gap-2.5 md:gap-3">
        {views.map((view) => (
          <FieldTodayRow key={view.tenantId} view={view} />
        ))}
      </ul>
      {moreText !== null && (
        <p className="text-sm text-muted-foreground md:text-base">{moreText}</p>
      )}
    </div>
  );
}
