"use client";

import { RotateCcw } from "lucide-react";

import { PHOTO_RETRY_TEXT } from "@/components/client/field-cash/utils";
import { Button } from "@/components/ui/button";

import { FIELD_PROOF_FAILED_TITLE } from "./utils";

type FieldProofFailedProps = {
  // "Sinyal hilang. Foto masih di sini." or the API's message.
  message: string;
  onRetry: () => void;
};

// "Bukti belum terkirim" (board HP-Bukti-Karyawan, 4) under the photo: "Coba lagi" sends the
// same photo again. Nothing reached the admin.
export function FieldProofFailed({ message, onRetry }: FieldProofFailedProps) {
  return (
    <div
      role="alert"
      className="flex flex-col gap-2.5 rounded-xl border-[1.5px] border-warning bg-card px-3 py-2.5"
    >
      <div className="flex flex-col gap-0.5">
        <span className="text-[15px] font-semibold text-warning-strong md:text-base">
          {FIELD_PROOF_FAILED_TITLE}
        </span>
        <span className="text-sm text-muted-foreground md:text-base">
          {message}
        </span>
      </div>
      <Button
        type="button"
        variant="caution-outline"
        onClick={onRetry}
        className="h-12 w-full rounded-xl border-[1.5px] bg-card px-3 text-[15px] font-semibold md:text-base"
      >
        <RotateCcw aria-hidden="true" className="size-[18px]" />
        {PHOTO_RETRY_TEXT}
      </Button>
    </div>
  );
}
