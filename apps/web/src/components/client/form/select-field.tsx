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
  // A disabled choice shows but cannot be picked.
  options: readonly { value: string; label: string; disabled?: boolean }[];
  onValueChange: (value: string) => void;
  // Under the field; null for none.
  hint: string | null;
  error: string | undefined;
  disabled: boolean;
  // "warning" gives the select a warning border and the hint the warning text: a choice that
  // still works but needs the member's attention (a collector who was disabled).
  hintTone?: "muted" | "warning";
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
  hintTone = "muted",
  className,
}: SelectFieldProps) {
  const warning = hintTone === "warning";
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
          className={cn(SELECT_TRIGGER_CLASS, warning && "border-warning")}
        >
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent position="popper">
          {options.map((option) => (
            <SelectItem
              key={option.value}
              value={option.value}
              disabled={option.disabled}
              className={SELECT_ITEM_CLASS}
            >
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {error !== undefined && <FieldError id={errorId} message={error} />}
      {hint !== null && (
        <p
          id={hintId}
          className={cn(
            FIELD_HINT_CLASS,
            warning && "font-semibold text-warning",
          )}
        >
          {hint}
        </p>
      )}
    </div>
  );
}
