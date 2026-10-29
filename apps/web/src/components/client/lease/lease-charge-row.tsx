"use client";

import { FieldError } from "@/components/client/form/field-error";
import { RupiahInput } from "@/components/client/form/rupiah-input";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import type { ChargeRow } from "./type";
import { chargeAmountLabel } from "./utils";

type LeaseChargeRowProps = {
  // The amount input's id; the form focuses it by this id.
  amountId: string;
  row: ChargeRow;
  error: string | undefined;
  disabled: boolean;
  onCheckedChange: (checked: boolean) => void;
  onAmountChange: (text: string) => void;
};

// One charge type in "Biaya tambahan": a checkbox with its name, and its monthly amount in
// rupiah. The error under it is the row's own check or the API's on that charge.
export function LeaseChargeRow({
  amountId,
  row,
  error,
  disabled,
  onCheckedChange,
  onAmountChange,
}: LeaseChargeRowProps) {
  const checkboxId = `${amountId}-checked`;
  const errorId = `${amountId}-error`;

  return (
    <div className="flex flex-col gap-2 border-t py-2.5">
      <div className="flex items-center gap-3">
        <Label
          htmlFor={checkboxId}
          className="min-h-11 min-w-0 flex-1 gap-3 text-base leading-normal font-semibold md:text-[17px]"
        >
          <Checkbox
            id={checkboxId}
            checked={row.checked}
            onCheckedChange={(checked) => onCheckedChange(checked === true)}
            disabled={disabled}
            className="size-[22px]"
          />
          <span className="min-w-0 break-words">{row.name}</span>
        </Label>
        <RupiahInput
          id={amountId}
          name={`charge-${row.chargeTypeId}`}
          placeholder="0"
          ariaLabel={chargeAmountLabel(row.name)}
          value={row.amount}
          onChange={onAmountChange}
          invalid={error !== undefined}
          describedBy={error !== undefined ? errorId : undefined}
          disabled={disabled}
          className="h-12 w-[150px] shrink-0 md:w-[190px]"
        />
      </div>
      {error !== undefined && <FieldError id={errorId} message={error} />}
    </div>
  );
}
