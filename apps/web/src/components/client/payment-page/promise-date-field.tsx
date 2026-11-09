"use client";

import type { RefCallback } from "react";

import { DateField } from "@/components/client/form/date-field";

import { PROMISE_DATE_LABEL } from "./utils";

type PromiseDateFieldProps = {
  inputRef: RefCallback<HTMLElement>;
  // YYYY-MM-DD, "" for none.
  value: string;
  // The days the calendar offers (YYYY-MM-DD); a typed date is still checked on save.
  min: string;
  max: string;
  onChange: (date: string) => void;
  error: string | null;
};

// "Tanggal pelunasan", limited to the days a promise may name. On a phone it is the device's own
// date picker and the page loads no calendar code; on iPad and desktop, the calendar popover.
export function PromiseDateField({
  inputRef,
  value,
  min,
  max,
  onChange,
  error,
}: PromiseDateFieldProps) {
  return (
    <DateField
      inputRef={inputRef}
      label={PROMISE_DATE_LABEL}
      name="promisedDate"
      value={value}
      min={min}
      max={max}
      onChange={onChange}
      hint={null}
      error={error ?? undefined}
      disabled={false}
    />
  );
}
