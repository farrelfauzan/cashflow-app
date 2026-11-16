"use client";

import { Camera } from "lucide-react";

import { Button } from "@/components/ui/button";

import { FIELD_PROOF_PICK_NOTE, FIELD_PROOF_PICK_TEXT } from "./utils";

type FieldProofPickProps = {
  describedBy: string;
  onPick: () => void;
};

// No photo yet (board HP-Bukti-Karyawan, 2): the large dashed "Ambil foto bukti transfer", which
// opens the phone's camera or gallery.
export function FieldProofPick({ describedBy, onPick }: FieldProofPickProps) {
  return (
    <Button
      type="button"
      variant="outline"
      aria-describedby={describedBy}
      onClick={onPick}
      className="h-auto min-h-56 w-full flex-1 flex-col gap-2.5 rounded-xl border-[1.5px] border-dashed border-primary bg-accent/50 text-base font-bold text-primary shadow-none hover:bg-accent hover:text-primary active:bg-accent md:h-[min(52dvh,420px)] md:flex-none"
    >
      <Camera aria-hidden="true" className="size-7" />
      {FIELD_PROOF_PICK_TEXT}
      <span className="text-sm font-normal text-muted-foreground md:text-base">
        {FIELD_PROOF_PICK_NOTE}
      </span>
    </Button>
  );
}
