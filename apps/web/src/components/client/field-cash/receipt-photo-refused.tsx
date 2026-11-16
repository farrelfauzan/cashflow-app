"use client";

import { Button } from "@/components/ui/button";

import type { ChosenPhoto } from "./type";
import { PHOTO_OTHER_TEXT, PHOTO_REFUSED_TITLE } from "./utils";

import { ReceiptPhotoThumb } from "./receipt-photo-thumb";

type ReceiptPhotoRefusedProps = {
  photo: ChosenPhoto;
  // The error under the box, which says why.
  errorId: string;
  onPickOther: () => void;
};

// The photo itself cannot be used (not a JPEG, PNG or WebP, larger than 10 MB, or refused by the
// scan): the reason goes under it and only another photo helps.
export function ReceiptPhotoRefused({
  photo,
  errorId,
  onPickOther,
}: ReceiptPhotoRefusedProps) {
  return (
    <div className="flex items-center gap-3 rounded-xl border-2 border-destructive bg-card px-3 py-2.5">
      <ReceiptPhotoThumb photo={photo} />
      <span className="min-w-0 flex-1 text-[15px] font-semibold md:text-base">
        {PHOTO_REFUSED_TITLE}
      </span>
      <Button
        type="button"
        variant="outline"
        aria-describedby={errorId}
        onClick={onPickOther}
        className="rounded-xl border-[1.5px] bg-card px-3 text-[15px] font-semibold md:text-base"
      >
        {PHOTO_OTHER_TEXT}
      </Button>
    </div>
  );
}
