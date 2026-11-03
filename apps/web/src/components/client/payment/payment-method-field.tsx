"use client";

import { useId, type Ref } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { FIELD_LABEL_CLASS } from "@/components/client/form/utils";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import type { PaymentInputMethod } from "@/lib/api/generated/model";
import { isPaymentMethod, PAYMENT_METHOD_TEXT, PAYMENT_METHODS } from "./utils";

type PaymentMethodFieldProps = {
  // For moving focus to the group from outside (an API error on "method").
  inputRef: Ref<HTMLDivElement>;
  value: PaymentInputMethod;
  onChange: (method: PaymentInputMethod) => void;
  error: string | undefined;
  disabled: boolean;
};

// "Cara bayar": "Tunai" or "Transfer bank", as two large radio cards, each a whole label at
// least 56px tall.
export function PaymentMethodField({
  inputRef,
  value,
  onChange,
  error,
  disabled,
}: PaymentMethodFieldProps) {
  const id = useId();
  const errorId = `${id}-error`;

  return (
    <div className="flex flex-col gap-2">
      <span id={`${id}-legend`} className={FIELD_LABEL_CLASS}>
        Cara bayar
      </span>
      <RadioGroup
        ref={inputRef}
        tabIndex={-1}
        aria-labelledby={`${id}-legend`}
        aria-describedby={error !== undefined ? errorId : undefined}
        aria-invalid={error !== undefined}
        value={value}
        onValueChange={(next) => {
          if (isPaymentMethod(next)) onChange(next);
        }}
        disabled={disabled}
        className="grid-cols-2 gap-2.5 outline-none"
      >
        {PAYMENT_METHODS.map((method) => (
          <Label
            key={method}
            htmlFor={`${id}-${method}`}
            className="min-h-14 gap-3 rounded-xl border-[1.5px] bg-card px-3.5 text-base leading-normal font-semibold has-[button[data-state=checked]]:border-2 has-[button[data-state=checked]]:border-primary has-[button[data-state=checked]]:bg-accent md:text-[17px]"
          >
            <RadioGroupItem
              id={`${id}-${method}`}
              value={method}
              className="size-[22px]"
            />
            {PAYMENT_METHOD_TEXT[method]}
          </Label>
        ))}
      </RadioGroup>
      {error !== undefined && <FieldError id={errorId} message={error} />}
    </div>
  );
}
