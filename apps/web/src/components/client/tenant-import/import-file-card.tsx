"use client";

import { FileText } from "lucide-react";

import { Button } from "@/components/ui/button";
import { fileSizeText } from "./utils";

type ImportFileCardProps = {
  name: string;
  sizeBytes: number;
  onChange: () => void;
};

// The chosen file: its name, its size, and "Ganti berkas". The rows are counted by the API's
// check, not here.
export function ImportFileCard({
  name,
  sizeBytes,
  onChange,
}: ImportFileCardProps) {
  return (
    <div className="flex items-center gap-3.5 rounded-xl border-[1.5px] border-border bg-card p-4">
      <span
        aria-hidden="true"
        className="flex size-12 shrink-0 items-center justify-center rounded-xl bg-accent text-primary"
      >
        <FileText className="size-6" strokeWidth={2} />
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="truncate text-base font-semibold md:text-lg">
          {name}
        </span>
        <span className="text-[15px] text-muted-foreground md:text-base">
          {fileSizeText(sizeBytes)}
        </span>
      </div>
      <Button
        type="button"
        variant="ghost"
        onClick={onChange}
        className="h-11 shrink-0 rounded-xl px-2 text-[15px] font-semibold text-primary md:text-base"
      >
        Ganti berkas
      </Button>
    </div>
  );
}
