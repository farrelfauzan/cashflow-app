"use client";

import { Progress } from "@/components/ui/progress";

import type { ChosenPhoto } from "./type";
import {
  PHOTO_PROGRESS_LABEL,
  PHOTO_SENDING_NOTE,
  uploadingLine,
} from "./utils";

import { ReceiptPhotoThumb } from "./receipt-photo-thumb";

type ReceiptPhotoUploadingProps = {
  photo: ChosenPhoto | null;
  percent: number | null;
};

// State 2, "Mengirim foto… 64%": the photo goes to storage. Simpan waits ("Menunggu foto…");
// Batal still closes the sheet and stops it.
export function ReceiptPhotoUploading({
  photo,
  percent,
}: ReceiptPhotoUploadingProps) {
  return (
    <div className="flex flex-col gap-2 rounded-xl border-[1.5px] bg-card px-3 py-2.5">
      <div className="flex items-center gap-3">
        <ReceiptPhotoThumb photo={photo} />
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="text-[15px] font-semibold tabular-nums md:text-base">
            {uploadingLine(percent)}
          </span>
          <span className="text-sm text-muted-foreground md:text-base">
            {PHOTO_SENDING_NOTE}
          </span>
        </div>
      </div>
      <Progress
        value={percent}
        aria-label={PHOTO_PROGRESS_LABEL}
        className="h-1.5 bg-secondary"
      />
    </div>
  );
}
