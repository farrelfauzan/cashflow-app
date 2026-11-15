"use client";

import { useRef, useState, type FormEvent } from "react";

import { useApiForm } from "@/components/client/form/use-api-form";
import { useRefreshPayment } from "@/components/client/payment/use-refresh-payment";
import { notify } from "@/components/client/toast/notify";
import { useMatchTransferProof } from "@/lib/api/generated/check-queue/check-queue";

import type { ProofMatchOutput } from "./type";
import {
  checkActionErrorKind,
  PROOF_MATCH_API_FIELDS,
  proofMatchDefaults,
  proofMatchedToast,
  proofMatchRequestBody,
  proofMatchSchema,
  RESOLVED_TOAST,
  UPLOAD_REFUSED_TOAST,
} from "./utils";
import { useRefreshCheckQueue } from "./use-refresh-check-queue";

type MatchProofFormOptions = {
  orgId: string;
  itemId: string;
  tenantId: string;
  // The toast names the tenant.
  tenantName: string;
  // "Today" in Jakarta: the default and the latest "Tanggal masuk".
  today: string;
  // Closes the drawer, after a save or when the proof can no longer be matched.
  onDone: () => void;
  // The API says the photo is still being scanned: read it again.
  onNotReady: () => void;
};

// "Simpan pembayaran" (matchTransferProof): the form, its submit with a guard against a double
// press, and the answers. The requestId is made once when the drawer opens (the hook mounts with
// it) and sent unchanged on every retry, so a double submit or a retry after a lost answer records
// the payment once. A success closes the drawer with a toast and reloads the queue, the tenant,
// their account and open invoices, the dashboard and every invoice list (Penagihan's figures);
// cached money is never patched. A proof someone answered first, or whose photo the scan refused,
// closes the drawer and reloads the queue; any other refusal shows the API's message under the
// field it names or in the alert.
export function useMatchProofForm({
  orgId,
  itemId,
  tenantId,
  tenantName,
  today,
  onDone,
  onNotReady,
}: MatchProofFormOptions) {
  const [requestId] = useState(() => crypto.randomUUID());
  const { form, field, showApiError, alert } = useApiForm({
    schema: proofMatchSchema(today),
    defaultValues: proofMatchDefaults(today),
    apiFields: PROOF_MATCH_API_FIELDS,
  });
  const refreshQueue = useRefreshCheckQueue(orgId);
  const refreshPayment = useRefreshPayment(orgId, tenantId);
  async function refresh() {
    await Promise.all([refreshQueue({ tenantId }), refreshPayment()]);
  }

  // The toast is raised here, so it shows whatever happens to the drawer.
  const match = useMatchTransferProof({
    mutation: {
      onSuccess: (result) => {
        notify(proofMatchedToast(result, tenantName));
        return refresh();
      },
    },
  });
  const saving = match.isPending || match.isSuccess;

  // Set the moment a request goes out, before React renders `saving`: two quick presses both pass
  // the (async) checks, and only the first may send. Cleared when the API refuses.
  const sent = useRef(false);

  function refused(error: unknown) {
    sent.current = false;
    switch (checkActionErrorKind(error)) {
      case "resolved":
        notify(RESOLVED_TOAST);
        void refresh();
        onDone();
        return;
      case "upload_refused":
        notify(UPLOAD_REFUSED_TOAST);
        void refreshQueue();
        onDone();
        return;
      case "upload_not_ready":
        onNotReady();
        showApiError(error);
        return;
      default:
        showApiError(error);
    }
  }

  function send(fields: ProofMatchOutput) {
    if (saving || sent.current) return;
    sent.current = true;
    match.mutate(
      { orgId, itemId, data: proofMatchRequestBody(fields, requestId) },
      { onSuccess: onDone, onError: refused },
    );
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    void form.handleSubmit(send)(event);
  }

  return {
    form,
    noteField: field("note"),
    noteError: form.formState.errors.note?.message,
    alert,
    submit,
    saving,
    pending: match.isPending,
  };
}
