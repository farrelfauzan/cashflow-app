"use client";

import { ChevronRight } from "lucide-react";
import Link from "next/link";
import { useId } from "react";

import { FormAlert } from "@/components/client/form/form-alert";
import type { CheckDepositGap } from "@/lib/api/generated/model";
import { orgPath } from "@/lib/org-path";

import {
  CANCEL_LABEL,
  DEPOSIT_GAP_CALLOUT,
  DEPOSIT_GAP_NOTE_LABEL,
  DEPOSIT_GAP_NOTE_PLACEHOLDER,
  DEPOSIT_GAP_SUBMIT,
  DEPOSIT_GAP_TITLE,
  DEPOSIT_LINK_TEXT,
  DISMISS_TOAST,
  depositGapSheetView,
} from "./utils";

import { CheckCallout } from "./check-callout";
import { CheckNoteField } from "./check-note-field";
import { CheckSheetFooter } from "./check-sheet-footer";
import { CheckSheetFrame } from "./check-sheet-frame";
import { CheckTiles } from "./check-tiles";
import { CheckTodo } from "./check-todo";
import { useDismissForm } from "./use-dismiss-form";

type DepositGapSheetProps = {
  orgId: string;
  itemId: string;
  gap: CheckDepositGap;
  // Whether the member may close it; otherwise the drawer is read-only.
  canAct: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

// "Selisih setoran": a counted deposit that differs from its receipts. The tenants' invoices do
// not change; the admin settles the gap with the karyawan and closes it with a note, after which
// the deposit shows "Selisih ditutup".
export function DepositGapSheet({
  orgId,
  itemId,
  gap,
  canAct,
  open,
  onOpenChange,
}: DepositGapSheetProps) {
  const formId = `${useId()}-form`;
  const view = depositGapSheetView(gap);
  const close = () => onOpenChange(false);
  const dismiss = useDismissForm({
    orgId,
    kind: "deposit_gap",
    itemId,
    toast: DISMISS_TOAST.deposit_gap,
    refreshScope: { deposits: true },
    onDone: close,
  });

  return (
    <CheckSheetFrame
      open={open}
      onOpenChange={onOpenChange}
      busy={dismiss.pending}
      dirty={dismiss.form.formState.isDirty}
      title={DEPOSIT_GAP_TITLE}
      subtitle={view.subtitle}
      formId={canAct ? formId : undefined}
      onSubmit={dismiss.submit}
      footer={
        <CheckSheetFooter
          readOnly={!canAct}
          secondaryLabel={CANCEL_LABEL}
          primaryLabel={DEPOSIT_GAP_SUBMIT}
          formId={formId}
          saving={dismiss.saving}
          pending={dismiss.pending}
        />
      }
    >
      {dismiss.alert !== undefined && <FormAlert message={dismiss.alert} />}
      <CheckTiles tiles={view.tiles} />
      <CheckCallout tone="info" text={DEPOSIT_GAP_CALLOUT} />
      <CheckTodo text={view.todo}>
        <Link
          href={orgPath(orgId, "billing/deposits?tab=confirmed")}
          className="inline-flex min-h-11 items-center gap-1 self-start rounded-md text-base font-semibold text-primary hover:underline focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
        >
          {DEPOSIT_LINK_TEXT}
          <ChevronRight aria-hidden="true" className="size-4" />
        </Link>
      </CheckTodo>
      {canAct && (
        <CheckNoteField
          label={DEPOSIT_GAP_NOTE_LABEL}
          placeholder={DEPOSIT_GAP_NOTE_PLACEHOLDER}
          registration={dismiss.noteField}
          error={dismiss.noteError}
          disabled={dismiss.saving}
        />
      )}
    </CheckSheetFrame>
  );
}
