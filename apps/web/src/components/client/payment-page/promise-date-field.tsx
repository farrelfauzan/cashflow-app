"use client";

import { useId, type Ref } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { FIELD_LABEL_CLASS, INPUT_CLASS } from "@/components/client/form/utils";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

import { PROMISE_DATE_LABEL } from "./utils";

type PromiseDateFieldProps = {
  inputRef: Ref<HTMLInputElement>;
  // YYYY-MM-DD, "" for none.
  value: string;
  // The days the calendar offers (YYYY-MM-DD); a typed date is still checked on save.
  min: string;
  max: string;
  onChange: (date: string) => void;
  error: string | null;
};

// "Tanggal pelunasan": the device's own date picker, limited to the days a promise may name. No
// calendar library, so the page loads nothing more.
export function PromiseDateField({
  inputRef,
  value,
  min,
  max,
  onChange,
  error,
}: PromiseDateFieldProps) {
  const id = useId();
  const errorId = `${id}-error`;

  return (
    <div className="flex min-w-0 flex-col gap-2">
      <Label htmlFor={`${id}-date`} className={FIELD_LABEL_CLASS}>
        {PROMISE_DATE_LABEL}
      </Label>
      <Input
        ref={inputRef}
        id={`${id}-date`}
        name="promisedDate"
        type="date"
        value={value}
        min={min}
        max={max}
        onChange={(event) => onChange(event.target.value)}
        aria-invalid={error !== null}
        aria-describedby={error === null ? undefined : errorId}
        className={INPUT_CLASS}
      />
      {error !== null && <FieldError id={errorId} message={error} />}
    </div>
  );
}
