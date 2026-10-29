"use client";

import { RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";

import { TemplateDownloadLink } from "./template-download-link";

type ImportCheckActionsProps = {
  orgId: string;
  // A row has an error: the primary action is choosing the fixed file.
  hasErrors: boolean;
  onRestart: () => void;
};

// The check's header buttons (boards Impor-Cek.dc.html and Impor-Siap.dc.html). With errors, on
// iPad: "Unduh template" and the primary "Pilih berkas yang sudah diperbaiki" (the phone has it in
// its bottom bar). Without: a text button "Pilih berkas lain", since "Simpan" is the primary one.
export function ImportCheckActions({
  orgId,
  hasErrors,
  onRestart,
}: ImportCheckActionsProps) {
  if (hasErrors) {
    return (
      <div className="hidden shrink-0 items-center gap-2 lg:flex">
        <TemplateDownloadLink
          orgId={orgId}
          label="Unduh template"
          variant="ghost"
        />
        <Button
          type="button"
          onClick={onRestart}
          className="h-12 rounded-xl px-5 text-[17px] font-semibold"
        >
          Pilih berkas yang sudah diperbaiki
        </Button>
      </div>
    );
  }
  return (
    <Button
      type="button"
      variant="ghost"
      onClick={onRestart}
      className="h-12 shrink-0 self-start rounded-xl px-3 text-base font-semibold text-primary lg:self-auto"
    >
      <RefreshCw aria-hidden="true" className="size-5" strokeWidth={2} />
      Pilih berkas lain
    </Button>
  );
}
