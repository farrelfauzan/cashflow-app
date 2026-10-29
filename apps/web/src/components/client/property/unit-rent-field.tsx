"use client";

import type { RefObject } from "react";

import { RupiahField } from "@/components/client/form/rupiah-field";

type UnitRentFieldProps = {
  inputRef: RefObject<HTMLInputElement | null>;
  // The field's text: digits grouped with dots, empty for none.
  value: string;
  onChange: (text: string) => void;
  error: string | undefined;
  disabled: boolean;
};

// "Sewa biasa per bulan": the unit's usual rent, digits only, sent as an integer.
export function UnitRentField({
  inputRef,
  value,
  onChange,
  error,
  disabled,
}: UnitRentFieldProps) {
  return (
    <RupiahField
      inputRef={inputRef}
      label="Sewa biasa per bulan (boleh dikosongkan)"
      name="defaultRent"
      placeholder="1.250.000"
      hint="Hanya angka. Dipakai sebagai usulan saat membuat sewa baru, dan bisa diubah per penyewa."
      value={value}
      onChange={onChange}
      error={error}
      disabled={disabled}
    />
  );
}
