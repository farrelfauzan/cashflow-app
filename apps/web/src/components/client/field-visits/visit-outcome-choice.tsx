"use client";

import { cn } from "cn";
import type { ReactNode } from "react";

import { Label } from "@/components/ui/label";
import { RadioGroupItem } from "@/components/ui/radio-group";

import type { VisitOutcomeOption } from "./type";

type VisitOutcomeChoiceProps = {
  // The radio's id, for its label.
  id: string;
  option: VisitOutcomeOption;
  checked: boolean;
  disabled: boolean;
  // What the answer needs, shown inside its card once it is chosen (the cash, the promise, the
  // number); none for "Tidak bertemu".
  children?: ReactNode;
};

// One answer of "Hasil kunjungan" (board HP-Kunjungan, columns 2 to 5): a card at least 64px tall
// whose label row is the radio, and, once chosen, what that answer needs below it, inside the
// card but outside the label, so its own buttons and fields work on their own.
export function VisitOutcomeChoice({
  id,
  option,
  checked,
  disabled,
  children,
}: VisitOutcomeChoiceProps) {
  return (
    <div
      className={cn(
        "flex flex-col rounded-xl border-[1.5px] bg-card",
        checked && "border-2 border-primary bg-accent",
      )}
    >
      <Label htmlFor={id} className="min-h-16 gap-3 px-4 py-3 leading-normal">
        <RadioGroupItem
          id={id}
          value={option.value}
          disabled={disabled}
          className="size-[22px] bg-card"
        />
        <span className="flex min-w-0 flex-col gap-0.5">
          <span className="text-base font-semibold md:text-[17px]">
            {option.label}
          </span>
          <span className="text-sm font-normal text-muted-foreground md:text-base">
            {option.sub}
          </span>
        </span>
      </Label>
      {checked && children !== undefined && (
        <div className="flex flex-col gap-2 px-3.5 pb-3.5 md:px-4">
          {children}
        </div>
      )}
    </div>
  );
}
