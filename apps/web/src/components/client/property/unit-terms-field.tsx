"use client";

import { useId, type RefObject } from "react";

import { FieldError } from "@/components/client/form/field-error";
import type { TermMonths, Unit } from "@/lib/api/generated/model";
import type {
  LastTermPrices,
  UnitCustomTermRow,
  UnitFormErrors,
  UnitTermRow,
} from "./type";
import {
  DROPPED_TERM_NOTE,
  dropsOfferedTerm,
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
  TERMS_HINT,
  TERMS_LEGEND,
  customTermView,
  toggleCustomTerm,
  unitTermRowView,
} from "./utils";

import { UnitCustomTermFields } from "./unit-custom-term-fields";
import { UnitTermChoice } from "./unit-term-choice";

type UnitTermsFieldProps = {
  fieldsetRef: RefObject<HTMLFieldSetElement | null>;
  // The saved unit; none when adding.
  unit: Unit | undefined;
  rows: readonly UnitTermRow[];
  // The fifth row: the unit's custom term, or "Lainnya".
  custom: UnitCustomTermRow;
  error: string | undefined;
  // Under each standard term's price field.
  priceErrors: UnitFormErrors["prices"];
  // Under the custom row's "Jumlah bulan" and price field.
  customMonthsError: string | undefined;
  customPriceError: string | undefined;
  // From the unit's price history: the last price of a term it no longer offers.
  lastPrices: LastTermPrices;
  disabled: boolean;
  onRowChange: (row: UnitTermRow) => void;
  onCustomChange: (custom: UnitCustomTermRow) => void;
  onChangePrice: (months: TermMonths) => void;
};

// "Masa sewa dan harga": the four standard terms a unit may offer and its one custom term
// (decision 33), each with its price.
export function UnitTermsField({
  fieldsetRef,
  unit,
  rows,
  custom,
  error,
  priceErrors,
  customMonthsError,
  customPriceError,
  lastPrices,
  disabled,
  onRowChange,
  onCustomChange,
  onChangePrice,
}: UnitTermsFieldProps) {
  const id = useId();
  const customView = customTermView(custom, unit, lastPrices);
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
        {customView.kind === "saved" ? (
          <UnitTermChoice
            row={{
              months: customView.months,
              checked: custom.checked,
              priceText: custom.priceText,
            }}
            view={customView.term}
            priceError={customPriceError}
            disabled={disabled}
            onCheckedChange={(checked) =>
              onCustomChange(toggleCustomTerm(custom, checked))
            }
            onPriceTextChange={(priceText) =>
              onCustomChange({ ...custom, priceText })
            }
            onChangePrice={() => onChangePrice(customView.months)}
          />
        ) : (
          <UnitCustomTermFields
            row={custom}
            view={customView}
            monthsError={customMonthsError}
            priceError={customPriceError}
            disabled={disabled}
            onCheckedChange={(checked) =>
              onCustomChange(toggleCustomTerm(custom, checked))
            }
            onMonthsTextChange={(monthsText) =>
              onCustomChange({ ...custom, monthsText })
            }
            onPriceTextChange={(priceText) =>
              onCustomChange({ ...custom, priceText })
            }
          />
        )}
      </div>
      {error !== undefined && <FieldError id={`${id}-error`} message={error} />}
      {dropsOfferedTerm(unit, rows, custom) && (
        <p className="text-sm leading-[1.45] font-semibold text-foreground/80 md:text-base">
          {DROPPED_TERM_NOTE}
        </p>
      )}
    </fieldset>
  );
}
