"use client";

import { FileX } from "lucide-react";

import { BLOCKED_FILE_TEXT } from "./utils";

type BlockedFileTileProps = {
  // "Diperiksa Sabtu 14 Nov, 11.07 · sudah diamankan".
  scannedLine: string;
};

// In place of the photo, which is never shown: the file cannot be opened.
export function BlockedFileTile({ scannedLine }: BlockedFileTileProps) {
  return (
    <div className="flex items-center gap-3 rounded-xl border border-dashed border-warning bg-muted px-4 py-4">
      <span
        aria-hidden="true"
        className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-warning/15 text-warning"
      >
        <FileX className="size-5" />
      </span>
      <div className="flex flex-col gap-0.5">
        <span className="text-base font-semibold md:text-[17px]">
          {BLOCKED_FILE_TEXT}
        </span>
        <span className="text-sm text-muted-foreground md:text-base">
          {scannedLine}
        </span>
      </div>
    </div>
  );
}
