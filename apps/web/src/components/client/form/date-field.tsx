"use client";

import { Suspense, useId, type RefCallback } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

import { LazyDatePicker } from "./lazy-date-picker";
import { NativeDateInput } from "./native-date-input";
import { StaticDateTrigger } from "./static-date-trigger";
import type { DateControlProps } from "./type";
import { useWideLayout } from "./use-wide-layout";
import { FIELD_HINT_CLASS, FIELD_LABEL_CLASS } from "./utils";

type DateFieldProps = {
  // For moving focus from outside, such as react-hook-form's field ref.
  inputRef: RefCallback<HTMLElement>;
  label: string;
  // The input's name, the API field it fills ("startDate").
  name: string;
  // YYYY-MM-DD, "" for none.
  value: string;
  // The first and last day offered (YYYY-MM-DD); the schema still checks the value.
  min?: string;
  max?: string;
  onChange: (date: string) => void;
  // Under the field; null for none.
  hint: string | null;
  error: string | undefined;
  disabled: boolean;
  className?: string;
};

// A labelled calendar date, with its error and hint (board "iPad · Kolom tanggal dengan
// kalender"). On iPad and desktop it opens the shadcn calendar in a popover; on a phone it is
// the device's own date input, whose picker the operating system draws. Chosen by width, not by
// device: until the width is known (server render, hydration) both are rendered and CSS shows
// the right one, so the field never changes under the user's eyes.
export function DateField({
  inputRef,
  label,
  name,
  value,
  min,
  max,
  onChange,
  hint,
  error,
  disabled,
  className,
}: DateFieldProps) {
  const id = useId();
  const wide = useWideLayout();
  const errorId = `${id}-error`;
  const hintId = `${id}-hint`;
  const labelId = `${id}-label`;
  const describedBy = [
    error !== undefined ? errorId : null,
    hint !== null ? hintId : null,
  ]
    .filter((part) => part !== null)
    .join(" ");
  const control: DateControlProps = {
    id: `${id}-date`,
    labelId,
    name,
    controlRef: inputRef,
    value,
    min,
    max,
    onChange,
    invalid: error !== undefined,
    describedBy: describedBy === "" ? undefined : describedBy,
    disabled,
  };
  const fallback = (
    <StaticDateTrigger
      value={value}
      invalid={error !== undefined}
      disabled={disabled}
      className={wide === null ? "hidden md:flex" : undefined}
    />
  );

  return (
    <div className={cn("flex min-w-0 flex-col gap-2", className)}>
      <Label id={labelId} htmlFor={control.id} className={FIELD_LABEL_CLASS}>
        {label}
      </Label>
      {wide !== true && (
        <NativeDateInput
          {...control}
          className={wide === null ? "md:hidden" : undefined}
        />
      )}
      {wide === null && fallback}
      {wide === true && (
        <Suspense fallback={fallback}>
          <LazyDatePicker {...control} />
        </Suspense>
      )}
      {error !== undefined && <FieldError id={errorId} message={error} />}
      {hint !== null && (
        <p id={hintId} className={FIELD_HINT_CLASS}>
          {hint}
        </p>
      )}
    </div>
  );
}
