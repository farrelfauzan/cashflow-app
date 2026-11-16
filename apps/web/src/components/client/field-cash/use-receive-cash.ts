"use client";

import {
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { useWatch } from "react-hook-form";

import { useApiForm } from "@/components/client/form/use-api-form";
import { useRecordCashPayment } from "@/lib/api/generated/field/field";
import type { CashPaymentResult } from "@/lib/api/generated/model";

import type { CashFormOutput, ReceiveCash } from "./type";
import {
  CASH_API_FIELDS,
  CASH_FORM_DEFAULTS,
  cashFormSchema,
  cashPaymentBody,
  recordErrorView,
} from "./utils";

import { useReceiptPhoto } from "./use-receipt-photo";
import { useRefreshFieldCash } from "./use-refresh-field-cash";

type ReceiveCashOptions = {
  orgId: string;
  tenantId: string;
  // Made once per opening of the sheet and sent unchanged on every retry, so a double press or a
  // retry after a lost answer records the cash once (the API answers 200 the second time).
  requestId: string;
};

// One opening of "Terima tunai": the amount (react-hook-form, checked on Simpan), the receipt
// photo (sent before Simpan works, decision 50) and recordCashPayment with the photo's upload id.
//
// The lists behind (Tagihan hari ini, Tunai di tangan, the receipts held) are reloaded when the
// sheet closes or goes, not the moment the cash is recorded: the row that opened the sheet may
// change or go once the tenant has paid, and "Tersimpan" must stay on screen until "Selesai".
// They are reloaded after an answer that may hide a recorded payment too (no answer), and, if the
// sheet goes while the request runs, once it settles.
export function useReceiveCash({
  orgId,
  tenantId,
  requestId,
}: ReceiveCashOptions): ReceiveCash {
  const photo = useReceiptPhoto(orgId, tenantId);
  const refresh = useRefreshFieldCash(orgId);
  const { form, showApiError, alert } = useApiForm({
    schema: cashFormSchema,
    defaultValues: CASH_FORM_DEFAULTS,
    apiFields: CASH_API_FIELDS,
  });
  const amountText = useWatch({ control: form.control, name: "amount" });
  const [result, setResult] = useState<CashPaymentResult | null>(null);
  const [locked, setLocked] = useState(false);

  // Set the moment a request goes out, before React renders `saving`: two quick presses both
  // pass the (async) checks, and only the first may send. Cleared when the API refuses.
  const sent = useRef(false);
  // A request is running (read when the sheet goes, where render state is stale).
  const pendingRef = useRef(false);
  // Cash was recorded, or may have been: the lists are reloaded when the sheet closes.
  const changedRef = useRef(false);
  // The sheet went while a request ran: reload the lists once it settles.
  const refreshOnSettleRef = useRef(false);

  // Mutation-level, so it runs even when the sheet is gone by the time the API answers.
  const record = useRecordCashPayment({
    mutation: {
      onSettled: () => {
        pendingRef.current = false;
        if (!refreshOnSettleRef.current) return;
        refreshOnSettleRef.current = false;
        void refresh();
      },
    },
  });
  const saving = record.isPending;

  function finish() {
    photo.cancel();
    if (pendingRef.current) {
      refreshOnSettleRef.current = true;
      return;
    }
    if (!changedRef.current) return;
    changedRef.current = false;
    void refresh();
  }

  const onGone = useEffectEvent(finish);
  useEffect(() => () => onGone(), []);

  function refused(error: unknown) {
    sent.current = false;
    const view = recordErrorView(error);
    setLocked(view.kind === "unknown");
    switch (view.kind) {
      case "unknown":
        changedRef.current = true;
        form.setError("root.server", { type: "server", message: view.message });
        return;
      case "photo":
        photo.refuse(view.message);
        return;
      case "amount":
        showApiError(error);
        return;
      case "alert":
        form.setError("root.server", { type: "server", message: view.message });
        return;
    }
  }

  function send(fields: CashFormOutput) {
    if (sent.current || saving || result !== null) return;
    if (photo.state.kind !== "sent") return;
    sent.current = true;
    pendingRef.current = true;
    record.mutate(
      {
        orgId,
        tenantId,
        data: cashPaymentBody(fields.amount, requestId, photo.state.uploadId),
      },
      {
        onSuccess: (answer) => {
          changedRef.current = true;
          setLocked(false);
          setResult(answer);
        },
        onError: refused,
      },
    );
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    void form.handleSubmit(send)(event);
  }

  return {
    form,
    alert,
    amountText,
    photo,
    result,
    saving,
    locked,
    dirty:
      result === null &&
      (form.formState.isDirty || photo.state.kind !== "empty"),
    submit,
    finish,
  };
}
