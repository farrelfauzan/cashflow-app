"use client";

import type { CorrectionRowView } from "./type";
import {
  ACCOUNT_CARD_CLASS,
  CORRECTIONS_LIST_LABEL,
  CORRECTIONS_TITLE,
  NO_CORRECTIONS_TEXT,
} from "./utils";

import { CorrectionRow } from "./correction-row";

type CorrectionListProps = {
  corrections: readonly CorrectionRowView[];
};

// "Koreksi nominal": every correction of the tenant's invoices and payments, newest first. With
// none, the board's one footer line, and the heading only for screen readers.
export function CorrectionList({ corrections }: CorrectionListProps) {
  if (corrections.length === 0) {
    return (
      <section>
        <h2 className="sr-only">{CORRECTIONS_TITLE}</h2>
        <p className="text-[15px] text-muted-foreground md:text-base">
          {NO_CORRECTIONS_TEXT}
        </p>
      </section>
    );
  }
  return (
    <section className={ACCOUNT_CARD_CLASS}>
      <h2 className="text-xl font-bold md:text-2xl">{CORRECTIONS_TITLE}</h2>
      <ul aria-label={CORRECTIONS_LIST_LABEL} className="flex flex-col gap-3">
        {corrections.map((correction) => (
          <CorrectionRow key={correction.id} correction={correction} />
        ))}
      </ul>
    </section>
  );
}
