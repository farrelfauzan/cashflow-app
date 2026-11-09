"use client";

import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

import type { DateControlProps } from "./type";
import { INPUT_CLASS } from "./utils";

// The device's own date input (the iPhone's wheel, Android's calendar), used on phones. The
// operating system draws the picker, so the page loads no calendar code.
export function NativeDateInput({
  id,
  name,
  controlRef,
  value,
  min,
  max,
  onChange,
  invalid,
  describedBy,
  disabled,
  className,
}: DateControlProps) {
  return (
    <Input
      ref={controlRef}
      id={id}
      name={name}
      type="date"
      value={value}
      min={min}
      max={max}
      onChange={(event) => onChange(event.target.value)}
      disabled={disabled}
      aria-invalid={invalid}
      aria-describedby={describedBy}
      className={cn(INPUT_CLASS, className)}
    />
  );
}
