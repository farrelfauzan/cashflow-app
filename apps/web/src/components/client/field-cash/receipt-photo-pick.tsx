"use client";

import { Camera } from "lucide-react";

import { Button } from "@/components/ui/button";

import { PHOTO_PICK_TEXT } from "./utils";

type ReceiptPhotoPickProps = {
  describedBy: string;
  onPick: () => void;
};

// State 1, "Belum ada foto": the dashed "Ambil foto kwitansi", which opens the phone's camera or
// gallery.
export function ReceiptPhotoPick({
  describedBy,
  onPick,
}: ReceiptPhotoPickProps) {
  return (
    <Button
      type="button"
      variant="outline"
      aria-describedby={describedBy}
      onClick={onPick}
      className="h-auto min-h-18 w-full gap-2.5 rounded-xl border-[1.5px] border-dashed border-primary bg-accent/50 text-base font-bold text-primary shadow-none hover:bg-accent hover:text-primary active:bg-accent"
    >
      <Camera aria-hidden="true" className="size-6" />
      {PHOTO_PICK_TEXT}
    </Button>
  );
}
