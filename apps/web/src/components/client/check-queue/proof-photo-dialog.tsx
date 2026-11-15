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

import { CLOSE_LABEL, ZOOM_TITLE } from "./utils";

type ProofPhotoDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // The short-lived download link, never kept beyond getUpload's answer.
  url: string;
  alt: string;
  // "Bu Wati Handayani · Kamar 11, Kos Melati".
  subtitle: string;
};

// "Perbesar foto": the proof as large as the screen allows, over the drawer. Escape, a tap outside
// and "Tutup" close it and leave the drawer as it was.
export function ProofPhotoDialog({
  open,
  onOpenChange,
  url,
  alt,
  subtitle,
}: ProofPhotoDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="flex h-[calc(100dvh-2rem)] max-w-[calc(100%-2rem)] flex-col gap-4 rounded-2xl p-4 sm:max-w-3xl md:p-6"
      >
        <DialogHeader className="gap-1 text-left">
          <DialogTitle className="text-xl font-bold md:text-2xl">
            {ZOOM_TITLE}
          </DialogTitle>
          <DialogDescription className="text-sm text-muted-foreground md:text-base">
            {subtitle}
          </DialogDescription>
        </DialogHeader>
        {/* eslint-disable-next-line @next/next/no-img-element -- A presigned link to the bucket that lives a few minutes: next/image would need the bucket's host in its config and would fetch and cache the file on the server under a link that soon stops working. */}
        <img
          src={url}
          alt={alt}
          referrerPolicy="no-referrer"
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
