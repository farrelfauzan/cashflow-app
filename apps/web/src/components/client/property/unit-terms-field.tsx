"use client";

import { useId } from "react";
import { useController, useWatch, type UseFormReturn } from "react-hook-form";

import { FieldError } from "@/components/client/form/field-error";
import type { TermMonths, Unit, UnitInput } from "@/lib/api/generated/model";
import type {
  LastTermPrices,
  StandardTermMonths,
  UnitCustomTermRow,
  UnitFormValues,
} from "./type";
import {
  DROPPED_TERM_NOTE,
  dropsOfferedTerm,
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
  TERMS_HINT,
  TERMS_LEGEND,
  UNIT_TERM_CHOICES,
  customTermView,
  toggleCustomTerm,
  unitTermRowView,
} from "./utils";

import { UnitCustomTermFields } from "./unit-custom-term-fields";
import { UnitTermChoice } from "./unit-term-choice";

type UnitTermsFieldProps = {
  // The unit form: its "terms", "prices" and "custom" fields.
  form: UseFormReturn<UnitFormValues, unknown, UnitInput>;
  // The saved unit; none when adding.
  unit: Unit | undefined;
  // From the unit's price history: the last price of a term it no longer offers.
  lastPrices: LastTermPrices;
  disabled: boolean;
  onChangePrice: (months: TermMonths) => void;
};

// "Masa sewa dan harga": the four standard terms a unit may offer and its one custom term
// (decision 33), each with its price.
export function UnitTermsField({
  form,
  unit,
  lastPrices,
  disabled,
  onChangePrice,
}: UnitTermsFieldProps) {
  const id = useId();
  const { control, clearErrors, setValue } = form;
  // Registered in the order the fields appear on screen, after the label: a failed check
  // focuses the first field with an error in register order. The checkbox group ("terms") has
  // an error only when no field below it shows, so its place does not matter.
  const terms = useController({ control, name: "terms" });
  const price1 = useController({ control, name: "prices.1" });
  const price3 = useController({ control, name: "prices.3" });
  const price6 = useController({ control, name: "prices.6" });
  const price12 = useController({ control, name: "prices.12" });
  const customMonths = useController({ control, name: "custom.monthsText" });
  const customPrice = useController({ control, name: "custom.priceText" });
  const [customChecked, customShowsSaved] = useWatch({
    control,
    name: ["custom.checked", "custom.showsSaved"],
  });
  const prices = { 1: price1, 3: price3, 6: price6, 12: price12 } as const;
  const custom: UnitCustomTermRow = {
    checked: customChecked,
    showsSaved: customShowsSaved,
    monthsText: customMonths.field.value,
    priceText: customPrice.field.value,
  };
  const customView = customTermView(custom, unit, lastPrices);
  const error = terms.fieldState.error?.message;
  const describedBy = [error !== undefined ? `${id}-error` : null, `${id}-hint`]
    .filter((part) => part !== null)
    .join(" ");

  // Changing a standard term clears the group's error and its own price's.
  function changeChecked(months: StandardTermMonths, checked: boolean) {
    terms.field.onChange({ ...terms.field.value, [months]: checked });
    clearErrors(["terms", `prices.${months}`]);
  }

  function changePrice(months: StandardTermMonths, priceText: string) {
    prices[months].field.onChange(priceText);
    clearErrors(["terms", `prices.${months}`]);
  }

  // Changing the custom row clears the group's error and the row's own.
  function clearCustomErrors() {
    clearErrors(["terms", "custom.monthsText", "custom.priceText"]);
  }

  function changeCustomChecked(checked: boolean) {
    const next = toggleCustomTerm(custom, checked);
    setValue("custom.checked", next.checked);
    setValue("custom.showsSaved", next.showsSaved);
    clearCustomErrors();
  }

  function changeCustomMonths(monthsText: string) {
    customMonths.field.onChange(monthsText);
    clearCustomErrors();
  }

  function changeCustomPrice(priceText: string) {
    customPrice.field.onChange(priceText);
    clearCustomErrors();
  }

  return (
    <fieldset
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
        {UNIT_TERM_CHOICES.map((months) => (
          <UnitTermChoice
            key={months}
            row={{
              months,
              checked: terms.field.value[months],
              priceText: prices[months].field.value,
            }}
            view={unitTermRowView(unit, months, lastPrices)}
            // The first checkbox takes the focus for an error on the whole group.
            checkboxRef={
              months === UNIT_TERM_CHOICES[0] ? terms.field.ref : undefined
            }
            priceRef={prices[months].field.ref}
            priceError={prices[months].fieldState.error?.message}
            disabled={disabled}
            onCheckedChange={(checked) => changeChecked(months, checked)}
            onPriceTextChange={(priceText) => changePrice(months, priceText)}
            onChangePrice={() => onChangePrice(months)}
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
            priceRef={customPrice.field.ref}
            priceError={customPrice.fieldState.error?.message}
            disabled={disabled}
            onCheckedChange={changeCustomChecked}
            onPriceTextChange={changeCustomPrice}
            onChangePrice={() => onChangePrice(customView.months)}
          />
        ) : (
          <UnitCustomTermFields
            row={custom}
            view={customView}
            monthsRef={customMonths.field.ref}
            priceRef={customPrice.field.ref}
            monthsError={customMonths.fieldState.error?.message}
            priceError={customPrice.fieldState.error?.message}
            disabled={disabled}
            onCheckedChange={changeCustomChecked}
            onMonthsTextChange={changeCustomMonths}
            onPriceTextChange={changeCustomPrice}
          />
        )}
      </div>
      {error !== undefined && <FieldError id={`${id}-error`} message={error} />}
      {dropsOfferedTerm(unit, terms.field.value, custom) && (
        <p className="text-sm leading-[1.45] font-semibold text-foreground/80 md:text-base">
          {DROPPED_TERM_NOTE}
        </p>
      )}
    </fieldset>
  );
}
