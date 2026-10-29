"use client";

import { Check } from "lucide-react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import type { ImportSummary } from "@/lib/api/generated/model";
import { orgPath } from "@/lib/org-path";
import { createdCounts, doneText, doneTitle } from "./utils";

import { ImportPhoneFooter } from "./import-phone-footer";

type ImportDoneStepProps = {
  orgId: string;
  fileName: string;
  organizationName: string;
  summary: ImportSummary;
  onRestart: () => void;
};

// Step 3, "Selesai" (boards Impor-Hasil.dc.html and Impor-Hasil-Phone.dc.html): what the import
// saved, with the counts the API returned.
export function ImportDoneStep({
  orgId,
  fileName,
  organizationName,
  summary,
  onRestart,
}: ImportDoneStepProps) {
  const listHref = orgPath(orgId, "tenants");
  return (
    <div className="flex w-full flex-col gap-3 lg:max-w-[640px] lg:gap-[18px]">
      <div
        role="status"
        className="flex items-start gap-3 rounded-2xl bg-accent p-4 lg:items-center lg:gap-4 lg:p-5"
      >
        <span
          aria-hidden="true"
          className="flex size-10 shrink-0 items-center justify-center rounded-full bg-primary text-primary-foreground lg:size-[52px]"
        >
          <Check className="size-6" strokeWidth={3} />
        </span>
        <div className="flex flex-col gap-1">
          <h2 className="text-xl font-bold text-primary lg:text-[26px]">
            {doneTitle(summary)}
          </h2>
          <p className="text-[15px] leading-normal break-words md:text-[17px]">
            {doneText(fileName, organizationName)}
          </p>
        </div>
      </div>
      <section
        aria-labelledby="import-created-title"
        className="flex flex-col rounded-2xl border bg-card px-4 pt-1.5 pb-3.5 lg:px-[22px]"
      >
        <h3
          id="import-created-title"
          className="mt-2.5 mb-1 text-lg font-bold lg:mt-3.5 lg:mb-1.5 lg:text-[19px]"
        >
          Yang dibuat
        </h3>
        <dl>
          {createdCounts(summary).map((row) => (
            <div
              key={row.label}
              className="flex justify-between gap-2.5 border-t py-2 text-base lg:py-2.5 lg:text-lg"
            >
              <dt>{row.label}</dt>
              <dd className="font-bold">{row.value}</dd>
            </div>
          ))}
        </dl>
      </section>
      <div className="flex flex-col gap-2.5 lg:flex-row">
        <Button
          type="button"
          variant="outline"
          onClick={onRestart}
          className="h-12 w-full rounded-xl border-[1.5px] bg-card px-[18px] text-base font-semibold lg:w-auto lg:text-[17px]"
        >
          Impor berkas lain
        </Button>
        <Button
          asChild
          className="hidden h-12 rounded-xl px-5 text-[17px] font-semibold lg:inline-flex"
        >
          <Link href={listHref}>Lihat daftar penyewa</Link>
        </Button>
      </div>
      <ImportPhoneFooter>
        <Button
          asChild
          className="h-13 w-full rounded-xl px-5 text-[17px] font-semibold"
        >
          <Link href={listHref}>Lihat daftar penyewa</Link>
        </Button>
      </ImportPhoneFooter>
    </div>
  );
}
