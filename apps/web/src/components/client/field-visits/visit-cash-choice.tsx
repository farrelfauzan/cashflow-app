"use client";

import { useId } from "react";

import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";

import type { VisitCashChoiceView } from "./type";
import { CASH_LIST_LABEL, NO_CASH_TEXT, RECEIVE_CASH_TEXT } from "./utils";

type VisitCashChoiceProps = {
  choices: readonly VisitCashChoiceView[];
  // The one "Sudah bayar hari ini" names.
  paymentId: string | null;
  onChoose: (paymentId: string) => void;
  disabled: boolean;
  // The member may record cash (cash:record): without cash yet, "Terima tunai" is offered.
  canReceive: boolean;
  onReceiveCash: () => void;
};

// Under "Sudah bayar hari ini" (board HP-Kunjungan, columns 2 and 3): the cash taken from the
// tenant today that no outcome names yet, newest first and chosen; without any, why, and "Terima
// tunai", which opens that sheet over this one. No amount is typed here: cash always goes through
// Terima tunai, with its receipt photo.
export function VisitCashChoice({
  choices,
  paymentId,
  onChoose,
  disabled,
  canReceive,
  onReceiveCash,
}: VisitCashChoiceProps) {
  const id = useId();
  if (choices.length === 0) {
    return (
      <div className="flex flex-col gap-2.5 rounded-xl bg-muted px-3.5 py-3">
        <p className="text-sm leading-normal md:text-base">{NO_CASH_TEXT}</p>
        {canReceive && (
          <Button
            type="button"
            variant="outline"
            disabled={disabled}
            onClick={onReceiveCash}
            className="h-12 rounded-xl border-[1.5px] bg-card text-base font-semibold"
          >
            {RECEIVE_CASH_TEXT}
          </Button>
        )}
      </div>
    );
  }
  return (
    <div className="flex flex-col gap-1.5">
      <span id={`${id}-label`} className="text-sm font-semibold md:text-base">
        {CASH_LIST_LABEL}
      </span>
      <RadioGroup
        aria-labelledby={`${id}-label`}
        value={paymentId ?? ""}
        onValueChange={onChoose}
        disabled={disabled}
        className="gap-2"
      >
        {choices.map((choice) => (
          <Label
            key={choice.paymentId}
            htmlFor={`${id}-${choice.paymentId}`}
            className="min-h-12 gap-3 rounded-lg border-[1.5px] bg-card px-3.5 py-2.5 text-sm leading-normal font-normal tabular-nums md:text-base"
          >
            <RadioGroupItem
              id={`${id}-${choice.paymentId}`}
              value={choice.paymentId}
              className="size-5"
            />
            {choice.label}
          </Label>
        ))}
      </RadioGroup>
    </div>
  );
}
