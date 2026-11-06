"use client";

import { LoaderCircle } from "lucide-react";
import { useRef, useState, type FormEvent } from "react";

import { FormAlert } from "@/components/client/form/form-alert";
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
  PromiseErrorTarget,
  PromiseFormError,
  PromiseOfferView,
  PromisePaying,
} from "./type";
import { useMakePromise } from "./use-make-promise";
import {
  PROMISE_KIND_LEGEND,
  PROMISE_SAVING_TEXT,
  offeredPromiseKind,
  promiseFormView,
  promiseRequest,
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
  initial,
  token,
  refetchPage,
  onLinkGone,
  onSaved,
  onBack,
}: PromiseFormProps) {
  const [draft, setDraft] = useState<PromiseDraft>(initial);
  const [error, setError] = useState<PromiseFormError | null>(null);
  const { save, pending } = useMakePromise({ token, refetchPage, onLinkGone });

  const kindRef = useRef<HTMLDivElement>(null);
  const dateRef = useRef<HTMLInputElement>(null);
  const amountRef = useRef<HTMLInputElement>(null);

  const formView = promiseFormView(offer, paying, editing);
  const kind = offeredPromiseKind(draft.kind, offer);

  function errorFor(target: PromiseErrorTarget): string | null {
    return error !== null && error.target === target ? error.message : null;
  }

  function show(next: PromiseFormError) {
    setError(next);
    const target =
      next.target === "kind"
        ? kindRef.current
        : next.target === "promisedDate"
          ? dateRef.current
          : next.target === "promisedAmount"
            ? amountRef.current
            : null;
    target?.focus();
  }

  // Editing a field clears its error; the next save checks it again.
  function edit(change: Partial<PromiseDraft>, clears: PromiseErrorTarget[]) {
    setDraft((current) => ({ ...current, ...change }));
    if (error !== null && clears.includes(error.target)) setError(null);
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) return;
    const check = promiseRequest(draft, offer, paying);
    if (check.kind === "invalid") {
      show(check.error);
      return;
    }
    setError(null);
    const result = await save(check.body);
    if (result.kind === "saved") {
      onSaved(result.promise);
      return;
    }
    if (result.kind === "failed" && result.error.kind === "shown") {
      show(result.error.error);
    }
  }

  const alert = errorFor("alert");

  return (
    <form
      noValidate
      onSubmit={(event) => void submit(event)}
      className="flex flex-1 flex-col gap-4 md:gap-5"
    >
      <PaymentPageHeader header={header} />
      <p className="text-sm font-semibold md:text-base">{formView.subline}</p>
      <ChoiceRadioGroup
        groupRef={kindRef}
        legend={PROMISE_KIND_LEGEND}
        legendHidden
        options={formView.kindOptions}
        value={kind}
        onChange={(value) => {
          const next = toPromiseKind(value);
          if (next !== null) edit({ kind: next }, ["kind", "promisedDate"]);
        }}
        error={errorFor("kind")}
      />
      {kind === PromiseKind.specific_date && (
        <PromiseDateField
          inputRef={dateRef}
          value={draft.date}
          min={formView.dateMin}
          max={formView.dateMax}
          onChange={(date) => edit({ date }, ["promisedDate"])}
          error={errorFor("promisedDate")}
        />
      )}
      <PromiseAmountField
        inputRef={amountRef}
        text={draft.amountText}
        hint={formView.amountHint}
        onTextChange={(amountText) => edit({ amountText }, ["promisedAmount"])}
        error={errorFor("promisedAmount")}
      />
      {alert !== null && <FormAlert message={alert} />}
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
