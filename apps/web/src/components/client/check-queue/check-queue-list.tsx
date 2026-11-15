"use client";

import type { CheckRowView } from "./type";
import { LIST_LABEL } from "./utils";

import { CheckQueueRow } from "./check-queue-row";

type CheckQueueListProps = {
  rows: readonly CheckRowView[];
  onOpen: (itemId: string) => void;
};

// The waiting items, oldest first (the API's order), in one bordered card on every size.
export function CheckQueueList({ rows, onOpen }: CheckQueueListProps) {
  return (
    <ul
      aria-label={LIST_LABEL}
      className="overflow-hidden rounded-2xl border bg-card"
    >
      {rows.map((row) => (
        <CheckQueueRow key={row.id} row={row} onOpen={() => onOpen(row.id)} />
      ))}
    </ul>
  );
}
