"use client";

import { ImageOff, LoaderCircle } from "lucide-react";
import { useState } from "react";

import type { ProofPhoto } from "./type";
import {
  PROOF_NO_PREVIEW_TEXT,
  PROOF_PREPARING_TEXT,
  PROOF_PREVIEW_ALT,
} from "./utils";

type ProofPhotoPreviewProps = {
  // Null while the chosen photo is made ready. Only its object URL is shown, so the karyawan's
  // "Bukti transfer dari penyewa" (field-proof) passes its own photo too.
  photo: Pick<ProofPhoto, "previewUrl"> | null;
};

// The chosen photo, whole, as it will be sent. A file this browser cannot show (a HEIC photo on a
// desktop browser, a file that is not a photo) says so; the API's answer then tells what is wrong
// with it.
export function ProofPhotoPreview({ photo }: ProofPhotoPreviewProps) {
  // The preview that failed to load; a new photo has another URL, so it is tried again.
  const [failedUrl, setFailedUrl] = useState<string | null>(null);
  const failed = photo !== null && failedUrl === photo.previewUrl;

  return (
    <div className="relative flex min-h-56 flex-1 items-center justify-center overflow-hidden rounded-xl bg-muted md:h-[min(52dvh,420px)] md:flex-none">
      {photo === null ? (
        <p className="flex items-center gap-2 text-sm text-muted-foreground md:text-base">
          <LoaderCircle aria-hidden="true" className="size-5 animate-spin" />
          {PROOF_PREPARING_TEXT}
        </p>
      ) : failed ? (
        <p className="flex flex-col items-center gap-2 px-6 text-center text-sm text-muted-foreground md:text-base">
          <ImageOff aria-hidden="true" className="size-8" />
          {PROOF_NO_PREVIEW_TEXT}
        </p>
      ) : (
        // eslint-disable-next-line @next/next/no-img-element -- An object URL of a photo on this device: nothing on a server to optimize, and next/image's optimizer cannot fetch a blob: URL.
        <img
          src={photo.previewUrl}
          alt={PROOF_PREVIEW_ALT}
          onError={() => setFailedUrl(photo.previewUrl)}
          className="absolute inset-0 size-full object-contain"
        />
      )}
    </div>
  );
}
