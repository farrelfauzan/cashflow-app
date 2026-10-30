"use client";

import {
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type Ref,
} from "react";

import { cn } from "@/lib/utils";
import {
  caretAfterDigits,
  rupiahDigitsBefore,
  rupiahFieldText,
} from "@/lib/format/rupiah-input";

type RupiahInputProps = {
  // For moving focus from outside: a ref object, or a callback such as react-hook-form's.
  inputRef?: Ref<HTMLInputElement>;
  id: string;
  name: string;
  placeholder: string;
  // Its accessible name when no <label> points at it.
  ariaLabel?: string;
  // The field's text: digits grouped with dots, empty for none.
  value: string;
  onChange: (text: string) => void;
  invalid: boolean;
  describedBy: string | undefined;
  disabled: boolean;
  // The box's size: its height and width.
  className?: string;
};

// An amount in rupiah behind a fixed "Rp". It takes digits only (anything else typed or pasted is
// dropped) and shows them grouped like 1.250.000; the form sends the integer. The caret stays
// after the same digits when the text is grouped again, and Backspace or Delete next to a dot
// removes the digit beyond it.
export function RupiahInput({
  inputRef,
  id,
  name,
  placeholder,
  ariaLabel,
  value,
  onChange,
  invalid,
  describedBy,
  disabled,
  className,
}: RupiahInputProps) {
  // The input's own ref places the caret; the caller's gets the same input.
  const ref = useRef<HTMLInputElement>(null);
  useImperativeHandle(inputRef, () => ref.current as HTMLInputElement, []);

  // Where the caret goes once the grouped text is in the field. A new object on every change, so
  // the field renders and the effect runs even when the text stays the same (a letter typed).
  const [caret, setCaret] = useState<{ at: number } | null>(null);
  useLayoutEffect(() => {
    if (caret !== null) ref.current?.setSelectionRange(caret.at, caret.at);
  }, [caret, ref]);

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
    <div
      className={cn(
        "flex h-13 items-center rounded-xl border-[1.5px] border-input bg-card transition-[color,box-shadow] focus-within:border-ring focus-within:ring-[3px] focus-within:ring-ring/50 has-[input[aria-invalid=true]]:border-2 has-[input[aria-invalid=true]]:border-destructive",
        className,
      )}
    >
      <span
        aria-hidden="true"
        className="pl-3.5 text-base text-muted-foreground md:text-[17px]"
      >
        Rp
      </span>
      <input
        ref={ref}
        id={id}
        name={name}
        type="text"
        inputMode="numeric"
        autoComplete="off"
        placeholder={placeholder}
        aria-label={ariaLabel}
        value={value}
        onChange={(event) => handleChange(event.target)}
        onKeyDown={handleKeyDown}
        disabled={disabled}
        aria-invalid={invalid}
        aria-describedby={describedBy}
        className="h-full min-w-0 flex-1 bg-transparent px-2.5 text-base outline-none placeholder:text-muted-foreground disabled:opacity-50 md:text-[17px]"
      />
    </div>
  );
}
