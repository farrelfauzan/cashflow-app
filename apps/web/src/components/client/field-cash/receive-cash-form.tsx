"use client";

import { useId } from "react";
import { Controller } from "react-hook-form";

import { FormAlert } from "@/components/client/form/form-alert";
import { RupiahField } from "@/components/client/form/rupiah-field";
import { LeaseInfoNote } from "@/components/client/lease/lease-info-note";

import type { ReceiveCash, ReceiveCashTenant } from "./type";
import {
  AMOUNT_HINT,
  AMOUNT_LABEL,
  FIFO_NOTE,
  canSave,
  saveLabel,
} from "./utils";

import { CashOwedBox } from "./cash-owed-box";
import { CashSaveFooter } from "./cash-save-footer";
import { ReceiptPhotoField } from "./receipt-photo-field";

type ReceiveCashFormProps = {
  cash: ReceiveCash;
  tenant: ReceiveCashTenant;
  subtitle: string;
};

// "Terima tunai" before saving (board HP-Tagihan, 2): what the tenant owes, "Uang yang diterima"
// (empty when opened, digits only, any part), the receipt photo, the FIFO note, and Simpan naming
// the amount.
export function ReceiveCashForm({
  cash,
  tenant,
  subtitle,
}: ReceiveCashFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const { form, photo, saving, locked } = cash;

  return (
    <>
      <form
        id={formId}
        noValidate
        onSubmit={cash.submit}
        data-slot="form-sheet-body"
        className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-5 py-4 md:gap-[18px] md:px-6 md:py-5"
      >
        {cash.alert !== undefined && <FormAlert message={cash.alert} />}
        <CashOwedBox tenant={tenant} />
        <Controller
          control={form.control}
          name="amount"
          render={({ field: amount, fieldState }) => (
            <RupiahField
              inputRef={amount.ref}
              label={AMOUNT_LABEL}
              name="amount"
              placeholder="0"
              hint={AMOUNT_HINT}
              value={amount.value}
              onChange={(text) => {
                amount.onChange(text);
                form.clearErrors("amount");
              }}
              error={fieldState.error?.message}
              disabled={saving || locked}
            />
          )}
        />
        <ReceiptPhotoField
          photo={photo}
          subtitle={subtitle}
          locked={locked}
          saving={saving}
        />
        <LeaseInfoNote text={FIFO_NOTE} />
      </form>
      <CashSaveFooter
        formId={formId}
        label={saveLabel(photo.state, cash.amountText, saving)}
        canSave={canSave(photo.state, saving)}
        saving={saving}
      />
    </>
  );
}
