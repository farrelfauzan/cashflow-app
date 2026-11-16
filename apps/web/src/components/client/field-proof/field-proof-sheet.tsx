"use client";

import { XIcon } from "lucide-react";
import { useEffect, useId } from "react";

import {
  CANCEL_TEXT,
  CLOSE_LABEL,
  PHOTO_ACCEPT,
  PHOTO_OTHER_TEXT,
  sheetSubtitle,
} from "@/components/client/field-cash/utils";
import { FormSheet } from "@/components/client/form-sheet/form-sheet";
import { FormSheetContent } from "@/components/client/form-sheet/form-sheet-content";
import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { FIELD_HINT_CLASS } from "@/components/client/form/utils";
import { ProofPhotoPreview } from "@/components/client/payment-page/proof-photo-preview";
import { ProofSendProgress } from "@/components/client/payment-page/proof-send-progress";
import { Button } from "@/components/ui/button";
import {
  SheetClose,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";

import type { FieldProofTenant } from "./type";
import { useFieldProof } from "./use-field-proof";
import {
  FIELD_PROOF_EXPLAIN,
  FIELD_PROOF_HINT,
  FIELD_PROOF_PICK_TEXT,
  FIELD_PROOF_TITLE,
  canSendFieldProof,
  fieldProofPhoto,
  fieldProofPrimaryLabel,
} from "./utils";

import { FieldProofFailed } from "./field-proof-failed";
import { FieldProofPick } from "./field-proof-pick";

type FieldProofSheetProps = {
  orgId: string;
  tenant: FieldProofTenant;
  // The row's place line ("Kos Melati 11"), repeated in the subtitle.
  placeLabel: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // Whether this opening may have sent the proof without the app knowing (completeUpload's answer
  // was lost): the caller then reopens this same sheet instead of a fresh one, so "Coba lagi"
  // completes the same upload and the admin gets one proof.
  onUnsureChange: (unsure: boolean) => void;
};

// "Bukti transfer dari penyewa" (board HP-Bukti-Karyawan, columns 2 to 4): a bottom sheet on the
// phone, the right-hand drawer from md up, like "Terima tunai". The caller mounts it afresh for
// each opening. A photo is chosen and shown first ("Pilih foto lain" replaces it); "Kirim bukti"
// sends it with its percent, and nothing closes the sheet (the X, Escape, a tap outside, a
// swipe) until it is sent or failed. Once sent, the sheet closes and a toast says so; the row
// does not change.
export function FieldProofSheet({
  orgId,
  tenant,
  placeLabel,
  open,
  onOpenChange,
  onUnsureChange,
}: FieldProofSheetProps) {
  const { state, unsure, inputRef, pickPhoto, choose, send, retry } =
    useFieldProof({
      orgId,
      tenantId: tenant.tenantId,
      onSent: () => onOpenChange(false),
    });
  useEffect(() => {
    onUnsureChange(unsure);
  }, [unsure, onUnsureChange]);
  const id = useId();
  const hintId = `${id}-hint`;
  const errorId = `${id}-error`;
  const sending = state.kind === "sending";
  const photo = fieldProofPhoto(state);
  const error = state.kind === "ready" ? state.error : null;

  return (
    <FormSheet open={open} onOpenChange={onOpenChange} busy={sending}>
      <FormSheetContent showCloseButton={false}>
        <SheetHeader className="flex-row items-start gap-3 border-b px-5 pt-5 pb-3 md:px-6 md:pt-6 md:pb-4">
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <SheetTitle className="text-xl leading-tight font-bold md:text-2xl">
              {FIELD_PROOF_TITLE}
            </SheetTitle>
            <SheetDescription className="text-sm text-muted-foreground md:text-base">
              {sheetSubtitle(tenant.tenantName, placeLabel)}
            </SheetDescription>
          </div>
          <SheetClose asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={CLOSE_LABEL}
              disabled={sending}
              className="-mt-1.5 -mr-2 rounded-full"
            >
              <XIcon aria-hidden="true" className="size-5" />
            </Button>
          </SheetClose>
        </SheetHeader>
        <div
          data-slot="form-sheet-body"
          className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-5 py-4 md:px-6"
        >
          <p className="rounded-xl bg-muted px-3.5 py-3 text-sm leading-normal md:text-base">
            {FIELD_PROOF_EXPLAIN}
          </p>
          {state.kind === "empty" ? (
            <FieldProofPick describedBy={hintId} onPick={pickPhoto} />
          ) : (
            <ProofPhotoPreview photo={photo} />
          )}
          {state.kind === "empty" && (
            <p id={hintId} className={FIELD_HINT_CLASS}>
              {FIELD_PROOF_HINT}
            </p>
          )}
          {state.kind === "sending" && (
            <ProofSendProgress percent={state.percent} />
          )}
          {state.kind === "failed" && (
            <FieldProofFailed message={state.message} onRetry={retry} />
          )}
          {error !== null && error.kind === "file" && (
            <FieldError id={errorId} message={error.message} />
          )}
          {error !== null && error.kind === "denied" && (
            <FormAlert message={error.message} />
          )}
          <input
            ref={inputRef}
            type="file"
            accept={PHOTO_ACCEPT}
            aria-label={FIELD_PROOF_PICK_TEXT}
            tabIndex={-1}
            disabled={sending || unsure}
            className="sr-only"
            onChange={(event) => {
              const file = event.target.files?.[0];
              // Cleared, so choosing the same photo again still reports a change.
              event.target.value = "";
              if (file !== undefined) choose(file);
            }}
          />
        </div>
        <div
          data-slot="form-sheet-footer"
          className="flex gap-2.5 border-t px-4 py-3 md:px-6 md:py-4"
        >
          {photo === null ? (
            <SheetClose asChild>
              <Button
                type="button"
                variant="outline"
                className="h-12 flex-1 rounded-xl border-[1.5px] bg-card text-base font-semibold md:h-13 md:text-[17px]"
              >
                {CANCEL_TEXT}
              </Button>
            </SheetClose>
          ) : (
            <Button
              type="button"
              variant="outline"
              disabled={sending || unsure}
              onClick={pickPhoto}
              className="h-12 flex-1 rounded-xl border-[1.5px] bg-card px-3 text-base font-semibold md:h-13 md:text-[17px]"
            >
              {PHOTO_OTHER_TEXT}
            </Button>
          )}
          <Button
            type="button"
            disabled={!canSendFieldProof(state)}
            onClick={send}
            className="h-12 flex-[1.4] rounded-xl px-3 text-base font-bold md:h-13 md:text-[17px]"
          >
            {fieldProofPrimaryLabel(state)}
          </Button>
        </div>
      </FormSheetContent>
    </FormSheet>
  );
}
