"use client";

import { useRef, type FormEvent } from "react";

import { useApiForm } from "@/components/client/form/use-api-form";
import { notify } from "@/components/client/toast/notify";
import type { ToastMessage } from "@/components/client/toast/type";
import { useDismissCheckItem } from "@/lib/api/generated/check-queue/check-queue";
import type { CheckQueueKind } from "@/lib/api/generated/model";

import type { CheckRefreshScope, NoteFormOutput } from "./type";
import {
  checkActionErrorKind,
  dismissFormSchema,
  dismissRequestBody,
  NOTE_API_FIELDS,
  NOTE_FORM_DEFAULTS,
  RESOLVED_TOAST,
} from "./utils";
import { useRefreshCheckQueue } from "./use-refresh-check-queue";

type DismissFormOptions = {
  orgId: string;
  kind: CheckQueueKind;
  itemId: string;
  // The friendly early checks: dismissFormSchema, or rejectFormSchema for a new number.
  schema?: typeof dismissFormSchema;
  toast: ToastMessage;
  refreshScope?: CheckRefreshScope;
  // Closes the drawer, after a save or when someone answered the item first.
  onDone: () => void;
};

// A note that takes an item off the queue (dismissCheckItem): the form, its submit with a guard
// against a double press, and the answers. A success closes the drawer with a toast and reloads
// the queue; an item someone answered first does the same with "Sudah ditangani admin lain";
// any other refusal shows the API's message under the note or in the alert.
export function useDismissForm({
  orgId,
  kind,
  itemId,
  schema = dismissFormSchema,
  toast,
  refreshScope,
  onDone,
}: DismissFormOptions) {
  const { form, field, showApiError, alert } = useApiForm({
    schema,
    defaultValues: NOTE_FORM_DEFAULTS,
    apiFields: NOTE_API_FIELDS,
  });
  const refresh = useRefreshCheckQueue(orgId);

  // The toast is raised here, so it shows whatever happens to the drawer.
  const dismiss = useDismissCheckItem({
    mutation: {
      onSuccess: () => {
        notify(toast);
        return refresh(refreshScope);
      },
    },
  });
  const saving = dismiss.isPending || dismiss.isSuccess;

  // Set the moment a request goes out, before React renders `saving`: two quick presses both pass
  // the (async) checks, and only the first may send. Cleared when the API refuses.
  const sent = useRef(false);

  function refused(error: unknown) {
    sent.current = false;
    if (checkActionErrorKind(error) === "resolved") {
      notify(RESOLVED_TOAST);
      void refresh(refreshScope);
      onDone();
      return;
    }
    showApiError(error);
  }

  function send(fields: NoteFormOutput) {
    if (saving || sent.current) return;
    sent.current = true;
    dismiss.mutate(
      { orgId, kind, itemId, data: dismissRequestBody(fields) },
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
    pending: dismiss.isPending,
  };
}
