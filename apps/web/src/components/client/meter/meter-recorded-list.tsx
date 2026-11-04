"use client";

import type { RecordedMeterRowView } from "./type";
import {
  RECORDED_EMPTY_TEXT,
  RECORDED_LOADING_TEXT,
  RECORDED_TITLE,
} from "./utils";

import { MeterRecordedRow } from "./meter-recorded-row";

type MeterRecordedListProps = {
  // null while the list loads.
  rows: readonly RecordedMeterRowView[] | null;
  // A message when the list could not be loaded.
  error: string | null;
  onOpen: (row: RecordedMeterRowView) => void;
};

// "Sudah dicatat": the lease's recorded amounts, newest month first (the first page).
export function MeterRecordedList({
  rows,
  error,
  onOpen,
}: MeterRecordedListProps) {
  let body;
  if (error !== null) {
    body = (
      <p className="text-[15px] text-muted-foreground md:text-base">{error}</p>
    );
  } else if (rows === null) {
    body = (
      <p className="text-[15px] text-muted-foreground md:text-base">
        {RECORDED_LOADING_TEXT}
      </p>
    );
  } else if (rows.length === 0) {
    body = (
      <p className="text-[15px] text-muted-foreground md:text-base">
        {RECORDED_EMPTY_TEXT}
      </p>
    );
  } else {
    body = (
      <ul aria-label={RECORDED_TITLE} className="flex flex-col">
        {rows.map((row) => (
          <MeterRecordedRow key={row.id} row={row} onOpen={() => onOpen(row)} />
        ))}
      </ul>
    );
  }
  return (
    <section className="flex flex-col gap-2 border-t pt-4">
      <h3 className="text-base font-bold md:text-[17px]">{RECORDED_TITLE}</h3>
      {body}
    </section>
  );
}
