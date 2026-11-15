"use client";

import { useId, useState } from "react";
import { useWatch } from "react-hook-form";

import { FormAlert } from "@/components/client/form/form-alert";
import { notify } from "@/components/client/toast/notify";
import { useGetTenantAccount } from "@/lib/api/generated/money/money";
import type { CheckTransferProof } from "@/lib/api/generated/model";
import { jakartaToday } from "@/lib/format/date";

import type { ProofSheetStep } from "./type";
import {
  BACK_LABEL,
  NOTE_LABEL,
  previewAmount,
  PROOF_CALLOUT,
  PROOF_DISMISSED_TOAST,
  PROOF_NOTE_LABEL,
  PROOF_NOTE_PLACEHOLDER,
  PROOF_PENDING_TEXT,
  PROOF_REJECT,
  PROOF_REJECT_HINT,
  PROOF_REJECT_PLACEHOLDER,
  PROOF_REJECT_SUBMIT,
  PROOF_REJECT_TITLE,
  PROOF_SUBMIT,
  PROOF_TITLE,
  proofPayerView,
  proofPhotoAlt,
  proofPhotoCaption,
  tenantLine,
  UPLOAD_REFUSED_TOAST,
} from "./utils";

import { CheckCallout } from "./check-callout";
import { CheckNoteField } from "./check-note-field";
import { CheckSheetFooter } from "./check-sheet-footer";
import { CheckSheetFrame } from "./check-sheet-frame";
import { ProofMatchFields } from "./proof-match-fields";
import { ProofPayer } from "./proof-payer";
import { ProofPhoto } from "./proof-photo";
import { ProofPreview } from "./proof-preview";
import { useDismissForm } from "./use-dismiss-form";
import { useMatchProofForm } from "./use-match-proof-form";
import { useProofPreview } from "./use-proof-preview";
import { useProofUpload } from "./use-proof-upload";
import { useRefreshCheckQueue } from "./use-refresh-check-queue";

type TransferProofSheetProps = {
  orgId: string;
  itemId: string;
  // When the tenant sent it (the item's createdAt).
  createdAt: string;
  proof: CheckTransferProof;
  // Whether the member may answer items (payment:check for the whole organization).
  canAct: boolean;
  // Whether the member may also record payments; matching needs both.
  canMatch: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

// "Bukti transfer" (board Admin-Cek-Bukti): a tenant's photo of a bank transfer, sent from the
// payment page. The admin checks the business account's statement, then types what arrived and
// when; the API previews what it would pay and "Simpan pembayaran" records it as a transfer for the
// proof's own tenant. "Bukan pembayaran" turns the drawer into a note that takes the proof off the
// queue. While the photo is being scanned it is not shown and cannot be matched. A member who may
// only read (the Supervisor) sees the photo and "Tutup".
export function TransferProofSheet({
  orgId,
  itemId,
  createdAt,
  proof,
  canAct,
  canMatch,
  open,
  onOpenChange,
}: TransferProofSheetProps) {
  const id = useId();
  const matchFormId = `${id}-match`;
  const rejectFormId = `${id}-reject`;
  const tenant = proof.tenant;
  const subtitle = tenantLine(tenant);
  const [today] = useState(() => jakartaToday(new Date()));
  const [step, setStep] = useState<ProofSheetStep>("review");
  const close = () => onOpenChange(false);
  const refreshQueue = useRefreshCheckQueue(orgId);

  const upload = useProofUpload({
    orgId,
    proof,
    enabled: open,
    // The row's chip still says "Foto sedang diperiksa": reload the queue.
    onCleaned: () => void refreshQueue(),
    onRefused: () => {
      notify(UPLOAD_REFUSED_TOAST);
      void refreshQueue();
      close();
    },
  });
  const account = useGetTenantAccount(orgId, tenant.id, {
    query: { enabled: open },
  });
  const match = useMatchProofForm({
    orgId,
    itemId,
    tenantId: tenant.id,
    tenantName: tenant.name,
    today,
    onDone: close,
    onNotReady: upload.reload,
  });
  const reject = useDismissForm({
    orgId,
    kind: "transfer_proof",
    itemId,
    toast: PROOF_DISMISSED_TOAST,
    onDone: close,
  });
  const typed = useWatch({ control: match.form.control, name: "amount" });
  const amount = canMatch ? previewAmount(typed) : null;
  const preview = useProofPreview({ orgId, tenantId: tenant.id, amount });

  const rejecting = canAct && step === "reject";
  const pending = match.pending || reject.pending;
  const photo = (
    <ProofPhoto
      photo={upload.photo}
      caption={proofPhotoCaption(
        createdAt,
        proof.invoiceId,
        account.data?.invoices,
      )}
      alt={proofPhotoAlt(tenant)}
      subtitle={subtitle}
      onReload={upload.reload}
    />
  );
  const pendingCallout = upload.photo.kind === "pending" && (
    <CheckCallout tone="info" text={PROOF_PENDING_TEXT} />
  );

  let body;
  let footer;
  if (rejecting) {
    body = (
      <>
        {reject.alert !== undefined && <FormAlert message={reject.alert} />}
        {photo}
        {pendingCallout}
        <CheckNoteField
          label={NOTE_LABEL}
          placeholder={PROOF_REJECT_PLACEHOLDER}
          registration={reject.noteField}
          error={reject.noteError}
          disabled={reject.saving}
          hint={PROOF_REJECT_HINT}
        />
      </>
    );
    footer = (
      <CheckSheetFooter
        readOnly={false}
        secondaryLabel={BACK_LABEL}
        onSecondary={() => setStep("review")}
        primaryLabel={PROOF_REJECT_SUBMIT}
        formId={rejectFormId}
        saving={reject.saving}
        pending={reject.pending}
        balanced
      />
    );
  } else if (canMatch) {
    body = (
      <>
        {match.alert !== undefined && <FormAlert message={match.alert} />}
        {photo}
        {pendingCallout}
        <CheckCallout tone="warning" text={PROOF_CALLOUT} />
        <ProofPayer payer={proofPayerView(tenant, account.data?.outstanding)} />
        <ProofMatchFields
          form={match.form}
          today={today}
          disabled={match.saving}
        />
        <ProofPreview
          view={preview.view}
          loading={preview.loading}
          error={preview.error}
          typed={amount !== null}
        />
        <CheckNoteField
          label={PROOF_NOTE_LABEL}
          placeholder={PROOF_NOTE_PLACEHOLDER}
          registration={match.noteField}
          error={match.noteError}
          disabled={match.saving}
        />
      </>
    );
    footer = (
      <CheckSheetFooter
        readOnly={false}
        secondaryLabel={PROOF_REJECT}
        onSecondary={() => setStep("reject")}
        primaryLabel={PROOF_SUBMIT}
        formId={matchFormId}
        saving={match.saving}
        pending={match.pending}
        primaryDisabled={!upload.ready}
        balanced
      />
    );
  } else {
    body = (
      <>
        {photo}
        {pendingCallout}
      </>
    );
    footer = (
      <CheckSheetFooter
        readOnly
        secondaryLabel={PROOF_REJECT}
        primaryLabel={PROOF_SUBMIT}
        saving={false}
        pending={false}
      />
    );
  }

  let formId: string | undefined;
  let onSubmit;
  if (rejecting) {
    formId = rejectFormId;
    onSubmit = reject.submit;
  } else if (canMatch) {
    formId = matchFormId;
    onSubmit = match.submit;
  }

  return (
    <CheckSheetFrame
      open={open}
      onOpenChange={onOpenChange}
      busy={pending}
      dirty={match.form.formState.isDirty || reject.form.formState.isDirty}
      title={rejecting ? PROOF_REJECT_TITLE : PROOF_TITLE}
      subtitle={subtitle}
      formId={formId}
      onSubmit={onSubmit}
      footer={footer}
    >
      {body}
    </CheckSheetFrame>
  );
}
