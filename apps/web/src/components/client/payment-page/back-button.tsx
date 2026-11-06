"use client";

import { Button } from "@/components/ui/button";

import { BACK_TEXT } from "./utils";

type BackButtonProps = {
  onClick: () => void;
  // While a save runs, so the answer never lands on another view.
  disabled?: boolean;
};

// "Kembali": the secondary that returns to the previous view of the payment page.
export function BackButton({ onClick, disabled = false }: BackButtonProps) {
  return (
    <Button
      type="button"
      variant="outline"
      size="lg"
      className="w-full font-semibold"
      disabled={disabled}
      onClick={onClick}
    >
      {BACK_TEXT}
    </Button>
  );
}
