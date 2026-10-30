"use client";

import { useId, useRef } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { RupiahField } from "@/components/client/form/rupiah-field";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import type { UnitCustomTermRow, UnitCustomTermView } from "./type";
import {
  CUSTOM_MONTHS_LABEL,
  CUSTOM_TERM_LABEL,
  customTermPriceLabel,
  FIELD_LABEL_CLASS,
  INPUT_CLASS,
  monthsDigits,
  termPriceHint,
} from "./utils";

type UnitCustomTermFieldsProps = {
  row: UnitCustomTermRow;
  view: Extract<UnitCustomTermView, { kind: "lainnya" }>;
  // Under "Jumlah bulan" and under the price field.
  monthsError: string | undefined;
  priceError: string | undefined;
  disabled: boolean;
  onCheckedChange: (checked: boolean) => void;
  onMonthsTextChange: (text: string) => void;
  onPriceTextChange: (text: string) => void;
};

// "Lainnya", the last row of "Masa sewa dan harga": a checkbox, and once ticked the month count of
// the unit's custom term (decision 33) and its price, indented like a standard term's price.
export function UnitCustomTermFields({
  row,
  view,
  monthsError,
  priceError,
  disabled,
  onCheckedChange,
  onMonthsTextChange,
  onPriceTextChange,
}: UnitCustomTermFieldsProps) {
  const id = useId();
  const priceRef = useRef<HTMLInputElement>(null);

  return (
    <div className="flex flex-col gap-2.5 border-t py-2.5 first:border-t-0">
      <Label
        htmlFor={`${id}-checked`}
        className="min-h-11 gap-3 text-base leading-normal font-semibold md:text-[17px]"
      >
        <Checkbox
          id={`${id}-checked`}
          checked={row.checked}
          onCheckedChange={(checked) => onCheckedChange(checked === true)}
          disabled={disabled}
          className="size-[22px]"
        />
        {CUSTOM_TERM_LABEL}
      </Label>

      {row.checked && (
        <div className="flex flex-col gap-[18px] pl-[34px]">
          <div className="flex flex-col gap-2">
            <Label htmlFor={`${id}-months`} className={FIELD_LABEL_CLASS}>
              {CUSTOM_MONTHS_LABEL}
            </Label>
            <Input
              id={`${id}-months`}
              name="custom-months"
              type="text"
              inputMode="numeric"
              autoComplete="off"
              maxLength={3}
              placeholder="0"
              value={row.monthsText}
              onChange={(event) =>
                onMonthsTextChange(monthsDigits(event.target.value))
              }
              disabled={disabled}
              aria-invalid={monthsError !== undefined}
              aria-describedby={
                monthsError !== undefined ? `${id}-months-error` : undefined
              }
              className={cn(INPUT_CLASS, "w-28")}
            />
            {monthsError !== undefined && (
              <FieldError id={`${id}-months-error`} message={monthsError} />
            )}
          </div>
          <RupiahField
            inputRef={priceRef}
            label={customTermPriceLabel(row)}
            name="price-custom"
            placeholder="0"
            hint={termPriceHint(view.price)}
            value={row.priceText}
            onChange={onPriceTextChange}
            error={priceError}
            disabled={disabled}
          />
        </div>
      )}
    </div>
  );
}
