"use client";

import { useId, useRef } from "react";

import { RupiahField } from "@/components/client/form/rupiah-field";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { formatRupiah } from "@/lib/format/rupiah";
import type { UnitTermRow, UnitTermRowView } from "./type";
import {
  changePriceLabel,
  nextPriceText,
  TERM_PRICE_HINT,
  termLabel,
  termPriceLabel,
} from "./utils";

type UnitTermChoiceProps = {
  row: UnitTermRow;
  view: UnitTermRowView;
  disabled: boolean;
  onCheckedChange: (checked: boolean) => void;
  onPriceTextChange: (text: string) => void;
  // "Ubah harga" on a term with a price in force.
  onChangePrice: () => void;
};

// One term in "Masa sewa dan harga": a checkbox, and for a checked term its price in force with
// "Ubah harga", or a field for a price.
export function UnitTermChoice({
  row,
  view,
  disabled,
  onCheckedChange,
  onPriceTextChange,
  onChangePrice,
}: UnitTermChoiceProps) {
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
        {termLabel(row.months)}
      </Label>

      {row.checked && view.kind === "priced" && (
        <div className="flex items-center justify-between gap-3 pl-[34px]">
          <div className="flex min-w-0 flex-col gap-0.5">
            <span className="text-base font-semibold md:text-[17px]">
              {formatRupiah(view.price)}
            </span>
            {view.nextPrice !== undefined && (
              <span className="text-sm text-muted-foreground md:text-base">
                {nextPriceText(view.nextPrice)}
              </span>
            )}
          </div>
          <Button
            type="button"
            variant="outline"
            aria-label={changePriceLabel(row.months)}
            onClick={onChangePrice}
            disabled={disabled}
            className="h-11 shrink-0 rounded-xl border-[1.5px] bg-card px-4 text-[15px] font-semibold md:text-base"
          >
            Ubah harga
          </Button>
        </div>
      )}

      {row.checked && view.kind === "input" && (
        <div className="flex flex-col gap-1.5 pl-[34px]">
          <RupiahField
            inputRef={priceRef}
            label={termPriceLabel(row.months)}
            name={`price-${row.months}`}
            placeholder="0"
            hint={TERM_PRICE_HINT}
            value={row.priceText}
            onChange={onPriceTextChange}
            error={undefined}
            disabled={disabled}
          />
          {view.nextPrice !== undefined && (
            <p className="text-sm text-muted-foreground md:text-base">
              {nextPriceText(view.nextPrice)}
            </p>
          )}
        </div>
      )}
    </div>
  );
}
