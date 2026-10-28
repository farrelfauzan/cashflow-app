"use client";

import {
  useId,
  useLayoutEffect,
  useState,
  type KeyboardEvent,
  type RefObject,
} from "react";

import { FieldError } from "@/components/client/form/field-error";
import { Label } from "@/components/ui/label";
import {
  caretAfterDigits,
  rupiahDigitsBefore,
  rupiahFieldText,
} from "@/lib/format/rupiah-input";

import { FIELD_HINT_CLASS, FIELD_LABEL_CLASS } from "./utils";

type UnitRentFieldProps = {
  inputRef: RefObject<HTMLInputElement | null>;
  // The field's text: digits grouped with dots, empty for none.
  value: string;
  onChange: (text: string) => void;
  error: string | undefined;
  disabled: boolean;
};

// "Sewa biasa per bulan": an amount in rupiah behind a fixed "Rp". It takes digits only (anything
// else typed or pasted is dropped) and shows them grouped like 1.250.000; the form sends the
// integer. The caret stays after the same digits when the text is grouped again, and Backspace
// or Delete next to a dot removes the digit beyond it.
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

  // Where the caret goes once the grouped text is in the field. A new object on every change, so
  // the field renders and the effect runs even when the text stays the same (a letter typed).
  const [caret, setCaret] = useState<{ at: number } | null>(null);
  useLayoutEffect(() => {
    if (caret !== null) inputRef.current?.setSelectionRange(caret.at, caret.at);
  }, [caret, inputRef]);

  function handleChange(input: HTMLInputElement) {
    const typed = input.value;
    const text = rupiahFieldText(typed);
    setCaret({
      at: caretAfterDigits(
        text,
        rupiahDigitsBefore(typed, input.selectionStart ?? typed.length),
      ),
    });
    onChange(text);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    const input = event.currentTarget;
    const at = input.selectionStart;
    if (at === null || at !== input.selectionEnd) return;
    if (event.key === "Backspace" && input.value[at - 1] === ".") {
      input.setSelectionRange(at - 1, at - 1);
    } else if (event.key === "Delete" && input.value[at] === ".") {
      input.setSelectionRange(at + 1, at + 1);
    }
  }

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
          onChange={(event) => handleChange(event.target)}
          onKeyDown={handleKeyDown}
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
