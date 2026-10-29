"use client";

import { useId, type RefObject } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

import { FIELD_HINT_CLASS, FIELD_LABEL_CLASS, INPUT_CLASS } from "./utils";

type DateFieldProps = {
  inputRef: RefObject<HTMLInputElement | null>;
  label: string;
  // The input's name, the API field it fills ("startDate").
  name: string;
  // YYYY-MM-DD, "" for none.
  value: string;
  onChange: (date: string) => void;
  // Under the field; null for none.
  hint: string | null;
  error: string | undefined;
  disabled: boolean;
  className?: string;
};

// A labelled calendar date of a panel form, with its error and hint.
export function DateField({
  inputRef,
  label,
  name,
  value,
  onChange,
  hint,
  error,
  disabled,
  className,
}: DateFieldProps) {
  const id = useId();
  const errorId = `${id}-error`;
  const hintId = `${id}-hint`;
  const describedBy = [
    error !== undefined ? errorId : null,
    hint !== null ? hintId : null,
  ]
    .filter((part) => part !== null)
    .join(" ");

  return (
    <div className={cn("flex min-w-0 flex-col gap-2", className)}>
      <Label htmlFor={`${id}-date`} className={FIELD_LABEL_CLASS}>
        {label}
      </Label>
      <Input
        ref={inputRef}
        id={`${id}-date`}
        name={name}
        type="date"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        disabled={disabled}
        aria-invalid={error !== undefined}
        aria-describedby={describedBy === "" ? undefined : describedBy}
        className={INPUT_CLASS}
      />
      {error !== undefined && <FieldError id={errorId} message={error} />}
      {hint !== null && (
        <p id={hintId} className={FIELD_HINT_CLASS}>
          {hint}
        </p>
      )}
    </div>
  );
}
