"use client";

import { Info } from "lucide-react";

import {
  COLUMN_NOTES,
  LIMITS_TEXT,
  OLD_DUE_DAY_NOTE,
  OPTIONAL_COLUMNS,
  REQUIRED_COLUMNS,
  TEMPLATE_EXAMPLE_TEXT,
} from "./utils";

import { ImportColumnChips } from "./import-column-chips";
import { TemplateDownloadLink } from "./template-download-link";

type ImportFormatCardProps = {
  orgId: string;
};

// "Format berkas": the template, the columns by their Indonesian names, and the limits.
export function ImportFormatCard({ orgId }: ImportFormatCardProps) {
  return (
    <section
      aria-labelledby="import-format-title"
      className="flex flex-col gap-3 rounded-2xl border bg-card p-[18px]"
    >
      <h2 id="import-format-title" className="text-[19px] font-bold">
        Format berkas
      </h2>
      <TemplateDownloadLink
        orgId={orgId}
        label="Unduh template CSV"
        variant="outline"
        className="w-full"
      />
      <p className="text-sm leading-normal text-muted-foreground md:text-base">
        {TEMPLATE_EXAMPLE_TEXT}
      </p>
      <ImportColumnChips title="Kolom wajib" columns={REQUIRED_COLUMNS} />
      <ImportColumnChips
        title="Kolom boleh dikosongkan"
        columns={OPTIONAL_COLUMNS}
      />
      {COLUMN_NOTES.map((note) => (
        <p
          key={note}
          className="text-sm leading-normal text-muted-foreground md:text-base"
        >
          {note}
        </p>
      ))}
      <div className="flex items-start gap-2.5 rounded-xl bg-secondary px-3.5 py-3">
        <Info
          aria-hidden="true"
          className="mt-0.5 size-[18px] shrink-0 text-muted-foreground"
          strokeWidth={2.2}
        />
        <p className="min-w-0 flex-1 text-sm leading-normal md:text-base">
          {OLD_DUE_DAY_NOTE}
        </p>
      </div>
      <p className="text-sm leading-normal text-muted-foreground md:text-base">
        {LIMITS_TEXT}
      </p>
    </section>
  );
}
