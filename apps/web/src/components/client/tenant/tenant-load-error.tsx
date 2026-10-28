"use client";

import { FormAlert } from "@/components/client/form/form-alert";
import { Button } from "@/components/ui/button";

type TenantLoadErrorProps = {
  // The API's own message, already in Bahasa Indonesia.
  message: string;
  onRetry: () => void;
};

// A page that did not load: the API's message and "Coba lagi".
export function TenantLoadError({ message, onRetry }: TenantLoadErrorProps) {
  return (
    <div className="flex flex-col items-start gap-3">
      <FormAlert message={message} />
      <Button
        type="button"
        variant="outline"
        className="font-semibold"
        onClick={onRetry}
      >
        Coba lagi
      </Button>
    </div>
  );
}
