"use client";

import type { ImportSummary } from "@/lib/api/generated/model";
import { cn } from "@/lib/utils";
import { alsoCreatedText, commitButton, summaryTiles } from "./utils";

import { ImportCommitButton } from "./import-commit-button";
import { ImportSummaryTile } from "./import-summary-tile";

type ImportCheckSummaryProps = {
  summary: ImportSummary;
  busy: boolean;
  onCommit: () => void;
};

// "Ringkasan": the counts, what else will be created, and "Simpan N baris" with why it is or is
// not available. A commit that can go is the primary action: here on iPad, in the bottom bar on
// the phone (board Impor-Siap-Phone.dc.html). One that cannot shows disabled on both.
export function ImportCheckSummary({
  summary,
  busy,
  onCommit,
}: ImportCheckSummaryProps) {
  const commit = commitButton(summary);
  const also = alsoCreatedText(summary);
  return (
    <section
      aria-labelledby="import-summary-title"
      className="flex flex-col gap-3 lg:rounded-2xl lg:border lg:bg-card lg:p-4"
    >
      <h2
        id="import-summary-title"
        className="sr-only lg:not-sr-only lg:text-[19px] lg:font-bold"
      >
        Ringkasan
      </h2>
      <div className="grid grid-cols-2 gap-2.5">
        {summaryTiles(summary).map((tile) => (
          <ImportSummaryTile key={tile.label} tile={tile} />
        ))}
      </div>
      {also !== null && (
        <p className="text-sm leading-normal text-muted-foreground md:text-base">
          {also}
        </p>
      )}
      <ImportCommitButton
        commit={commit}
        busy={busy}
        onCommit={onCommit}
        className={cn(commit.enabled && "hidden lg:inline-flex")}
      />
      <p className="text-sm leading-normal text-muted-foreground md:text-base">
        {commit.hint}
      </p>
    </section>
  );
}
