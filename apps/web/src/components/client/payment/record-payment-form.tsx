"use client";

import { useId, useRef, type FormEvent } from "react";
import { Controller } from "react-hook-form";

import { DateField } from "@/components/client/form/date-field";
import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { RupiahField } from "@/components/client/form/rupiah-field";
import { useApiForm } from "@/components/client/form/use-api-form";
import {
  FIELD_LABEL_CLASS,
  TEXTAREA_CLASS,
} from "@/components/client/form/utils";
import { LeaseInfoNote } from "@/components/client/lease/lease-info-note";
import { LeasePanelFooter } from "@/components/client/lease/lease-panel-footer";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { PaymentResult } from "@/lib/api/generated/model";
import { useRecordPayment } from "@/lib/api/generated/money/money";
import {
  AMOUNT_HINT,
  FIFO_NOTE,
  NOTE_PLACEHOLDER,
  PAYMENT_API_FIELDS,
  PAYMENT_NOTE_MAX_LENGTH,
  paymentFormDefaults,
  paymentFormSchema,
  paymentRequestBody,
  RECEIVED_ON_HINT,
} from "./utils";
import type { PaymentFormOutput } from "./type";

import { PaymentMethodField } from "./payment-method-field";
import { PaymentSummaryBox } from "./payment-summary-box";
import { useRefreshPayment } from "./use-refresh-payment";

type RecordPaymentFormProps = {
  orgId: string;
  tenantId: string;
  // "Today" in Jakarta: the default and the latest "Tanggal diterima".
  today: string;
  // Made once when the panel opened, sent unchanged on every retry, so a double submit or a
  // retry after a lost answer records the payment once.
  requestId: string;
  onSaved: (result: PaymentResult) => void;
};

// "Catat pembayaran" (board "Catat pembayaran"): what the tenant owes, then the amount received,
// how and when, and a note. The API pays the oldest invoices first and keeps the rest as credit;
// the panel then shows its answer.
export function RecordPaymentForm({
  orgId,
  tenantId,
  today,
  requestId,
  onSaved,
}: RecordPaymentFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const refresh = useRefreshPayment(orgId, tenantId);
  const { form, field, showApiError, alert } = useApiForm({
    schema: paymentFormSchema(today),
    defaultValues: paymentFormDefaults(today),
    apiFields: PAYMENT_API_FIELDS,
  });
  const noteError = form.formState.errors.note?.message;

  // The page behind is reloaded before the result shows, so it is current once the panel closes.
  const save = useRecordPayment({ mutation: { onSuccess: () => refresh() } });
  const saving = save.isPending || save.isSuccess;

  // Set the moment a request goes out, before React renders `saving`: two quick presses both
  // pass the (async) checks, and only the first may send. Cleared when the API refuses, so the
  // member can fix the form and send again with the same requestId.
  const sent = useRef(false);

  function send(fields: PaymentFormOutput) {
    if (saving || sent.current) return;
    sent.current = true;
    save.mutate(
      { orgId, data: paymentRequestBody(fields, requestId, tenantId) },
      {
        onSuccess: onSaved,
        onError: (error) => {
          sent.current = false;
          showApiError(error);
        },
      },
    );
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    void form.handleSubmit(send)(event);
  }

  return (
    <>
      <form
        id={formId}
        noValidate
        onSubmit={submit}
        className="flex min-h-0 flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-5"
      >
        {alert !== undefined && <FormAlert message={alert} />}
        <PaymentSummaryBox orgId={orgId} tenantId={tenantId} />
        <Controller
          control={form.control}
          name="amount"
          render={({ field: amount, fieldState }) => (
            <RupiahField
              inputRef={amount.ref}
              label="Jumlah diterima"
              name="amount"
              placeholder="0"
              hint={AMOUNT_HINT}
              value={amount.value}
              onChange={(text) => {
                amount.onChange(text);
                form.clearErrors("amount");
              }}
              error={fieldState.error?.message}
              disabled={saving}
            />
          )}
        />
        <Controller
          control={form.control}
          name="method"
          render={({ field: method, fieldState }) => (
            <PaymentMethodField
              inputRef={method.ref}
              value={method.value}
              onChange={(next) => {
                method.onChange(next);
                form.clearErrors("method");
              }}
              error={fieldState.error?.message}
              disabled={saving}
            />
          )}
        />
        <Controller
          control={form.control}
          name="receivedOn"
          render={({ field: receivedOn, fieldState }) => (
            <DateField
              inputRef={receivedOn.ref}
              label="Tanggal diterima"
              name="receivedOn"
              value={receivedOn.value}
              max={today}
              onChange={(date) => {
                receivedOn.onChange(date);
                form.clearErrors("receivedOn");
              }}
              hint={RECEIVED_ON_HINT}
              error={fieldState.error?.message}
              disabled={saving}
            />
          )}
        />
        <div className="flex flex-col gap-2">
          <Label htmlFor={`${id}-note`} className={FIELD_LABEL_CLASS}>
            Catatan (opsional)
          </Label>
          <Textarea
            id={`${id}-note`}
            autoComplete="off"
            maxLength={PAYMENT_NOTE_MAX_LENGTH}
            placeholder={NOTE_PLACEHOLDER}
            {...field("note")}
            disabled={saving}
            aria-invalid={noteError !== undefined}
            aria-describedby={
              noteError !== undefined ? `${id}-note-error` : undefined
            }
            className={TEXTAREA_CLASS}
          />
          {noteError !== undefined && (
            <FieldError id={`${id}-note-error`} message={noteError} />
          )}
        </div>
        <LeaseInfoNote text={FIFO_NOTE} />
      </form>
      <LeasePanelFooter
        formId={formId}
        cancelLabel="Batal"
        submitLabel="Simpan pembayaran"
        saving={saving}
        wide={false}
      />
    </>
  );
}
