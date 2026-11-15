"use client";

import { useId } from "react";
import type { UseFormRegisterReturn } from "react-hook-form";

import { FieldError } from "@/components/client/form/field-error";
import {
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
  TEXTAREA_CLASS,
} from "@/components/client/form/utils";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";

import { NOTE_MAX_LENGTH } from "./utils";

type CheckNoteFieldProps = {
  label: string;
  placeholder: string;
  registration: UseFormRegisterReturn;
  error: string | undefined;
  disabled: boolean;
  // A line under the field; null for none.
  hint?: string | null;
};

// The required note of a drawer, with its error under it.
export function CheckNoteField({
  label,
  placeholder,
  registration,
  error,
  disabled,
  hint = null,
}: CheckNoteFieldProps) {
  const id = useId();
  const described = [
    hint !== null ? `${id}-hint` : null,
    error !== undefined ? `${id}-error` : null,
  ].filter((part) => part !== null);
  return (
    <div className="flex flex-col gap-2">
      <Label htmlFor={id} className={FIELD_LABEL_CLASS}>
        {label}
      </Label>
      <Textarea
        id={id}
        autoComplete="off"
        placeholder={placeholder}
        maxLength={NOTE_MAX_LENGTH}
        {...registration}
        disabled={disabled}
        aria-invalid={error !== undefined}
        aria-describedby={
          described.length > 0 ? described.join(" ") : undefined
        }
        className={TEXTAREA_CLASS}
      />
      {hint !== null && (
        <p id={`${id}-hint`} className={FIELD_HINT_CLASS}>
          {hint}
        </p>
      )}
      {error !== undefined && <FieldError id={`${id}-error`} message={error} />}
    </div>
  );
}
