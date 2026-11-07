"use client";

import { XIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";

import { TelegramDialogBody } from "./telegram-dialog-body";
import {
  STAFF_TELEGRAM_LINKED_TEXT,
  STAFF_TELEGRAM_TEXT,
  STAFF_TELEGRAM_TITLE,
} from "./utils";

type TelegramDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // From getMyTelegram, read again on focus and after each change.
  linked: boolean;
};

// The member's Telegram link. The body mounts with the dialog, so a deep link asked for earlier
// is gone once the dialog closes.
export function TelegramDialog({
  open,
  onOpenChange,
  linked,
}: TelegramDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="flex max-h-[calc(100dvh-2rem)] max-w-[calc(100%-2rem)] flex-col gap-5 overflow-y-auto rounded-2xl bg-card p-5 sm:max-w-[480px] md:p-6"
      >
        <div className="flex items-start gap-3">
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <DialogTitle className="text-xl leading-tight font-bold md:text-[22px]">
              {STAFF_TELEGRAM_TITLE}
            </DialogTitle>
            <DialogDescription className="text-sm text-muted-foreground md:text-base">
              {linked ? STAFF_TELEGRAM_LINKED_TEXT : STAFF_TELEGRAM_TEXT}
            </DialogDescription>
          </div>
          <DialogClose asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label="Tutup"
              className="-mt-1.5 -mr-2 size-11 rounded-full"
            >
              <XIcon aria-hidden="true" className="size-5" />
            </Button>
          </DialogClose>
        </div>
        <TelegramDialogBody linked={linked} />
      </DialogContent>
    </Dialog>
  );
}
