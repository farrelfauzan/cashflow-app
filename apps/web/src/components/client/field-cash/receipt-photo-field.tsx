"use client";

import { useId, useRef, useState } from "react";

import { FieldError } from "@/components/client/form/field-error";
import {
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
} from "@/components/client/form/utils";

import type { ReceiptPhotoUpload } from "./type";
import {
  PHOTO_ACCEPT,
  PHOTO_HINT,
  PHOTO_LABEL,
  PHOTO_PICK_TEXT,
  PHOTO_REQUIRED_TEXT,
  photoAnnouncement,
} from "./utils";

import { ReceiptPhotoFailed } from "./receipt-photo-failed";
import { ReceiptPhotoPick } from "./receipt-photo-pick";
import { ReceiptPhotoRefused } from "./receipt-photo-refused";
import { ReceiptPhotoSent } from "./receipt-photo-sent";
import { ReceiptPhotoUploading } from "./receipt-photo-uploading";
import { ReceiptPhotoViewer } from "./receipt-photo-viewer";

type ReceiptPhotoFieldProps = {
  photo: ReceiptPhotoUpload;
  // The sheet's subtitle, repeated over the photo when it is shown whole.
  subtitle: string;
  // A save got no answer: the photo that went with it stays.
  locked: boolean;
  saving: boolean;
};

// "Foto kwitansi (wajib)" in its four states (board HP-Foto): no photo, sending with its percent,
// not sent ("Coba lagi" or another photo), and sent. A photo the API refused shows why under it.
// One file input serves every state; it is named "Ambil foto kwitansi" and takes any image with
// no `capture`, so the phone offers both its camera and its gallery. Changes of state are
// announced politely, without reading out every percent.
export function ReceiptPhotoField({
  photo,
  subtitle,
  locked,
  saving,
}: ReceiptPhotoFieldProps) {
  const id = useId();
  const labelId = `${id}-label`;
  const hintId = `${id}-hint`;
  const errorId = `${id}-error`;
  const inputRef = useRef<HTMLInputElement>(null);
  const [viewing, setViewing] = useState(false);
  const { state } = photo;
  const busy = saving || locked;

  function pick() {
    if (busy) return;
    inputRef.current?.click();
  }

  return (
    <div role="group" aria-labelledby={labelId} className="flex flex-col gap-2">
      <p id={labelId} className={FIELD_LABEL_CLASS}>
        {PHOTO_LABEL}{" "}
        <span className="font-normal text-muted-foreground">
          {PHOTO_REQUIRED_TEXT}
        </span>
      </p>

      {state.kind === "empty" && (
        <ReceiptPhotoPick describedBy={hintId} onPick={pick} />
      )}
      {state.kind === "uploading" && (
        <ReceiptPhotoUploading photo={state.photo} percent={state.percent} />
      )}
      {state.kind === "failed" && (
        <ReceiptPhotoFailed
          photo={state.photo}
          message={state.message}
          onRetry={photo.retry}
          onPickOther={pick}
        />
      )}
      {state.kind === "refused" && (
        <ReceiptPhotoRefused
          photo={state.photo}
          errorId={errorId}
          onPickOther={pick}
        />
      )}
      {state.kind === "sent" && (
        <ReceiptPhotoSent
          photo={state.photo}
          canReplace={!locked}
          disabled={saving}
          onView={() => setViewing(true)}
          onReplace={pick}
        />
      )}

      {state.kind === "empty" && (
        <p id={hintId} className={FIELD_HINT_CLASS}>
          {PHOTO_HINT}
        </p>
      )}
      {state.kind === "refused" && (
        <FieldError id={errorId} message={state.message} />
      )}
      <p role="status" aria-live="polite" className="sr-only">
        {photoAnnouncement(state)}
      </p>

      <input
        ref={inputRef}
        type="file"
        accept={PHOTO_ACCEPT}
        aria-label={PHOTO_PICK_TEXT}
        tabIndex={-1}
        disabled={busy}
        className="sr-only"
        onChange={(event) => {
          const file = event.target.files?.[0];
          // Cleared, so choosing the same photo again still reports a change.
          event.target.value = "";
          if (file !== undefined) photo.choose(file);
        }}
      />
      {state.kind === "sent" && (
        <ReceiptPhotoViewer
          open={viewing}
          onOpenChange={setViewing}
          photo={state.photo}
          subtitle={subtitle}
        />
      )}
    </div>
  );
}
