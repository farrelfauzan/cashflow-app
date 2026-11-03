"use client";

import { useId } from "react";
import { Controller, type UseFormReturn } from "react-hook-form";

import { FieldError } from "@/components/client/form/field-error";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import type { LeaseChargesInput } from "@/lib/api/generated/model";
import type { LeaseChargesFormValues } from "./type";
import { meteredField, METERED_ROW_HINT } from "./utils";

type LeaseMeteredRowProps = {
  // The charges form: the row is its field "metered.<chargeTypeId>".
  form: UseFormReturn<LeaseChargesFormValues, unknown, LeaseChargesInput>;
  chargeTypeId: string;
  name: string;
  disabled: boolean;
};

// One metered charge type in "Dari meteran": its name, the hint that its amount is recorded every
// month, and a switch. The switch has no amount, so the row has no check of its own; an error
// under it is the API's on that type, and flipping the switch clears it.
export function LeaseMeteredRow({
  form,
  chargeTypeId,
  name,
  disabled,
}: LeaseMeteredRowProps) {
  const id = useId();
  const switchId = `${id}-switch`;
  const nameId = `${id}-name`;
  const hintId = `${id}-hint`;
  const errorId = `${id}-error`;
  const fieldName = meteredField(chargeTypeId);

  return (
    <Controller
      control={form.control}
      name={fieldName}
      defaultValue={false}
      render={({ field, fieldState }) => {
        const error = fieldState.error?.message;
        return (
          <div className="flex flex-col gap-2 border-t py-2.5">
            <div className="flex items-center gap-3">
              <Label
                htmlFor={switchId}
                className="min-h-11 min-w-0 flex-1 flex-col items-start justify-center gap-0.5 text-base leading-normal font-semibold md:text-[17px]"
              >
                <span id={nameId} className="min-w-0 break-words">
                  {name}
                </span>
                <span
                  id={hintId}
                  className="text-[15px] font-normal text-muted-foreground md:text-base"
                >
                  {METERED_ROW_HINT}
                </span>
              </Label>
              <Switch
                ref={field.ref}
                id={switchId}
                checked={field.value === true}
                onCheckedChange={(on) => {
                  field.onChange(on);
                  form.clearErrors(fieldName);
                }}
                disabled={disabled}
                aria-labelledby={nameId}
                aria-describedby={
                  error !== undefined ? `${hintId} ${errorId}` : hintId
                }
                aria-invalid={error !== undefined}
                className="relative h-7 w-12 after:absolute after:-inset-x-1 after:-inset-y-2.5 after:content-[''] data-[size=default]:h-7 data-[size=default]:w-12 [&>span]:size-6!"
              />
            </div>
            {error !== undefined && <FieldError id={errorId} message={error} />}
          </div>
        );
      }}
    />
  );
}
