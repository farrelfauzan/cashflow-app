"use client";

import { useId, type Ref } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { cn } from "@/lib/utils";

import type { ChoiceOptionView } from "./type";

type ChoiceRadioGroupProps = {
  // For moving focus to the group (an error about the choice).
  groupRef?: Ref<HTMLDivElement>;
  legend: string;
  // The legend is read by screen readers only.
  legendHidden: boolean;
  options: readonly ChoiceOptionView[];
  value: string;
  onChange: (value: string) => void;
  error: string | null;
};

// A choice of the payment page as large radio rows: each row is a whole label at least 64px tall,
// with a bold label and a line under it (boards "Bayar-Pilih" and "Janji").
export function ChoiceRadioGroup({
  groupRef,
  legend,
  legendHidden,
  options,
  value,
  onChange,
  error,
}: ChoiceRadioGroupProps) {
  const id = useId();
  const errorId = `${id}-error`;

  return (
    <div className="flex flex-col gap-2.5">
      <span
        id={`${id}-legend`}
        className={cn(
          legendHidden
            ? "sr-only"
            : "text-base leading-normal font-semibold md:text-[17px]",
        )}
      >
        {legend}
      </span>
      <RadioGroup
        ref={groupRef}
        tabIndex={-1}
        aria-labelledby={`${id}-legend`}
        aria-describedby={error === null ? undefined : errorId}
        aria-invalid={error !== null}
        value={value}
        onValueChange={onChange}
        className="gap-2.5 outline-none"
      >
        {options.map((option) => (
          <Label
            key={option.value}
            htmlFor={`${id}-${option.value}`}
            className="min-h-16 gap-3 rounded-xl border-[1.5px] bg-card px-4 py-3 leading-normal has-[button[data-state=checked]]:border-2 has-[button[data-state=checked]]:border-primary has-[button[data-state=checked]]:bg-accent"
          >
            <RadioGroupItem
              id={`${id}-${option.value}`}
              value={option.value}
              className="size-[22px]"
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
        ))}
      </RadioGroup>
      {error !== null && <FieldError id={errorId} message={error} />}
    </div>
  );
}
