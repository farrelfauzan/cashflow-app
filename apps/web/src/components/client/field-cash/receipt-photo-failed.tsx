"use client";

import { RotateCcw } from "lucide-react";

import { Button } from "@/components/ui/button";

import type { ChosenPhoto } from "./type";
import {
  PHOTO_FAILED_TITLE,
  PHOTO_OTHER_TEXT,
  PHOTO_RETRY_TEXT,
} from "./utils";

import { ReceiptPhotoThumb } from "./receipt-photo-thumb";

type ReceiptPhotoFailedProps = {
  photo: ChosenPhoto;
  // "Sinyal hilang. Uang belum tercatat." or another reason, with the same ending.
  message: string;
  onRetry: () => void;
  onPickOther: () => void;
};

// State 3, "Foto belum terkirim": nothing is recorded. "Coba lagi" sends the same photo;
// "Pilih foto lain" replaces it. There is no other way to save.
export function ReceiptPhotoFailed({
  photo,
  message,
  onRetry,
  onPickOther,
}: ReceiptPhotoFailedProps) {
  return (
    <div className="flex flex-col gap-2.5 rounded-xl border-[1.5px] border-warning bg-card px-3 py-2.5">
      <div className="flex items-center gap-3">
        <ReceiptPhotoThumb photo={photo} />
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="text-[15px] font-semibold text-warning-strong md:text-base">
            {PHOTO_FAILED_TITLE}
          </span>
          <span className="text-sm text-muted-foreground md:text-base">
            {message}
          </span>
        </div>
      </div>
      <div className="flex gap-2">
        <Button
          type="button"
          variant="outline"
          onClick={onPickOther}
          className="flex-1 rounded-xl border-[1.5px] bg-card px-3 text-[15px] font-semibold md:text-base"
        >
          {PHOTO_OTHER_TEXT}
        </Button>
        <Button
          type="button"
          variant="caution-outline"
          onClick={onRetry}
          className="flex-1 rounded-xl border-[1.5px] bg-card px-3 text-[15px] font-semibold md:text-base"
        >
          <RotateCcw aria-hidden="true" className="size-[18px]" />
          {PHOTO_RETRY_TEXT}
        </Button>
      </div>
    </div>
  );
}
