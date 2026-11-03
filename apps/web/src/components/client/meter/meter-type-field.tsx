"use client";

import { useId, type Ref } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { FIELD_LABEL_CLASS } from "@/components/client/form/utils";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import type { LeaseMeteredType } from "@/lib/api/generated/model";

type MeterTypeFieldProps = {
  // For moving focus to the group from outside (an API error on "chargeTypeId").
  inputRef: Ref<HTMLDivElement>;
  // The lease's metered types (Listrik, Air), by name.
  types: readonly LeaseMeteredType[];
  value: string;
  onChange: (chargeTypeId: string) => void;
  error: string | undefined;
  disabled: boolean;
};

// "Jenis biaya": one large radio card per metered type of the lease, each at least 56px tall.
export function MeterTypeField({
  inputRef,
  types,
  value,
  onChange,
  error,
  disabled,
}: MeterTypeFieldProps) {
  const id = useId();
  const errorId = `${id}-error`;

  return (
    <div className="flex flex-col gap-2">
      <span id={`${id}-legend`} className={FIELD_LABEL_CLASS}>
        Jenis biaya
      </span>
      <RadioGroup
        ref={inputRef}
        tabIndex={-1}
        aria-labelledby={`${id}-legend`}
        aria-describedby={error !== undefined ? errorId : undefined}
        aria-invalid={error !== undefined}
        value={value}
        onValueChange={onChange}
        disabled={disabled}
        className="grid-cols-2 gap-2.5 outline-none"
      >
        {types.map((type) => (
          <Label
            key={type.chargeTypeId}
            htmlFor={`${id}-${type.chargeTypeId}`}
            className="min-h-14 gap-3 rounded-xl border-[1.5px] bg-card px-3.5 text-base leading-normal font-semibold has-[button[data-state=checked]]:border-2 has-[button[data-state=checked]]:border-primary has-[button[data-state=checked]]:bg-accent md:text-[17px]"
          >
            <RadioGroupItem
              id={`${id}-${type.chargeTypeId}`}
              value={type.chargeTypeId}
              className="size-[22px]"
            />
            {type.name}
          </Label>
        ))}
      </RadioGroup>
      {error !== undefined && <FieldError id={errorId} message={error} />}
    </div>
  );
}
