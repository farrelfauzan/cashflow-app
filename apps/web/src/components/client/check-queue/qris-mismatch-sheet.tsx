"use client";

import Link from "next/link";
import { useId } from "react";

import { FormAlert } from "@/components/client/form/form-alert";
import { Button } from "@/components/ui/button";
import type { CheckQrisAmountMismatch } from "@/lib/api/generated/model";
import { orgPath } from "@/lib/org-path";

import {
  CANCEL_LABEL,
  DISMISS_TOAST,
  NOTE_LABEL,
  QRIS_NOTE_PLACEHOLDER,
  QRIS_SUBMIT,
  QRIS_TITLE,
  QRIS_TODO,
  qrisMismatchSheetView,
} from "./utils";

import { CheckCallout } from "./check-callout";
import { CheckNoteField } from "./check-note-field";
import { CheckSheetFooter } from "./check-sheet-footer";
import { CheckSheetFrame } from "./check-sheet-frame";
import { CheckTiles } from "./check-tiles";
import { CheckTodo } from "./check-todo";
import { TransactionIdRow } from "./transaction-id-row";
import { useDismissForm } from "./use-dismiss-form";

type QrisMismatchSheetProps = {
  orgId: string;
  itemId: string;
  mismatch: CheckQrisAmountMismatch;
  canAct: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

// "QRIS tidak cocok": AutoGoPay reported another amount than the QR's, which the payment was
// recorded at. The admin checks AutoGoPay's dashboard, fixes the money with a correction on the
// tenant's page when it is short, and marks the item checked here.
export function QrisMismatchSheet({
  orgId,
  itemId,
  mismatch,
  canAct,
  open,
  onOpenChange,
}: QrisMismatchSheetProps) {
  const formId = `${useId()}-form`;
  const view = qrisMismatchSheetView(mismatch);
  const dismiss = useDismissForm({
    orgId,
    kind: "qris_amount_mismatch",
    itemId,
    toast: DISMISS_TOAST.qris_amount_mismatch,
    onDone: () => onOpenChange(false),
  });

  return (
    <CheckSheetFrame
      open={open}
      onOpenChange={onOpenChange}
      busy={dismiss.pending}
      dirty={dismiss.form.formState.isDirty}
      title={QRIS_TITLE}
      subtitle={view.subtitle}
      formId={canAct ? formId : undefined}
      onSubmit={dismiss.submit}
      footer={
        <CheckSheetFooter
          readOnly={!canAct}
          secondaryLabel={CANCEL_LABEL}
          primaryLabel={QRIS_SUBMIT}
          formId={formId}
          saving={dismiss.saving}
          pending={dismiss.pending}
        />
      }
    >
      {dismiss.alert !== undefined && <FormAlert message={dismiss.alert} />}
      <CheckTiles tiles={view.tiles} />
      <CheckCallout tone="info" text={view.callout} />
      <TransactionIdRow transactionId={view.transactionId} />
      <CheckTodo text={QRIS_TODO}>
        <Button
          asChild
          variant="outline"
          className="mt-1 self-start font-semibold"
        >
          <Link href={orgPath(orgId, `tenants/${view.tenantId}`)}>
            {view.tenantLinkText}
          </Link>
        </Button>
      </CheckTodo>
      {canAct && (
        <CheckNoteField
          label={NOTE_LABEL}
          placeholder={QRIS_NOTE_PLACEHOLDER}
          registration={dismiss.noteField}
          error={dismiss.noteError}
          disabled={dismiss.saving}
        />
      )}
    </CheckSheetFrame>
  );
}
