"use client";

import { FileUp } from "lucide-react";

import { Button } from "@/components/ui/button";
import { NO_FILE_TEXT } from "./utils";

type ImportFilePickerProps = {
  onPick: () => void;
};

// Step 1 before a file is chosen: the board's drop area, with "Pilih berkas CSV".
export function ImportFilePicker({ onPick }: ImportFilePickerProps) {
  return (
    <div className="flex flex-col items-center gap-3 rounded-2xl border-2 border-dashed border-border bg-background px-5 py-7 text-center lg:py-11">
      <FileUp
        aria-hidden="true"
        className="size-9 text-muted-foreground"
        strokeWidth={1.8}
      />
      <p className="text-[17px] font-semibold md:text-[19px]">{NO_FILE_TEXT}</p>
      <Button
        type="button"
        variant="outline"
        onClick={onPick}
        className="h-12 rounded-xl border-[1.5px] bg-card px-[18px] text-base font-semibold"
      >
        <FileUp aria-hidden="true" className="size-5" strokeWidth={2} />
        Pilih berkas CSV
      </Button>
    </div>
  );
}
