"use client";

import { Button } from "@/components/ui/button";

import type { ChosenPhoto } from "./type";
import {
  PHOTO_REPLACE_LABEL,
  PHOTO_REPLACE_TEXT,
  PHOTO_SENT_TITLE,
  PHOTO_VIEW_TEXT,
} from "./utils";

import { ReceiptPhotoThumb } from "./receipt-photo-thumb";

type ReceiptPhotoSentProps = {
  photo: ChosenPhoto;
  // "Ganti" is hidden once the photo went with a save whose answer was lost.
  canReplace: boolean;
  disabled: boolean;
  onView: () => void;
  onReplace: () => void;
};

// State 4, "Foto terkirim": Simpan works. A tap shows the photo whole; "Ganti" picks another
// before saving. The scan is not waited for.
export function ReceiptPhotoSent({
  photo,
  canReplace,
  disabled,
  onView,
  onReplace,
}: ReceiptPhotoSentProps) {
  return (
    <div className="flex items-center gap-1 rounded-xl border-[1.5px] border-primary bg-accent/50 py-1 pr-1 pl-2">
      <Button
        type="button"
        variant="ghost"
        onClick={onView}
        className="h-auto min-h-14 min-w-0 flex-1 justify-start gap-3 rounded-lg px-1 py-1.5 text-left whitespace-normal active:scale-100"
      >
        <ReceiptPhotoThumb photo={photo} />
        <span className="flex min-w-0 flex-col gap-0.5">
          <span className="text-[15px] font-semibold text-primary-strong md:text-base">
            {PHOTO_SENT_TITLE}
          </span>
          <span className="text-sm font-normal text-muted-foreground md:text-base">
            {PHOTO_VIEW_TEXT}
          </span>
        </span>
      </Button>
      {canReplace && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-label={PHOTO_REPLACE_LABEL}
          disabled={disabled}
          onClick={onReplace}
          className="rounded-lg text-[15px] font-semibold text-primary hover:text-primary md:text-base"
        >
          {PHOTO_REPLACE_TEXT}
        </Button>
      )}
    </div>
  );
}
