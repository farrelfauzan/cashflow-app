"use client";

import { useId, useState } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import type { CheckPhoneChanged } from "@/lib/api/generated/model";

import type { PhoneSheetStep } from "./type";
import {
  PHONE_APPLY,
  PHONE_BACK,
  PHONE_CALLOUT,
  PHONE_REASON_LABEL,
  PHONE_REASON_PLACEHOLDER,
  PHONE_REJECT,
  PHONE_REJECT_SUBMIT,
  PHONE_REJECT_TITLE,
  PHONE_REJECTED_TOAST,
  PHONE_TITLE,
  PHONE_TODO,
  phoneAppliedToast,
  phoneChangeSheetView,
  rejectFormSchema,
} from "./utils";

import { CheckCallout } from "./check-callout";
import { CheckNoteField } from "./check-note-field";
import { CheckSheetFooter } from "./check-sheet-footer";
import { CheckSheetFrame } from "./check-sheet-frame";
import { CheckTiles } from "./check-tiles";
import { CheckTodo } from "./check-todo";
import { useApplyPhoneChange } from "./use-apply-phone-change";
import { useDismissForm } from "./use-dismiss-form";
import { VisitNoteBox } from "./visit-note-box";

type PhoneChangeSheetProps = {
  orgId: string;
  itemId: string;
  // When the karyawan recorded it (the item's createdAt).
  createdAt: string;
  change: CheckPhoneChanged;
  canAct: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

// "Nomor baru": a tenant's new WhatsApp number a karyawan brought back from a visit. "Pakai nomor
// baru" makes it the tenant's (the API also needs tenant:manage on that tenant, so a refusal
// shows in the alert); "Jangan pakai" turns the same drawer into "Jangan pakai nomor baru", which
// takes the item off the queue with the reason.
export function PhoneChangeSheet({
  orgId,
  itemId,
  createdAt,
  change,
  canAct,
  open,
  onOpenChange,
}: PhoneChangeSheetProps) {
  const id = useId();
  const formId = `${id}-form`;
  const view = phoneChangeSheetView(change, createdAt);
  const [step, setStep] = useState<PhoneSheetStep>("review");
  const close = () => onOpenChange(false);
  const apply = useApplyPhoneChange({
    orgId,
    itemId,
    tenantId: change.tenant.id,
    toast: phoneAppliedToast(view),
    onDone: close,
  });
  const reject = useDismissForm({
    orgId,
    kind: "phone_changed",
    itemId,
    schema: rejectFormSchema,
    toast: PHONE_REJECTED_TOAST,
    onDone: close,
  });
  const rejecting = canAct && step === "reject";
  const pending = apply.pending || reject.pending;

  return (
    <CheckSheetFrame
      open={open}
      onOpenChange={onOpenChange}
      busy={pending}
      dirty={reject.form.formState.isDirty}
      title={rejecting ? PHONE_REJECT_TITLE : PHONE_TITLE}
      subtitle={view.subtitle}
      formId={rejecting ? formId : undefined}
      onSubmit={reject.submit}
      footer={
        rejecting ? (
          <CheckSheetFooter
            readOnly={false}
            secondaryLabel={PHONE_BACK}
            onSecondary={() => setStep("review")}
            primaryLabel={PHONE_REJECT_SUBMIT}
            formId={formId}
            saving={reject.saving}
            pending={reject.pending}
          />
        ) : (
          <CheckSheetFooter
            readOnly={!canAct}
            secondaryLabel={PHONE_REJECT}
            onSecondary={() => setStep("reject")}
            primaryLabel={PHONE_APPLY}
            onPrimary={apply.send}
            saving={apply.saving}
            pending={apply.pending}
          />
        )
      }
    >
      {rejecting && reject.alert !== undefined && (
        <FormAlert message={reject.alert} />
      )}
      {!rejecting && apply.alert !== undefined && (
        <FormAlert message={apply.alert} />
      )}
      <CheckTiles tiles={view.tiles} />
      {!rejecting && apply.phoneError !== undefined && (
        <FieldError id={`${id}-phone-error`} message={apply.phoneError} />
      )}
      {view.noteCaption !== null && view.note !== null && (
        <VisitNoteBox caption={view.noteCaption} note={view.note} />
      )}
      {rejecting ? (
        <CheckNoteField
          label={PHONE_REASON_LABEL}
          placeholder={PHONE_REASON_PLACEHOLDER}
          registration={reject.noteField}
          error={reject.noteError}
          disabled={reject.saving}
          hint={view.keepLine}
        />
      ) : (
        <>
          <CheckTodo text={PHONE_TODO} />
          <CheckCallout tone="info" text={PHONE_CALLOUT} />
        </>
      )}
    </CheckSheetFrame>
  );
}
