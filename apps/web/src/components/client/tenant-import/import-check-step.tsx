"use client";

import { useMemo, useState } from "react";

import { FormAlert } from "@/components/client/form/form-alert";
import { Button } from "@/components/ui/button";
import type { ImportResult } from "@/lib/api/generated/model";
import type { RowFilter } from "./type";
import {
  CHANGED_MESSAGE,
  commitButton,
  filterChips,
  filterRows,
  importRowView,
} from "./utils";

import { ImportCheckSummary } from "./import-check-summary";
import { ImportCommitButton } from "./import-commit-button";
import { ImportPhoneFooter } from "./import-phone-footer";
import { ImportRowFilters } from "./import-row-filters";
import { ImportRowList } from "./import-row-list";
import { ImportWarnings } from "./import-warnings";

type ImportCheckStepProps = {
  result: ImportResult;
  // The commit came back unsaved because the data changed since the check.
  changed: boolean;
  // The commit failed (a denial, no connection): the API's message.
  commitError: string | null;
  busy: boolean;
  onCommit: () => void;
  onRestart: () => void;
};

// Step 2, "Periksa" (boards Impor-Cek*.dc.html and Impor-Siap*.dc.html): the dry run's summary,
// the file's warnings, and every row with what will happen to it. Nothing is saved yet.
export function ImportCheckStep({
  result,
  changed,
  commitError,
  busy,
  onCommit,
  onRestart,
}: ImportCheckStepProps) {
  const [filter, setFilter] = useState<RowFilter>("all");
  const chips = filterChips(result.rows);
  // A filter whose rows are gone (after a new check) falls back to every row.
  const active = chips.some((chip) => chip.filter === filter) ? filter : "all";
  const rows = useMemo(
    () => filterRows(result.rows, active).map(importRowView),
    [result.rows, active],
  );
  const commit = commitButton(result.summary);

  return (
    <div className="flex flex-col gap-3.5 lg:gap-4">
      {changed && <FormAlert message={CHANGED_MESSAGE} />}
      {commitError !== null && <FormAlert message={commitError} />}
      <div className="flex flex-col gap-3.5 lg:flex-row-reverse lg:items-start lg:gap-4">
        <div className="lg:sticky lg:top-6 lg:w-[300px] lg:shrink-0">
          <ImportCheckSummary
            summary={result.summary}
            busy={busy}
            onCommit={onCommit}
          />
        </div>
        <div className="flex min-w-0 flex-1 flex-col gap-3">
          {result.warnings.length > 0 && (
            <ImportWarnings warnings={result.warnings} />
          )}
          <ImportRowFilters
            chips={chips}
            active={active}
            onChange={setFilter}
          />
          <ImportRowList rows={rows} />
        </div>
      </div>
      <ImportPhoneFooter>
        {commit.enabled ? (
          <ImportCommitButton
            commit={commit}
            busy={busy}
            onCommit={onCommit}
            className="h-13"
          />
        ) : (
          <Button
            type="button"
            onClick={onRestart}
            className="h-13 w-full rounded-xl px-5 text-[17px] font-semibold"
          >
            Pilih berkas yang sudah diperbaiki
          </Button>
        )}
      </ImportPhoneFooter>
    </div>
  );
}
