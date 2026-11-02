"use client";

import { CorrectionRow } from "@/components/client/billing/correction-row";
import type { CorrectionRowView } from "@/components/client/billing/type";
import {
  CORRECTIONS_LIST_LABEL,
  CORRECTIONS_TITLE,
  NO_CORRECTIONS_TEXT,
} from "./utils";

type RecentCorrectionsProps = {
  corrections: readonly CorrectionRowView[];
};

// "Koreksi terbaru": the newest ten corrections in the organization, by members or by the
// system (sewa tidak diperpanjang, potong deposit), newest first.
export function RecentCorrections({ corrections }: RecentCorrectionsProps) {
  return (
    <section className="flex min-w-0 flex-col gap-3 rounded-[14px] border bg-card p-4 md:gap-4 md:rounded-2xl md:px-6 md:py-[22px]">
      <h2 className="text-xl font-bold md:text-2xl">{CORRECTIONS_TITLE}</h2>
      {corrections.length === 0 ? (
        <p className="text-[15px] text-muted-foreground md:text-base">
          {NO_CORRECTIONS_TEXT}
        </p>
      ) : (
        <ul aria-label={CORRECTIONS_LIST_LABEL} className="flex flex-col gap-3">
          {corrections.map((correction) => (
            <CorrectionRow key={correction.id} correction={correction} />
          ))}
        </ul>
      )}
    </section>
  );
}
