"use client";

import { useId, type Ref } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { RupiahInput } from "@/components/client/form/rupiah-input";
import {
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
} from "@/components/client/form/utils";

import { PROMISE_AMOUNT_LABEL } from "./utils";

type PromiseAmountFieldProps = {
  inputRef: Ref<HTMLInputElement>;
  // Digits grouped with dots; empty promises everything left.
  text: string;
  // Under the field: "Kosongkan untuk semua sisa, Rp1.250.000."
  hint: string;
  onTextChange: (text: string) => void;
  error: string | null;
};

// "Nominal janji (boleh dikosongkan)": digits only, sent as integer rupiah.
export function PromiseAmountField({
  inputRef,
  text,
  hint,
  onTextChange,
  error,
}: PromiseAmountFieldProps) {
  const id = useId();
  const errorId = `${id}-error`;
  const hintId = `${id}-hint`;

  return (
    <div className="flex min-w-0 flex-col gap-2">
      <label htmlFor={`${id}-amount`} className={FIELD_LABEL_CLASS}>
        {PROMISE_AMOUNT_LABEL}
      </label>
      <RupiahInput
        inputRef={inputRef}
        id={`${id}-amount`}
        name="promisedAmount"
        placeholder=""
        value={text}
        onChange={onTextChange}
        invalid={error !== null}
        describedBy={error === null ? hintId : `${errorId} ${hintId}`}
        disabled={false}
      />
      {error !== null && <FieldError id={errorId} message={error} />}
      <p id={hintId} className={FIELD_HINT_CLASS}>
        {hint}
      </p>
    </div>
  );
}
