"use client";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

import type { ChosenPhoto } from "./type";
import { CLOSE_LABEL, PHOTO_PREVIEW_ALT, PHOTO_VIEWER_TITLE } from "./utils";

type ReceiptPhotoViewerProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  photo: ChosenPhoto;
  // "Bu Wati Handayani · Kos Melati 11".
  subtitle: string;
};

// "Ketuk untuk melihat": the photo that was sent, as large as the screen allows, over the sheet.
// It is the copy on this phone, so nothing is fetched. Escape, a tap outside and "Tutup" close it
// and leave the sheet as it was.
export function ReceiptPhotoViewer({
  open,
  onOpenChange,
  photo,
  subtitle,
}: ReceiptPhotoViewerProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="flex h-[calc(100dvh-2rem)] max-w-[calc(100%-2rem)] flex-col gap-4 rounded-2xl p-4 sm:max-w-3xl md:p-6"
      >
        <DialogHeader className="gap-1 text-left">
          <DialogTitle className="text-xl font-bold md:text-2xl">
            {PHOTO_VIEWER_TITLE}
          </DialogTitle>
          <DialogDescription className="text-sm text-muted-foreground md:text-base">
            {subtitle}
          </DialogDescription>
        </DialogHeader>
        {/* eslint-disable-next-line @next/next/no-img-element -- An object URL of a photo on this device: nothing on a server to optimize, and next/image's optimizer cannot fetch a blob: URL. */}
        <img
          src={photo.previewUrl}
          alt={PHOTO_PREVIEW_ALT}
          className="min-h-0 w-full flex-1 rounded-xl bg-muted object-contain"
        />
        <DialogClose asChild>
          <Button
            type="button"
            variant="outline"
            className="h-13 w-full rounded-xl border-[1.5px] bg-card text-base font-semibold md:text-[17px]"
          >
            {CLOSE_LABEL}
          </Button>
        </DialogClose>
      </DialogContent>
    </Dialog>
  );
}
