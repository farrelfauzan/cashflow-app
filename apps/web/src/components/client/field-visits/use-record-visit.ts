"use client";

import { useQueryClient } from "@tanstack/react-query";
import {
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { useWatch } from "react-hook-form";

import { useApiForm } from "@/components/client/form/use-api-form";
import { notify } from "@/components/client/toast/notify";
import {
  getListFieldTodayQueryKey,
  getListFieldVisitsQueryKey,
  useRecordVisitOutcome,
} from "@/lib/api/generated/field/field";
import type { FieldVisit, VisitOutcomeInput } from "@/lib/api/generated/model";

import type { RecordVisit, VisitOutcomeFormOutput } from "./type";
import {
  SAVED_TOAST_TITLE,
  VISIT_API_FIELDS,
  VISIT_FORM_DEFAULTS,
  canSaveOutcome,
  cashChoices,
  defaultOutcome,
  promiseSummary,
  selectedPaymentId,
  visitErrorView,
  visitOutcomeBody,
  visitOutcomeSchema,
} from "./utils";

type RecordVisitOptions = {
  orgId: string;
  // The tenant's row as the list holds it now: its cash and promise change while the sheet is
  // open (a nested Terima tunai or Catat janji bayar reloads the list).
  visit: FieldVisit;
  // Made once per opening of the sheet and sent unchanged on every retry, so a double press or a
  // retry after a lost answer records the outcome once (the API answers 200 the second time).
  requestId: string;
  // The outcome is recorded: the sheet closes.
  onSaved: () => void;
};

// One opening of "Catat hasil kunjungan", as useReceiveCash is for "Terima tunai": the answer,
// the number and the note (react-hook-form, checked on Simpan), the cash or promise it names (from
// the list), and recordVisitOutcome. A request that goes out is kept: after a lost answer the
// form is locked and Simpan sends that same request again, even if the list behind has changed
// since (the cash it named may have left unlinkedCashToday because it was recorded).
export function useRecordVisit({
  orgId,
  visit,
  requestId,
  onSaved,
}: RecordVisitOptions): RecordVisit {
  const queryClient = useQueryClient();
  const { form, showApiError, alert } = useApiForm({
    schema: visitOutcomeSchema,
    defaultValues: { ...VISIT_FORM_DEFAULTS, outcome: defaultOutcome(visit) },
    apiFields: VISIT_API_FIELDS,
  });
  const outcome = useWatch({ control: form.control, name: "outcome" });

  // The cash picked under "Sudah bayar hari ini"; the newest when none is. When newer cash
  // arrives (Terima tunai from this sheet), it is the one selected.
  const payments = visit.unlinkedCashToday;
  const newest = payments.at(-1)?.paymentId ?? null;
  const [chosenPayment, setChosenPayment] = useState<string | null>(null);
  const [seenNewest, setSeenNewest] = useState(newest);
  if (newest !== seenNewest) {
    setSeenNewest(newest);
    setChosenPayment(null);
  }
  const paymentId = selectedPaymentId(payments, chosenPayment);
  const promise = promiseSummary(visit.unlinkedPromiseToday);
  const links = {
    paymentId,
    promiseVersionId: promise?.promiseVersionId ?? null,
  };

  const [locked, setLocked] = useState(false);
  // The request that went out last; resent as it is after a lost answer.
  const sentBody = useRef<VisitOutcomeInput | null>(null);
  // Set the moment a request goes out, before React renders `saving`: two quick presses both
  // pass the (async) checks, and only the first may send.
  const sending = useRef(false);
  // An outcome was recorded, or may have been: the lists are reloaded when the sheet closes.
  const changedRef = useRef(false);

  const reload = () =>
    Promise.all(
      [getListFieldVisitsQueryKey(orgId), getListFieldTodayQueryKey(orgId)].map(
        (queryKey) => queryClient.invalidateQueries({ queryKey }),
      ),
    );

  // Mutation-level, so the toast and the reload happen even if the sheet is gone by the time the
  // API answers.
  const record = useRecordVisitOutcome({
    mutation: {
      onSuccess: () => {
        notify({
          tone: "create",
          title: SAVED_TOAST_TITLE,
          description: visit.tenantName,
        });
        return reload();
      },
    },
  });
  const saving = record.isPending;

  function finish() {
    if (!changedRef.current) return;
    changedRef.current = false;
    void reload();
  }
  const onGone = useEffectEvent(finish);
  useEffect(() => () => onGone(), []);

  function refused(error: unknown) {
    sending.current = false;
    const view = visitErrorView(error);
    setLocked(view.kind === "unknown");
    switch (view.kind) {
      case "unknown":
        changedRef.current = true;
        form.setError("root.server", { type: "server", message: view.message });
        return;
      case "field":
        showApiError(error);
        return;
      case "stale":
        void reload();
        form.setError("root.server", { type: "server", message: view.message });
        return;
      case "alert":
        form.setError("root.server", { type: "server", message: view.message });
        return;
    }
  }

  function post(body: VisitOutcomeInput) {
    if (sending.current || saving) return;
    sending.current = true;
    sentBody.current = body;
    record.mutate(
      { orgId, tenantId: visit.tenantId, data: body },
      {
        onSuccess: () => {
          sending.current = false;
          changedRef.current = false;
          setLocked(false);
          onSaved();
        },
        onError: refused,
      },
    );
  }

  function send(fields: VisitOutcomeFormOutput) {
    const body = visitOutcomeBody(fields, links, requestId);
    if (body !== null) post(body);
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    // Locked, the fields are disabled and the request already sent goes again unchanged, without
    // checking the form (or the list behind it) again.
    if (locked && sentBody.current !== null) {
      event.preventDefault();
      post(sentBody.current);
      return;
    }
    void form.handleSubmit(send)(event);
  }

  return {
    form,
    outcome,
    alert,
    cashChoices: cashChoices(payments),
    paymentId,
    choosePayment: setChosenPayment,
    promise,
    // Locked, the request already sent goes again, whatever the list shows now.
    canSave: locked ? !saving : canSaveOutcome(outcome, links, saving),
    saving,
    locked,
    dirty: form.formState.isDirty,
    submit,
    finish,
  };
}
