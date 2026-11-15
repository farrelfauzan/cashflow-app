"use client";

import { useRef, useState } from "react";

import { notify } from "@/components/client/toast/notify";
import type { ToastMessage } from "@/components/client/toast/type";
import { toFormError, type FormError } from "@/lib/api/form-error";
import { useApplyPhoneChange as useApplyPhoneChangeMutation } from "@/lib/api/generated/check-queue/check-queue";

import { checkActionErrorKind, RESOLVED_TOAST } from "./utils";
import { useRefreshCheckQueue } from "./use-refresh-check-queue";

type ApplyPhoneChangeOptions = {
  orgId: string;
  itemId: string;
  tenantId: string;
  toast: ToastMessage;
  onDone: () => void;
};

// "Pakai nomor baru" (applyPhoneChange): one press, guarded against a second. A success closes
// the drawer with a toast and reloads the queue and the tenant; an item someone answered first
// does the same with "Sudah ditangani admin lain". A refusal is kept to show: under the new
// number when another tenant has it (field "phone"), in the alert otherwise (a 403 for a tenant
// the member may not manage, among others).
export function useApplyPhoneChange({
  orgId,
  itemId,
  tenantId,
  toast,
  onDone,
}: ApplyPhoneChangeOptions) {
  const refresh = useRefreshCheckQueue(orgId);
  const [error, setError] = useState<FormError | null>(null);
  const apply = useApplyPhoneChangeMutation({
    mutation: {
      onSuccess: () => {
        notify(toast);
        return refresh({ tenantId });
      },
    },
  });
  const saving = apply.isPending || apply.isSuccess;
  const sent = useRef(false);

  function refused(failure: unknown) {
    sent.current = false;
    if (checkActionErrorKind(failure) === "resolved") {
      notify(RESOLVED_TOAST);
      void refresh({ tenantId });
      onDone();
      return;
    }
    setError(toFormError(failure, ["phone"]));
  }

  function send() {
    if (saving || sent.current) return;
    sent.current = true;
    setError(null);
    apply.mutate({ orgId, itemId }, { onSuccess: onDone, onError: refused });
  }

  return {
    send,
    saving,
    pending: apply.isPending,
    phoneError: error?.field === "phone" ? error.message : undefined,
    alert:
      error !== null && error.field !== "phone" ? error.message : undefined,
  };
}
