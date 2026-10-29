"use client";

import { useId, type RefObject } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";

import {
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
  SELECT_ITEM_CLASS,
  SELECT_TRIGGER_CLASS,
} from "./utils";

type SelectFieldProps = {
  triggerRef: RefObject<HTMLButtonElement | null>;
  label: string;
  // "" shows the placeholder.
  value: string;
  placeholder: string;
  options: readonly { value: string; label: string }[];
  onValueChange: (value: string) => void;
  // Under the field; null for none.
  hint: string | null;
  error: string | undefined;
  disabled: boolean;
  className?: string;
};

// A labelled select of a panel form, as tall as a text field, with its error and hint.
export function SelectField({
  triggerRef,
  label,
  value,
  placeholder,
  options,
  onValueChange,
  hint,
  error,
  disabled,
  className,
}: SelectFieldProps) {
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
      <Label htmlFor={`${id}-select`} className={FIELD_LABEL_CLASS}>
        {label}
      </Label>
      <Select value={value} onValueChange={onValueChange} disabled={disabled}>
        <SelectTrigger
          ref={triggerRef}
          id={`${id}-select`}
          aria-invalid={error !== undefined}
          aria-describedby={describedBy === "" ? undefined : describedBy}
          className={SELECT_TRIGGER_CLASS}
        >
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent position="popper">
          {options.map((option) => (
            <SelectItem
              key={option.value}
              value={option.value}
              className={SELECT_ITEM_CLASS}
            >
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {error !== undefined && <FieldError id={errorId} message={error} />}
      {hint !== null && (
        <p id={hintId} className={FIELD_HINT_CLASS}>
          {hint}
        </p>
      )}
    </div>
  );
}
