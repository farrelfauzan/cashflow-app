"use client";

import { useId } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import type { PropertyKind } from "@/lib/api/generated/model";
import {
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
  PROPERTY_KINDS,
  propertyKindLabel,
} from "./utils";

type PropertyKindFieldProps = {
  value: PropertyKind | null;
  onChange: (kind: PropertyKind) => void;
  error: string | undefined;
  disabled: boolean;
};

// "Jenis properti": Kos, Kontrakan, Ruko or Lainnya, as a radio group. Each choice is a whole
// label, 52px tall.
export function PropertyKindField({
  value,
  onChange,
  error,
  disabled,
}: PropertyKindFieldProps) {
  const id = useId();
  const errorId = `${id}-error`;
  const hintId = `${id}-hint`;

  return (
    <div className="flex flex-col gap-2">
      <span id={`${id}-legend`} className={FIELD_LABEL_CLASS}>
        Jenis properti
      </span>
      <RadioGroup
        aria-labelledby={`${id}-legend`}
        aria-describedby={error !== undefined ? `${errorId} ${hintId}` : hintId}
        aria-invalid={error !== undefined}
        value={value ?? ""}
        onValueChange={(next) => {
          const kind = PROPERTY_KINDS.find((candidate) => candidate === next);
          if (kind !== undefined) onChange(kind);
        }}
        disabled={disabled}
        className="grid-cols-2 gap-2.5"
      >
        {PROPERTY_KINDS.map((kind) => (
          <Label
            key={kind}
            htmlFor={`${id}-${kind}`}
            className="min-h-13 gap-2.5 rounded-xl border-[1.5px] bg-card px-3 text-base leading-normal font-normal md:px-3.5 md:text-[17px]"
          >
            <RadioGroupItem
              id={`${id}-${kind}`}
              value={kind}
              className="size-[22px]"
            />
            {propertyKindLabel(kind)}
          </Label>
        ))}
      </RadioGroup>
      {error !== undefined && <FieldError id={errorId} message={error} />}
      <p id={hintId} className={`${FIELD_HINT_CLASS} hidden md:block`}>
        Hanya untuk mengelompokkan di laporan.
      </p>
    </div>
  );
}
