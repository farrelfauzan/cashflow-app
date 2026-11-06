"use client";

import { Button } from "@/components/ui/button";

import type { QuickAmountView } from "./type";
import { PARTIAL_QUICK_LABEL } from "./utils";

type QuickAmountGridProps = {
  chips: readonly QuickAmountView[];
  // The field's valid amount; the chip with the same amount shows pressed.
  selected: number | null;
  onSelect: (amount: number) => void;
};

// The quick amounts of "Bayar sebagian", two per row; one tap fills the field.
export function QuickAmountGrid({
  chips,
  selected,
  onSelect,
}: QuickAmountGridProps) {
  if (chips.length === 0) return null;
  return (
    <div
      role="group"
      aria-label={PARTIAL_QUICK_LABEL}
      className="grid grid-cols-2 gap-2"
    >
      {chips.map((chip) => {
        const pressed = chip.amount === selected;
        return (
          <Button
            key={chip.amount}
            type="button"
            variant={pressed ? "secondary" : "outline"}
            aria-pressed={pressed}
            onClick={() => onSelect(chip.amount)}
            className="h-auto min-h-11 px-3 py-2 text-sm font-semibold whitespace-normal aria-pressed:border-primary aria-pressed:ring-1 aria-pressed:ring-primary md:text-base"
          >
            {chip.label}
          </Button>
        );
      })}
    </div>
  );
}
