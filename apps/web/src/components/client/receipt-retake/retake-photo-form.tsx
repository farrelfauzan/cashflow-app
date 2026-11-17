"use client";

import { useId } from "react";

import { CashSaveFooter } from "@/components/client/field-cash/cash-save-footer";
import { ReceiptPhotoField } from "@/components/client/field-cash/receipt-photo-field";
import { FormAlert } from "@/components/client/form/form-alert";

import type { RetakePhoto } from "./type";
import {
  RETAKE_EXPLAIN,
  RETAKE_PAYMENT_LABEL,
  canSaveRetake,
  retakeSaveLabel,
} from "./utils";

type RetakePhotoFormProps = {
  retake: RetakePhoto;
  // "Tunai Rp1.000.000 · 10.42".
  summary: string;
  // The sheet's subtitle, repeated over the photo when it is shown whole.
  subtitle: string;
};

// The body of "Ambil ulang foto kwitansi": which payment, why, and Terima tunai's "Foto kwitansi
// (wajib)" field in its states; then Batal and the one primary, "Simpan foto", which works once
// the new photo has arrived.
export function RetakePhotoForm({
  retake,
  summary,
  subtitle,
}: RetakePhotoFormProps) {
  const id = useId();
  const formId = `${id}-form`;

  return (
    <>
      <form
        id={formId}
        noValidate
        onSubmit={retake.submit}
        data-slot="form-sheet-body"
        className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-5 py-4 md:gap-[18px] md:px-6 md:py-5"
      >
        {retake.alert !== null && <FormAlert message={retake.alert} />}
        <section
          aria-label={RETAKE_PAYMENT_LABEL}
          className="flex flex-col gap-1 rounded-xl bg-muted px-3.5 py-3"
        >
          <span className="text-[15px] font-semibold tabular-nums md:text-base">
            {summary}
          </span>
          <p className="text-sm text-muted-foreground md:text-base">
            {RETAKE_EXPLAIN}
          </p>
        </section>
        <ReceiptPhotoField
          photo={retake.photo}
          subtitle={subtitle}
          locked={retake.locked}
          saving={retake.saving}
        />
      </form>
      <CashSaveFooter
        formId={formId}
        label={retakeSaveLabel(retake.photo.state.kind, retake.saving)}
        canSave={canSaveRetake(retake.photo.state.kind, retake.saving)}
        saving={retake.saving}
      />
    </>
  );
}
