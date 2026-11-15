"use client";

import { Controller, type UseFormReturn } from "react-hook-form";

import { DateField } from "@/components/client/form/date-field";
import { RupiahField } from "@/components/client/form/rupiah-field";
import { FIELD_HINT_CLASS } from "@/components/client/form/utils";

import type { ProofMatchOutput, ProofMatchValues } from "./type";
import {
  PROOF_AMOUNT_LABEL,
  PROOF_DATE_LABEL,
  PROOF_FIELDS_HINT,
} from "./utils";

type ProofMatchFieldsProps = {
  form: UseFormReturn<ProofMatchValues, unknown, ProofMatchOutput>;
  // "Today" in Jakarta: the latest day offered.
  today: string;
  disabled: boolean;
};

// "Jumlah yang masuk" (empty when the drawer opens, digits only) and "Tanggal masuk" (today by
// default, never later), side by side from md up and stacked on the phone, with the reminder to
// copy them from the bank statement rather than the photo.
export function ProofMatchFields({
  form,
  today,
  disabled,
}: ProofMatchFieldsProps) {
  return (
    <div className="flex flex-col gap-2">
      <div className="grid grid-cols-1 items-start gap-[18px] md:grid-cols-[1.2fr_1fr] md:gap-3">
        <Controller
          control={form.control}
          name="amount"
          render={({ field: amount, fieldState }) => (
            <RupiahField
              inputRef={amount.ref}
              label={PROOF_AMOUNT_LABEL}
              name="amount"
              placeholder="0"
              hint={null}
              value={amount.value}
              onChange={(text) => {
                amount.onChange(text);
                form.clearErrors("amount");
              }}
              error={fieldState.error?.message}
              disabled={disabled}
            />
          )}
        />
        <Controller
          control={form.control}
          name="receivedOn"
          render={({ field: receivedOn, fieldState }) => (
            <DateField
              inputRef={receivedOn.ref}
              label={PROOF_DATE_LABEL}
              name="receivedOn"
              value={receivedOn.value}
              max={today}
              onChange={(date) => {
                receivedOn.onChange(date);
                form.clearErrors("receivedOn");
              }}
              hint={null}
              error={fieldState.error?.message}
              disabled={disabled}
            />
          )}
        />
      </div>
      <p className={FIELD_HINT_CLASS}>{PROOF_FIELDS_HINT}</p>
    </div>
  );
}
