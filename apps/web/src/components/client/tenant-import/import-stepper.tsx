"use client";

import { Check } from "lucide-react";

import { cn } from "@/lib/utils";
import type { StepNumber } from "./type";
import { STEP_LABELS, STEPS, stepProgressText } from "./utils";

type ImportStepperProps = {
  step: StepNumber;
};

// Where the import is: numbered steps on iPad, a three-part bar with "Langkah N dari 3: ..." on
// the phone (boards Impor*.dc.html). Only one of the two is displayed.
export function ImportStepper({ step }: ImportStepperProps) {
  return (
    <>
      <ol aria-label="Langkah impor" className="hidden items-center lg:flex">
        {STEPS.map((n) => (
          <li
            key={n}
            aria-current={n === step ? "step" : undefined}
            className="flex items-center"
          >
            {n > 1 && (
              <span aria-hidden="true" className="mx-2.5 h-0.5 w-9 bg-border" />
            )}
            <span
              aria-hidden="true"
              className={cn(
                "flex size-[30px] shrink-0 items-center justify-center rounded-full text-[15px] font-bold",
                n <= step
                  ? "bg-primary text-primary-foreground"
                  : "border-[1.5px] border-border bg-card text-muted-foreground",
              )}
            >
              {n < step ? <Check className="size-4" strokeWidth={3} /> : n}
            </span>
            <span
              className={cn(
                "ml-2 text-base",
                n === step
                  ? "font-bold text-foreground"
                  : "font-medium text-muted-foreground",
              )}
            >
              {STEP_LABELS[n]}
            </span>
          </li>
        ))}
      </ol>
      <div className="flex flex-col gap-2 lg:hidden">
        <div aria-hidden="true" className="flex gap-1.5">
          {STEPS.map((n) => (
            <span
              key={n}
              className={cn(
                "h-1.5 flex-1 rounded-full",
                n <= step ? "bg-primary" : "bg-border",
              )}
            />
          ))}
        </div>
        <p className="text-[15px] font-semibold">{stepProgressText(step)}</p>
      </div>
    </>
  );
}
