"use client";

import { useId, type RefObject } from "react";

import { FieldError } from "@/components/client/form/field-error";
import {
  FIELD_HINT_CLASS,
  SELECT_ITEM_CLASS,
  SELECT_TRIGGER_CLASS,
} from "@/components/client/form/utils";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { TermMonths } from "@/lib/api/generated/model";
import type { BillingChoice, DateOption } from "./type";
import {
  billAllHint,
  billAllLabel,
  billedFromSelectLabel,
  billingExplanation,
  billPickHint,
  billPickLabel,
  billPickSelectHint,
} from "./utils";

import { LeaseInfoNote } from "./lease-info-note";

type LeaseBillingFieldProps = {
  selectRef: RefObject<HTMLButtonElement | null>;
  // A start date before the day the lease is entered.
  startDate: string;
  // The day the lease is entered: today for a new lease, its enteredOn for one being changed.
  enteredOn: string;
  today: string;
  // The lease's term: the periods the choices and the note name.
  termMonths: TermMonths;
  billing: BillingChoice;
  onBillingChange: (billing: BillingChoice) => void;
  options: readonly DateOption[];
  // The period start the select shows.
  billedFrom: string;
  onBilledFromChange: (date: string) => void;
  error: string | undefined;
  disabled: boolean;
};

// "Mulai tagih dari" for a lease entered late (boards Sewa-Baru.dc.html and Sewa-Ubah.dc.html):
// bill every period since the start date, or start from a later period the owner did not
// collect by hand. The note under it says, with the real dates, which periods are invoiced and
// when they are due.
export function LeaseBillingField({
  selectRef,
  startDate,
  enteredOn,
  today,
  termMonths,
  billing,
  onBillingChange,
  options,
  billedFrom,
  onBilledFromChange,
  error,
  disabled,
}: LeaseBillingFieldProps) {
  const id = useId();
  const errorId = `${id}-error`;
  const hintId = `${id}-hint`;
  const picking = billing === "pick";
  const billingStart = picking ? billedFrom : startDate;

  return (
    <div className="flex flex-col gap-3">
      <fieldset className="flex min-w-0 flex-col gap-1.5 rounded-[14px] border-[1.5px] border-input bg-card p-4">
        <legend className="px-1.5 text-base font-bold md:text-[17px]">
          Mulai tagih dari
        </legend>
        <RadioGroup
          value={billing}
          onValueChange={(next) => {
            if (next === "start" || next === "pick") onBillingChange(next);
          }}
          disabled={disabled}
          className="gap-1.5"
        >
          <Label
            htmlFor={`${id}-start`}
            className="min-h-11 items-start gap-3 py-2 font-normal"
          >
            <RadioGroupItem
              id={`${id}-start`}
              value="start"
              className="mt-0.5 size-[22px]"
            />
            <span className="flex flex-col gap-0.5">
              <span className="text-base font-semibold">
                {billAllLabel(termMonths)}
              </span>
              <span className="text-sm leading-[1.45] text-muted-foreground md:text-[15px]">
                {billAllHint(startDate, termMonths)}
              </span>
            </span>
          </Label>
          <Label
            htmlFor={`${id}-pick`}
            className="min-h-11 items-start gap-3 py-2 font-normal"
          >
            <RadioGroupItem
              id={`${id}-pick`}
              value="pick"
              className="mt-0.5 size-[22px]"
            />
            <span className="flex flex-col gap-0.5">
              <span className="text-base font-semibold">
                {billPickLabel(termMonths)}
              </span>
              <span className="text-sm leading-[1.45] text-muted-foreground md:text-[15px]">
                {billPickHint(termMonths)}
              </span>
            </span>
          </Label>
        </RadioGroup>
        <div className="flex flex-col gap-1.5 pl-[34px]">
          <Select
            value={billedFrom}
            onValueChange={onBilledFromChange}
            disabled={disabled || !picking}
          >
            <SelectTrigger
              ref={selectRef}
              aria-label={billedFromSelectLabel(termMonths)}
              aria-invalid={error !== undefined}
              aria-describedby={
                error !== undefined ? `${errorId} ${hintId}` : hintId
              }
              className={SELECT_TRIGGER_CLASS}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent position="popper">
              {options.map((option) => (
                <SelectItem
                  key={option.value}
                  value={option.value}
                  className={SELECT_ITEM_CLASS}
                >
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {error !== undefined && <FieldError id={errorId} message={error} />}
          <p id={hintId} className={FIELD_HINT_CLASS}>
            {billPickSelectHint(termMonths)}
          </p>
        </div>
      </fieldset>
      {billingStart !== "" && (
        <LeaseInfoNote
          text={billingExplanation(
            startDate,
            billingStart,
            enteredOn,
            today,
            termMonths,
          )}
        />
      )}
    </div>
  );
}
