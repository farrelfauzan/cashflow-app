"use client";

import { useId, type ReactNode, type Ref } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { Label } from "@/components/ui/label";

import { FIELD_HINT_CLASS, FIELD_LABEL_CLASS } from "./utils";

import { RupiahInput } from "./rupiah-input";

type RupiahFieldProps = {
  // For moving focus from outside: a ref object, or a callback such as react-hook-form's.
  inputRef: Ref<HTMLInputElement>;
  label: string;
  // The input's name, the API field it fills ("depositAmount").
  name: string;
  placeholder: string;
  // Under the field; none for no hint.
  hint: ReactNode;
  // The field's text: digits grouped with dots, empty for none.
  value: string;
  onChange: (text: string) => void;
  error: string | undefined;
  disabled: boolean;
};

// A labelled amount field in rupiah: RupiahInput with its label, error and hint.
export function RupiahField({
  inputRef,
  label,
  name,
  placeholder,
  hint,
  value,
  onChange,
  error,
  disabled,
}: RupiahFieldProps) {
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
    <div className="flex flex-col gap-2">
      <Label htmlFor={`${id}-amount`} className={FIELD_LABEL_CLASS}>
        {label}
      </Label>
      <RupiahInput
        inputRef={inputRef}
        id={`${id}-amount`}
        name={name}
        placeholder={placeholder}
        value={value}
        onChange={onChange}
        invalid={error !== undefined}
        describedBy={describedBy === "" ? undefined : describedBy}
        disabled={disabled}
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
