"use client";

import { ImageIcon } from "lucide-react";
import { useState } from "react";

import type { ChosenPhoto } from "./type";

type ReceiptPhotoThumbProps = {
  // Null while the chosen photo is made ready.
  photo: ChosenPhoto | null;
};

// The chosen photo, small, beside its state. A photo this browser cannot show (a HEIC photo on a
// desktop browser) shows a plain picture icon; the API judges the file itself.
export function ReceiptPhotoThumb({ photo }: ReceiptPhotoThumbProps) {
  // The preview that failed to load; a new photo has another URL, so it is tried again.
  const [failedUrl, setFailedUrl] = useState<string | null>(null);
  const shown = photo !== null && failedUrl !== photo.previewUrl;

  return (
    <span
      aria-hidden="true"
      className="relative flex size-12 shrink-0 items-center justify-center overflow-hidden rounded-lg border bg-muted text-muted-foreground"
    >
      {shown ? (
        // eslint-disable-next-line @next/next/no-img-element -- An object URL of a photo on this device: nothing on a server to optimize, and next/image's optimizer cannot fetch a blob: URL.
        <img
          src={photo.previewUrl}
          alt=""
          onError={() => setFailedUrl(photo.previewUrl)}
          className="absolute inset-0 size-full object-cover"
        />
      ) : (
        <ImageIcon className="size-5" />
      )}
    </span>
  );
}
