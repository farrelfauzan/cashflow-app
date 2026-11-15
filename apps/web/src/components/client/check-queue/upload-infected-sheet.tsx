"use client";

import { useId } from "react";

import { FormAlert } from "@/components/client/form/form-alert";
import type { CheckUploadInfected } from "@/lib/api/generated/model";

import {
  CANCEL_LABEL,
  DISMISS_TOAST,
  NOTE_LABEL,
  UPLOAD_CALLOUT,
  UPLOAD_NOTE_PLACEHOLDER,
  UPLOAD_SUBMIT,
  UPLOAD_TITLE,
  uploadInfectedSheetView,
} from "./utils";

import { BlockedFileTile } from "./blocked-file-tile";
import { CheckCallout } from "./check-callout";
import { CheckNoteField } from "./check-note-field";
import { CheckSheetFooter } from "./check-sheet-footer";
import { CheckSheetFrame } from "./check-sheet-frame";
import { CheckTodo } from "./check-todo";
import { useDismissForm } from "./use-dismiss-form";

type UploadInfectedSheetProps = {
  orgId: string;
  itemId: string;
  upload: CheckUploadInfected;
  canAct: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

// "File ditolak": an upload the malware scan refused. The file is never shown; nothing about
// money changed. The admin asks for a new photo and marks the item handled with a note.
export function UploadInfectedSheet({
  orgId,
  itemId,
  upload,
  canAct,
  open,
  onOpenChange,
}: UploadInfectedSheetProps) {
  const formId = `${useId()}-form`;
  const view = uploadInfectedSheetView(upload);
  const dismiss = useDismissForm({
    orgId,
    kind: "upload_infected",
    itemId,
    toast: DISMISS_TOAST.upload_infected,
    onDone: () => onOpenChange(false),
  });

  return (
    <CheckSheetFrame
      open={open}
      onOpenChange={onOpenChange}
      busy={dismiss.pending}
      dirty={dismiss.form.formState.isDirty}
      title={UPLOAD_TITLE}
      subtitle={view.subtitle}
      formId={canAct ? formId : undefined}
      onSubmit={dismiss.submit}
      footer={
        <CheckSheetFooter
          readOnly={!canAct}
          secondaryLabel={CANCEL_LABEL}
          primaryLabel={UPLOAD_SUBMIT}
          formId={formId}
          saving={dismiss.saving}
          pending={dismiss.pending}
        />
      }
    >
      {dismiss.alert !== undefined && <FormAlert message={dismiss.alert} />}
      <BlockedFileTile scannedLine={view.scannedLine} />
      <CheckCallout tone="warning" text={UPLOAD_CALLOUT} />
      <CheckTodo text={view.todo} />
      {canAct && (
        <CheckNoteField
          label={NOTE_LABEL}
          placeholder={UPLOAD_NOTE_PLACEHOLDER}
          registration={dismiss.noteField}
          error={dismiss.noteError}
          disabled={dismiss.saving}
        />
      )}
    </CheckSheetFrame>
  );
}
