"use client";

import { FieldError } from "@/components/client/form/field-error";
import { RupiahInput } from "@/components/client/form/rupiah-input";

import { PARTIAL_AMOUNT_HINT, PARTIAL_AMOUNT_LABEL } from "./utils";

const FIELD_ID = "partial-amount";
const HINT_ID = "partial-amount-hint";
const ERROR_ID = "partial-amount-error";

type PartialAmountFieldProps = {
  // The field's text: digits grouped with dots, empty for none.
  text: string;
  onTextChange: (text: string) => void;
  // The message under the field (above one QR's most, or the API's refusal); null for the hint.
  error: string | null;
};

// "Nominal yang dibayar sekarang": a large amount field, with "Bebas berapa pun, tanpa minimum."
// under it, or what is wrong with the amount.
export function PartialAmountField({
  text,
  onTextChange,
  error,
}: PartialAmountFieldProps) {
  return (
    <div className="flex flex-col gap-2.5 rounded-xl border bg-card px-4 py-4 shadow-xs md:px-5">
      <label htmlFor={FIELD_ID} className="text-sm font-semibold md:text-base">
        {PARTIAL_AMOUNT_LABEL}
      </label>
      <RupiahInput
        id={FIELD_ID}
        name="amount"
        placeholder="0"
        value={text}
        onChange={onTextChange}
        invalid={error !== null}
        describedBy={error === null ? HINT_ID : ERROR_ID}
        disabled={false}
        className="h-16"
        textClassName="text-2xl font-bold md:text-[28px]"
      />
      {error === null ? (
        <p id={HINT_ID} className="text-sm text-muted-foreground md:text-base">
          {PARTIAL_AMOUNT_HINT}
        </p>
      ) : (
        <FieldError id={ERROR_ID} message={error} />
      )}
    </div>
  );
}
