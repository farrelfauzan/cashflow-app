"use client";

import { XIcon } from "lucide-react";

import { FormSheet } from "@/components/client/form-sheet/form-sheet";
import { FormSheetContent } from "@/components/client/form-sheet/form-sheet-content";
import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { Button } from "@/components/ui/button";
import {
  SheetClose,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";

import { ProofPhotoPreview } from "./proof-photo-preview";
import { ProofSendProgress } from "./proof-send-progress";
import type { ProofSheetState } from "./type";
import {
  PROOF_CLOSE_LABEL,
  PROOF_OTHER_PHOTO_TEXT,
  PROOF_SHEET_TEXT,
  PROOF_SHEET_TITLE,
  canSendProof,
  proofPrimaryLabel,
} from "./utils";

type ProofSheetProps = {
  state: ProofSheetState;
  onPickPhoto: () => void;
  onSend: () => void;
  onClose: () => void;
};

// The sheet of "Kirim bukti transfer" (board "Kirim bukti transfer", 2): the chosen photo, then
// "Pilih foto lain" and "Kirim foto". While the photo is sent it shows the percent, and no close
// (the X, Escape, a tap outside, a swipe) ends it. A bottom sheet on the phone, a centred dialog
// from md up, over the page's narrow column.
export function ProofSheet({
  state,
  onPickPhoto,
  onSend,
  onClose,
}: ProofSheetProps) {
  const sending = state.kind === "sending";
  const photo =
    state.kind === "ready" || state.kind === "sending" ? state.photo : null;
  const error = state.kind === "ready" ? state.error : null;

  return (
    <FormSheet
      open={state.kind !== "closed"}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      busy={sending}
    >
      <FormSheetContent wide="dialog" showCloseButton={false}>
        <SheetHeader className="flex-row items-start gap-3 border-b px-5 pt-5 pb-4 md:border-b-0 md:px-6 md:pt-6 md:pb-0">
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <SheetTitle className="text-xl leading-tight font-bold md:text-[22px]">
              {PROOF_SHEET_TITLE}
            </SheetTitle>
            <SheetDescription className="text-sm text-muted-foreground md:text-base">
              {PROOF_SHEET_TEXT}
            </SheetDescription>
          </div>
          <SheetClose asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={PROOF_CLOSE_LABEL}
              disabled={sending}
              className="-mt-1.5 -mr-2 rounded-full"
            >
              <XIcon aria-hidden="true" className="size-5" />
            </Button>
          </SheetClose>
        </SheetHeader>
        <div
          data-slot="form-sheet-body"
          className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-5 py-4 md:flex-none md:px-6"
        >
          <ProofPhotoPreview photo={photo} />
          {error !== null && error.kind === "file" && (
            <FieldError id="proof-photo-error" message={error.message} />
          )}
          {error !== null && error.kind !== "file" && (
            <FormAlert message={error.message} />
          )}
          {state.kind === "sending" && (
            <ProofSendProgress percent={state.percent} />
          )}
        </div>
        <div
          data-slot="form-sheet-footer"
          className="flex gap-2.5 border-t px-5 py-4 md:border-t-0 md:px-6 md:pt-2 md:pb-6"
        >
          <Button
            type="button"
            variant="outline"
            disabled={sending || state.kind === "preparing"}
            onClick={onPickPhoto}
            className="h-13 flex-1 rounded-xl border-[1.5px] bg-card px-3 text-base font-semibold md:text-[17px]"
          >
            {PROOF_OTHER_PHOTO_TEXT}
          </Button>
          <Button
            type="button"
            disabled={!canSendProof(state)}
            onClick={onSend}
            className="h-13 flex-[1.4] rounded-xl px-3 text-base font-semibold md:text-[17px]"
          >
            {proofPrimaryLabel(state)}
          </Button>
        </div>
      </FormSheetContent>
    </FormSheet>
  );
}
