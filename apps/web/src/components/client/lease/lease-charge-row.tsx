"use client";

import { useId } from "react";
import { useController, type UseFormReturn } from "react-hook-form";

import { FieldError } from "@/components/client/form/field-error";
import { RupiahInput } from "@/components/client/form/rupiah-input";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import type { LeaseChargesInput } from "@/lib/api/generated/model";
import type { ChargeEntry, LeaseChargesFormValues } from "./type";
import {
  chargeAmountLabel,
  chargeField,
  NO_CHARGE_ENTRY,
  withChargeAmount,
  withChargeChecked,
} from "./utils";

type LeaseChargeRowProps = {
  // The charges form: the row is its field "charges.<chargeTypeId>".
  form: UseFormReturn<LeaseChargesFormValues, unknown, LeaseChargesInput>;
  chargeTypeId: string;
  name: string;
  disabled: boolean;
};

// One charge type in "Biaya tambahan": a checkbox with its name, and its monthly amount in
// rupiah. The error under it is the row's own check or the API's on that charge; editing the
// row clears it, and focusing the row focuses its amount.
export function LeaseChargeRow({
  form,
  chargeTypeId,
  name,
  disabled,
}: LeaseChargeRowProps) {
  const id = useId();
  const amountId = `${id}-amount`;
  const checkboxId = `${id}-checked`;
  const errorId = `${id}-error`;
  const fieldName = chargeField(chargeTypeId);
  const { field, fieldState } = useController({
    control: form.control,
    name: fieldName,
    defaultValue: NO_CHARGE_ENTRY,
  });
  const entry = field.value;
  const error = fieldState.error?.message;

  function change(next: ChargeEntry) {
    field.onChange(next);
    form.clearErrors(fieldName);
  }

  return (
    <div className="flex flex-col gap-2 border-t py-2.5">
      <div className="flex items-center gap-3">
        <Label
          htmlFor={checkboxId}
          className="min-h-11 min-w-0 flex-1 gap-3 text-base leading-normal font-semibold md:text-[17px]"
        >
          <Checkbox
            id={checkboxId}
            checked={entry.checked}
            onCheckedChange={(checked) =>
              change(withChargeChecked(entry, checked === true))
            }
            disabled={disabled}
            className="size-[22px]"
          />
          <span className="min-w-0 break-words">{name}</span>
        </Label>
        <RupiahInput
          inputRef={field.ref}
          id={amountId}
          name={`charge-${chargeTypeId}`}
          placeholder="0"
          ariaLabel={chargeAmountLabel(name)}
          value={entry.amount}
          onChange={(text) => change(withChargeAmount(entry, text))}
          invalid={error !== undefined}
          describedBy={error !== undefined ? errorId : undefined}
          disabled={disabled}
          className="h-12 w-[150px] shrink-0 md:w-[190px]"
        />
      </div>
      {error !== undefined && <FieldError id={errorId} message={error} />}
    </div>
  );
}
