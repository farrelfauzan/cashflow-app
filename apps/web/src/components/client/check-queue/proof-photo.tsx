"use client";

import { FileX, Hourglass, Maximize2 } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

import type { ProofPhotoCaption, ProofPhotoState } from "./type";
import {
  PHOTO_ERROR_TEXT,
  PHOTO_LOADING_TEXT,
  PROOF_LINK_LABEL,
  PROOF_SENT_LABEL,
  ZOOM_LABEL,
} from "./utils";

import { ProofPhotoDialog } from "./proof-photo-dialog";

type ProofPhotoProps = {
  photo: ProofPhotoState;
  caption: ProofPhotoCaption;
  alt: string;
  // The drawer's subtitle, repeated in the large view.
  subtitle: string;
  // Asks getUpload for a new link, after the image failed to load (most likely an expired link).
  onReload: () => void;
  // "Perbesar foto" is open. Held by the drawer, which ignores every close while it is, so an
  // Escape meant for the large view never closes the drawer behind it.
  zoomed: boolean;
  onZoomedChange: (zoomed: boolean) => void;
};

// The proof's photo beside when the tenant sent it and from which invoice's link, with "Perbesar
// foto" once the photo is clean. While the photo is being scanned, a placeholder stands in its
// place.
export function ProofPhoto({
  photo,
  caption,
  alt,
  subtitle,
  onReload,
  zoomed,
  onZoomedChange,
}: ProofPhotoProps) {
  // How many times the image failed to load: the first asks for a new link, the second gives up.
  const [failures, setFailures] = useState(0);
  const url = photo.kind === "clean" && failures < 2 ? photo.url : null;
  const message =
    photo.kind === "error"
      ? photo.message
      : photo.kind === "clean" && failures >= 2
        ? PHOTO_ERROR_TEXT
        : null;

  function failed() {
    if (failures === 0) onReload();
    setFailures((count) => count + 1);
  }

  const frame =
    "flex h-40 w-30 shrink-0 items-center justify-center overflow-hidden rounded-xl border bg-muted md:h-50 md:w-[150px]";
  let thumbnail;
  if (url !== null) {
    thumbnail = (
      <div className={frame}>
        {/* eslint-disable-next-line @next/next/no-img-element -- A presigned link to the bucket that lives a few minutes: next/image would need the bucket's host in its config and would fetch and cache the file on the server under a link that soon stops working. */}
        <img
          src={url}
          alt={alt}
          referrerPolicy="no-referrer"
          onError={failed}
          className="size-full object-cover object-top"
        />
      </div>
    );
  } else {
    const Icon = message !== null ? FileX : Hourglass;
    thumbnail = (
      <div
        aria-hidden="true"
        className={cn(
          frame,
          "text-muted-foreground",
          photo.kind === "loading" && "animate-pulse",
        )}
      >
        <Icon className="size-7" />
      </div>
    );
  }

  return (
    <div className="flex items-stretch gap-3.5">
      {thumbnail}
      <div className="flex min-w-0 flex-1 flex-col justify-between gap-2">
        <dl className="flex flex-col gap-1">
          <dt className="text-sm text-muted-foreground md:text-base">
            {PROOF_SENT_LABEL}
          </dt>
          <dd className="text-base font-semibold whitespace-nowrap md:text-[17px]">
            {caption.sentAt}
          </dd>
          {caption.link !== null && (
            <>
              <dt className="mt-1.5 text-sm text-muted-foreground md:text-base">
                {PROOF_LINK_LABEL}
              </dt>
              <dd className="text-base font-semibold md:text-[17px]">
                {caption.link}
              </dd>
            </>
          )}
        </dl>
        {photo.kind === "loading" && (
          <p className="text-sm text-muted-foreground md:text-base">
            {PHOTO_LOADING_TEXT}
          </p>
        )}
        {message !== null && (
          <p className="text-sm text-muted-foreground md:text-base">
            {message}
          </p>
        )}
        {url !== null && (
          <Button
            type="button"
            variant="ghost"
            onClick={() => onZoomedChange(true)}
            className="-ml-1 h-11 justify-start gap-1.5 self-start px-1 text-base font-semibold text-primary hover:bg-accent hover:text-primary has-[>svg]:px-1 md:text-[17px]"
          >
            <Maximize2 aria-hidden="true" className="size-[18px]" />
            {ZOOM_LABEL}
          </Button>
        )}
      </div>
      {url !== null && (
        <ProofPhotoDialog
          open={zoomed}
          onOpenChange={onZoomedChange}
          url={url}
          alt={alt}
          subtitle={subtitle}
        />
      )}
    </div>
  );
}
