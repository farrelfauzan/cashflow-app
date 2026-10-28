"use client";

import { CircleAlert } from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";

type FormAlertProps = {
  message: string;
};

// A form-level error (rate limit, WhatsApp down, no connection): the API's message at the top
// of the form, announced by screen readers through role="alert".
export function FormAlert({ message }: FormAlertProps) {
  return (
    <Alert
      variant="warning"
      className="rounded-xl px-3.5 py-3 has-[>svg]:grid-cols-[20px_1fr] has-[>svg]:gap-x-2.5 md:px-4 md:py-3.5 md:has-[>svg]:grid-cols-[22px_1fr] [&>svg]:size-5 [&>svg]:translate-y-px md:[&>svg]:size-[22px] md:[&>svg]:translate-y-0"
    >
      <CircleAlert aria-hidden="true" />
      <AlertDescription className="text-[15px] leading-[1.45] md:text-base">
        {message}
      </AlertDescription>
    </Alert>
  );
}
