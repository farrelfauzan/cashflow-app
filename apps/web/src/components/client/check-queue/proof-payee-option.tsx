"use client";

import { Label } from "@/components/ui/label";
import { RadioGroupItem } from "@/components/ui/radio-group";

import type { ProofPayee } from "./type";
import { PAYEE_ENDED_TAG } from "./utils";

type ProofPayeeOptionProps = {
  // Unique within the picker: the radio's id.
  inputId: string;
  payee: ProofPayee;
};

// One tenant of "Pembayaran untuk siapa?" as a large radio row: the name, the unit, and "Sewa
// selesai" for a tenant whose every lease ended (they may still owe, so they can be chosen).
export function ProofPayeeOption({ inputId, payee }: ProofPayeeOptionProps) {
  return (
    <Label
      htmlFor={inputId}
      className="min-h-16 gap-3 rounded-xl border-[1.5px] bg-card px-3.5 py-2.5 leading-normal has-[button[data-state=checked]]:border-2 has-[button[data-state=checked]]:border-primary has-[button[data-state=checked]]:bg-accent"
    >
      <RadioGroupItem id={inputId} value={payee.id} className="size-[22px]" />
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="text-base font-semibold md:text-[17px]">
          {payee.name}
        </span>
        {payee.place !== null && (
          <span className="text-sm font-normal text-muted-foreground md:text-base">
            {payee.place}
          </span>
        )}
      </span>
      {payee.ended && (
        <span className="shrink-0 rounded-md bg-muted px-2.5 py-1 text-sm font-medium text-foreground md:text-[15px]">
          {PAYEE_ENDED_TAG}
        </span>
      )}
    </Label>
  );
}
