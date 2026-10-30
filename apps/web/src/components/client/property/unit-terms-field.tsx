"use client";

import { useId, type RefObject } from "react";

import { FieldError } from "@/components/client/form/field-error";
import type { TermMonths, Unit } from "@/lib/api/generated/model";
import type { LastTermPrices, UnitFormErrors, UnitTermRow } from "./type";
import {
  DROPPED_TERM_NOTE,
  dropsOfferedTerm,
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
  TERMS_HINT,
  TERMS_LEGEND,
  unitTermRowView,
} from "./utils";

import { UnitTermChoice } from "./unit-term-choice";

type UnitTermsFieldProps = {
  fieldsetRef: RefObject<HTMLFieldSetElement | null>;
  // The saved unit; none when adding.
  unit: Unit | undefined;
  rows: readonly UnitTermRow[];
  error: string | undefined;
  // Under each term's price field.
  priceErrors: UnitFormErrors["prices"];
  // From the unit's price history: the last price of a term it no longer offers.
  lastPrices: LastTermPrices;
  disabled: boolean;
  onRowChange: (row: UnitTermRow) => void;
  onChangePrice: (months: TermMonths) => void;
};

// "Masa sewa dan harga": the four terms a unit may offer, each with its price.
export function UnitTermsField({
  fieldsetRef,
  unit,
  rows,
  error,
  priceErrors,
  lastPrices,
  disabled,
  onRowChange,
  onChangePrice,
}: UnitTermsFieldProps) {
  const id = useId();
  const describedBy = [error !== undefined ? `${id}-error` : null, `${id}-hint`]
    .filter((part) => part !== null)
    .join(" ");

  return (
    <fieldset
      ref={fieldsetRef}
      aria-describedby={describedBy}
      className="flex min-w-0 flex-col gap-2"
    >
      <legend className={FIELD_LABEL_CLASS}>{TERMS_LEGEND}</legend>
      <p id={`${id}-hint`} className={FIELD_HINT_CLASS}>
        {TERMS_HINT}
      </p>
      <div
        data-invalid={error !== undefined}
        className="flex flex-col rounded-xl border-[1.5px] bg-card px-3.5 data-[invalid=true]:border-2 data-[invalid=true]:border-destructive"
      >
        {rows.map((row) => (
          <UnitTermChoice
            key={row.months}
            row={row}
            view={unitTermRowView(unit, row.months, lastPrices)}
            priceError={priceErrors?.[row.months]}
            disabled={disabled}
            onCheckedChange={(checked) => onRowChange({ ...row, checked })}
            onPriceTextChange={(priceText) =>
              onRowChange({ ...row, priceText })
            }
            onChangePrice={() => onChangePrice(row.months)}
          />
        ))}
      </div>
      {error !== undefined && <FieldError id={`${id}-error`} message={error} />}
      {dropsOfferedTerm(unit, rows) && (
        <p className="text-sm leading-[1.45] font-semibold text-foreground/80 md:text-base">
          {DROPPED_TERM_NOTE}
        </p>
      )}
    </fieldset>
  );
}
