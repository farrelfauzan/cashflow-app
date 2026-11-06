"use client";

import { LoaderCircle } from "lucide-react";
import { Controller, useWatch } from "react-hook-form";

import { FormAlert } from "@/components/client/form/form-alert";
import { useApiForm } from "@/components/client/form/use-api-form";
import { Button } from "@/components/ui/button";
import type { PaymentPage, PaymentPromise } from "@/lib/api/generated/model";
import { PromiseKind } from "@/lib/api/generated/model";

import { BackButton } from "./back-button";
import { ChoiceRadioGroup } from "./choice-radio-group";
import { PaymentPageHeader } from "./payment-page-header";
import { PromiseAmountField } from "./promise-amount-field";
import { PromiseDateField } from "./promise-date-field";
import type {
  PaymentPageHeaderView,
  PromiseDraft,
  PromiseOfferView,
  PromisePaying,
} from "./type";
import { useMakePromise } from "./use-make-promise";
import {
  PROMISE_FORM_API_FIELDS,
  PROMISE_KIND_LEGEND,
  PROMISE_SAVING_TEXT,
  offeredPromiseKind,
  promiseFormError,
  promiseFormView,
  promiseSchema,
  toPromiseKind,
} from "./utils";

type PromiseFormProps = {
  // The page's header with "Kapan sisanya dilunasi?" as its title.
  header: PaymentPageHeaderView;
  offer: PromiseOfferView;
  // What is paid right after saving; null when nothing is paid now.
  paying: PromisePaying | null;
  // "Ubah janji": the primary reads "Simpan perubahan".
  editing: boolean;
  // The tenant's active promise, which saving replaces; null when there is none.
  active: PaymentPromise | null;
  // The fields as they open: empty, or the active promise's.
  initial: PromiseDraft;
  token: string;
  refetchPage: () => Promise<PaymentPage | undefined>;
  onLinkGone: () => void;
  onSaved: (promise: PaymentPromise) => void;
  onBack: () => void;
};

// Janji pelunasan (board "Janji"): when the rest is paid ("Bareng tagihan …" or a date of the
// tenant's own) and, optionally, how much. Checked by comparison on save, then by the API, whose
// message goes under the field it names or in the alert.
export function PromiseForm({
  header,
  offer,
  paying,
  editing,
  active,
  initial,
  token,
  refetchPage,
  onLinkGone,
  onSaved,
  onBack,
}: PromiseFormProps) {
  const { save, pending } = useMakePromise({ token, refetchPage, onLinkGone });
  const formView = promiseFormView(offer, paying, editing, active);
  const { form, showApiError, alert } = useApiForm({
    schema: promiseSchema(offer, paying),
    defaultValues: initial,
    apiFields: PROMISE_FORM_API_FIELDS,
    // `kind` (below) is this render's, the one whose submit sent the request, so the date's
    // error goes under its input only when that request was for "Pilih tanggal sendiri".
    apiError: (error) => promiseFormError(error, kind),
  });
  const kind = offeredPromiseKind(
    useWatch({ control: form.control, name: "kind" }),
    offer,
  );

  const submit = form.handleSubmit(async (body) => {
    if (pending) return;
    const result = await save(body);
    if (result.kind === "saved") {
      onSaved(result.promise);
      return;
    }
    if (result.kind === "failed" && result.error.kind === "shown") {
      showApiError(result.cause);
    }
  });

  return (
    <form
      noValidate
      onSubmit={(event) => void submit(event)}
      className="flex flex-1 flex-col gap-4 md:gap-5"
    >
      <PaymentPageHeader header={header} />
      {formView.replacesLine !== null && (
        <p className="text-sm font-semibold text-warning md:text-base">
          {formView.replacesLine}
        </p>
      )}
      <p className="text-sm font-semibold md:text-base">{formView.subline}</p>
      <Controller
        control={form.control}
        name="kind"
        render={({ field, fieldState }) => (
          <ChoiceRadioGroup
            groupRef={field.ref}
            legend={PROMISE_KIND_LEGEND}
            legendHidden
            options={formView.kindOptions}
            value={kind}
            onChange={(value) => {
              const next = toPromiseKind(value);
              if (next === null) return;
              field.onChange(next);
              form.clearErrors(["kind", "date"]);
            }}
            error={fieldState.error?.message ?? null}
          />
        )}
      />
      {kind === PromiseKind.specific_date && (
        <Controller
          control={form.control}
          name="date"
          render={({ field, fieldState }) => (
            <PromiseDateField
              inputRef={field.ref}
              value={field.value}
              min={formView.dateMin}
              max={formView.dateMax}
              onChange={(date) => {
                field.onChange(date);
                form.clearErrors("date");
              }}
              error={fieldState.error?.message ?? null}
            />
          )}
        />
      )}
      <Controller
        control={form.control}
        name="amountText"
        render={({ field, fieldState }) => (
          <PromiseAmountField
            inputRef={field.ref}
            text={field.value}
            hint={formView.amountHint}
            onTextChange={(amountText) => {
              field.onChange(amountText);
              form.clearErrors("amountText");
            }}
            error={fieldState.error?.message ?? null}
          />
        )}
      />
      {alert !== undefined && <FormAlert message={alert} />}
      <div className="mt-auto flex flex-col gap-2">
        <Button
          type="submit"
          size="lg"
          className="w-full font-semibold"
          disabled={pending}
        >
          {pending && (
            <LoaderCircle aria-hidden="true" className="animate-spin" />
          )}
          {pending ? PROMISE_SAVING_TEXT : formView.primaryLabel}
        </Button>
        <BackButton onClick={onBack} disabled={pending} />
      </div>
    </form>
  );
}
