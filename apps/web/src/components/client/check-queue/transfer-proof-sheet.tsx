"use client";

import { useId, useState } from "react";
import { useWatch } from "react-hook-form";

import { FormAlert } from "@/components/client/form/form-alert";
import { notify } from "@/components/client/toast/notify";
import { useGetTenantAccount } from "@/lib/api/generated/money/money";
import type { CheckTransferProof } from "@/lib/api/generated/model";
import { jakartaToday } from "@/lib/format/date";

import type { ProofPayee, ProofSheetStep } from "./type";
import {
  BACK_LABEL,
  CLOSE_LABEL,
  NOTE_LABEL,
  otherPayeeNotice,
  PAYEE_TITLE,
  PAYEE_USE,
  payeeSubtitle,
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
  senderPayee,
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
import { ProofPayeePicker } from "./proof-payee-picker";
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
  // Whether the member may also search tenants, which "Ganti" (another tenant's payment) needs.
  canChangePayer: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

// "Bukti transfer" (board Admin-Cek-Bukti): a tenant's photo of a bank transfer, sent from the
// payment page. The admin checks the business account's statement, then types what arrived and
// when; the API previews what it would pay and "Simpan pembayaran" records it as a transfer for the
// proof's own tenant, or for another tenant chosen with "Ganti" (board Admin-Cek-Bukti-Ganti: a
// relative paid for them), which turns the drawer into "Pembayaran untuk siapa?" and back; the
// typed amount, day and note stay. "Bukan pembayaran" turns the drawer into a note that takes the proof off the
// queue. While the photo is being scanned it is not shown and cannot be matched. A member who may
// answer items but not record payments gets only "Bukan pembayaran"; one who may only read (the
// Supervisor) sees the photo and "Tutup".
export function TransferProofSheet({
  orgId,
  itemId,
  createdAt,
  proof,
  canAct,
  canMatch,
  canChangePayer,
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
  const [sender] = useState(() => senderPayee(tenant));
  // Whose payment it becomes, and the choice being made in the picker.
  const [payee, setPayee] = useState<ProofPayee>(sender);
  const [draft, setDraft] = useState<ProofPayee>(sender);
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
  // The sender's account names the invoice of the link the proof came from; the payee's says what
  // they still owe (the same read while the payee is the sender).
  const account = useGetTenantAccount(orgId, tenant.id, {
    query: { enabled: open },
  });
  const payeeAccount = useGetTenantAccount(orgId, payee.id, {
    query: { enabled: open },
  });
  const match = useMatchProofForm({
    orgId,
    itemId,
    proofTenantId: tenant.id,
    payeeId: payee.id,
    payeeName: payee.name,
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
  const preview = useProofPreview({ orgId, tenantId: payee.id, amount });

  const rejecting = canAct && step === "reject";
  const choosing = canMatch && canChangePayer && step === "payee";
  // "Ganti" goes once a save may have recorded the payment for this payee, and while one runs.
  const mayChange = canChangePayer && !match.payeeLocked && !match.saving;
  function openPicker() {
    setDraft(payee);
    setStep("payee");
  }
  const pending = match.pending || reject.pending;
  // Set by the press on "Perbesar foto", before the large view has registered as the top layer:
  // an Escape in that moment would otherwise reach the drawer and close it instead. It counts
  // only while the photo is clean: the large view goes with the photo, and must not leave the
  // drawer refusing every close.
  const [zoomed, setZoomed] = useState(false);
  const photo = (
    <ProofPhoto
      photo={upload.photo}
      caption={proofPhotoCaption(
        createdAt,
        proof.invoiceId,
        account.data?.invoices,
        proof.sentBy,
      )}
      alt={proofPhotoAlt(tenant)}
      subtitle={subtitle}
      onReload={upload.reload}
      zoomed={zoomed}
      onZoomedChange={setZoomed}
    />
  );
  const pendingCallout = upload.photo.kind === "pending" && (
    <CheckCallout tone="info" text={PROOF_PENDING_TEXT} />
  );

  let body;
  let footer;
  if (choosing) {
    body = (
      <ProofPayeePicker
        orgId={orgId}
        sender={sender}
        selected={draft}
        onSelect={setDraft}
        initialSearch={payee.id === sender.id ? "" : payee.name}
      />
    );
    footer = (
      <CheckSheetFooter
        readOnly={false}
        secondaryLabel={BACK_LABEL}
        onSecondary={() => setStep("review")}
        primaryLabel={PAYEE_USE}
        onPrimary={() => {
          setPayee(draft);
          setStep("review");
        }}
        saving={false}
        pending={false}
      />
    );
  } else if (rejecting) {
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
        <ProofPayer
          payer={proofPayerView(payee, payeeAccount.data?.outstanding)}
          notice={otherPayeeNotice(tenant, payee.id)}
          onChange={mayChange ? openPicker : undefined}
        />
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
  } else if (canAct) {
    // May answer items but not record payments (a custom role): the proof can only be taken off
    // the queue as not a payment, which needs payment:check alone.
    body = (
      <>
        {photo}
        {pendingCallout}
      </>
    );
    footer = (
      <CheckSheetFooter
        readOnly={false}
        secondaryLabel={CLOSE_LABEL}
        primaryLabel={PROOF_REJECT}
        onPrimary={() => setStep("reject")}
        saving={false}
        pending={false}
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
  } else if (canMatch && !choosing) {
    // The picker is no form: Enter in "Cari penyewa" must not save the payment.
    formId = matchFormId;
    onSubmit = match.submit;
  }

  return (
    <CheckSheetFrame
      open={open}
      onOpenChange={onOpenChange}
      busy={pending || (zoomed && upload.photo.kind === "clean")}
      dirty={
        match.form.formState.isDirty ||
        reject.form.formState.isDirty ||
        payee.id !== sender.id
      }
      title={
        choosing ? PAYEE_TITLE : rejecting ? PROOF_REJECT_TITLE : PROOF_TITLE
      }
      subtitle={choosing ? payeeSubtitle(tenant) : subtitle}
      formId={formId}
      onSubmit={onSubmit}
      footer={footer}
    >
      {body}
    </CheckSheetFrame>
  );
}
