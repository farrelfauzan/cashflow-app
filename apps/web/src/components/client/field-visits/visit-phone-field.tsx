"use client";

import { useId } from "react";
import type { UseFormRegisterReturn } from "react-hook-form";

import { FieldError } from "@/components/client/form/field-error";
import {
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
  INPUT_CLASS,
} from "@/components/client/form/utils";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

import { PHONE_HINT, PHONE_LABEL, PHONE_PLACEHOLDER } from "./utils";

type VisitPhoneFieldProps = {
  // react-hook-form's registration of newPhone, clearing its error when edited.
  registration: UseFormRegisterReturn<"newPhone">;
  error: string | undefined;
  disabled: boolean;
};

// Under "Nomor WA berubah" (board HP-Kunjungan, column 5): the tenant's new number, typed as they
// say it. The API reads it (invalid_phone comes back under it); the admin applies it from Antrean
// cek.
export function VisitPhoneField({
  registration,
  error,
  disabled,
}: VisitPhoneFieldProps) {
  const id = useId();
  const errorId = `${id}-error`;
  const hintId = `${id}-hint`;
  return (
    <div className="flex flex-col gap-2">
      <Label htmlFor={id} className={FIELD_LABEL_CLASS}>
        {PHONE_LABEL}
      </Label>
      <Input
        id={id}
        type="tel"
        inputMode="tel"
        autoComplete="off"
        placeholder={PHONE_PLACEHOLDER}
        {...registration}
        disabled={disabled}
        aria-invalid={error !== undefined}
        aria-describedby={error === undefined ? hintId : `${errorId} ${hintId}`}
        className={INPUT_CLASS}
      />
      {error !== undefined && <FieldError id={errorId} message={error} />}
      <p id={hintId} className={FIELD_HINT_CLASS}>
        {PHONE_HINT}
      </p>
    </div>
  );
}
