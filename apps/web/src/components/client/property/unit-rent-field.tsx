"use client";

import { useId, type Ref } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { Label } from "@/components/ui/label";
import { rupiahFieldText } from "@/lib/format/rupiah-input";

import { FIELD_HINT_CLASS, FIELD_LABEL_CLASS } from "./utils";

type UnitRentFieldProps = {
  inputRef: Ref<HTMLInputElement>;
  // The field's text: digits grouped with dots, empty for none.
  value: string;
  onChange: (text: string) => void;
  error: string | undefined;
  disabled: boolean;
};

// "Sewa biasa per bulan": an amount in rupiah behind a fixed "Rp". It takes digits only (anything
// else typed or pasted is dropped) and shows them grouped like 1.250.000; the form sends the
// integer.
export function UnitRentField({
  inputRef,
  value,
  onChange,
  error,
  disabled,
}: UnitRentFieldProps) {
  const id = useId();
  const errorId = `${id}-error`;
  const hintId = `${id}-hint`;

  return (
    <div className="flex flex-col gap-2">
      <Label htmlFor={`${id}-rent`} className={FIELD_LABEL_CLASS}>
        Sewa biasa per bulan (boleh dikosongkan)
      </Label>
      <div className="flex h-13 items-center rounded-xl border-[1.5px] border-input bg-card transition-[color,box-shadow] focus-within:border-ring focus-within:ring-[3px] focus-within:ring-ring/50 has-[input[aria-invalid=true]]:border-2 has-[input[aria-invalid=true]]:border-destructive">
        <span
          aria-hidden="true"
          className="pl-3.5 text-base text-muted-foreground md:text-[17px]"
        >
          Rp
        </span>
        <input
          ref={inputRef}
          id={`${id}-rent`}
          name="defaultRent"
          type="text"
          inputMode="numeric"
          autoComplete="off"
          placeholder="1.250.000"
          value={value}
          onChange={(event) => onChange(rupiahFieldText(event.target.value))}
          disabled={disabled}
          aria-invalid={error !== undefined}
          aria-describedby={
            error !== undefined ? `${errorId} ${hintId}` : hintId
          }
          className="h-full min-w-0 flex-1 bg-transparent px-2.5 text-base outline-none placeholder:text-muted-foreground disabled:opacity-50 md:text-[17px]"
        />
      </div>
      {error !== undefined && <FieldError id={errorId} message={error} />}
      <p id={hintId} className={FIELD_HINT_CLASS}>
        Hanya angka. Dipakai sebagai usulan saat membuat sewa baru, dan bisa
        diubah per penyewa.
      </p>
    </div>
  );
}
