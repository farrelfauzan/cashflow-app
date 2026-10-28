"use client";

import { CircleAlert } from "lucide-react";

type FieldErrorProps = {
  id: string;
  message: string;
};

// The error under one form field. The field points at it with aria-describedby and sets
// aria-invalid.
export function FieldError({ id, message }: FieldErrorProps) {
  return (
    <p
      id={id}
      role="alert"
      className="flex items-center gap-2 text-[15px] font-medium text-destructive md:text-base"
    >
      <CircleAlert
        aria-hidden="true"
        className="size-[18px] shrink-0 md:size-5"
        strokeWidth={2.2}
      />
      {message}
    </p>
  );
}
