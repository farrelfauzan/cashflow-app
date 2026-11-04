"use client";

import { useId, type Ref } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { INPUT_CLASS } from "@/components/client/form/utils";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { METER_NUMBER_MAX_DIGITS, meterDigits } from "./utils";

type MeterNumberInputProps = {
  inputRef: Ref<HTMLInputElement>;
  // "Awal" or "Akhir".
  label: string;
  // The API field it fills: "meterStart" or "meterEnd".
  name: string;
  value: string;
  onChange: (digits: string) => void;
  error: string | undefined;
  // The shared hint under both numbers.
  hintId: string;
  disabled: boolean;
};

// One meter reading: a numeric text field that keeps digits only.
export function MeterNumberInput({
  inputRef,
  label,
  name,
  value,
  onChange,
  error,
  hintId,
  disabled,
}: MeterNumberInputProps) {
  const id = useId();
  const errorId = `${id}-error`;
  return (
    <div className="flex min-w-0 flex-1 flex-col gap-1.5">
      <Label
        htmlFor={`${id}-input`}
        className="text-sm font-semibold text-muted-foreground md:text-base"
      >
        {label}
      </Label>
      <Input
        ref={inputRef}
        id={`${id}-input`}
        name={name}
        inputMode="numeric"
        autoComplete="off"
        maxLength={METER_NUMBER_MAX_DIGITS}
        value={value}
        onChange={(event) => onChange(meterDigits(event.target.value))}
        disabled={disabled}
        aria-invalid={error !== undefined}
        aria-describedby={error !== undefined ? `${errorId} ${hintId}` : hintId}
        className={INPUT_CLASS}
      />
      {error !== undefined && <FieldError id={errorId} message={error} />}
    </div>
  );
}
