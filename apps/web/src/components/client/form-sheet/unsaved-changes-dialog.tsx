"use client";

import { cn } from "cn";

import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import {
  UNSAVED_CHANGES_BODY,
  UNSAVED_CHANGES_DISCARD,
  UNSAVED_CHANGES_KEEP,
  UNSAVED_CHANGES_TITLE,
} from "./utils";

type UnsavedChangesDialogProps = {
  open: boolean;
  // "Ya, tutup" was chosen: the dialog closes without its exit animation, so the sheet can close
  // right after it.
  discarding: boolean;
  // "Lanjut mengisi", Escape or a tap outside the dialog.
  onKeepEditing: () => void;
  onDiscard: () => void;
  // Once the dialog is gone: the FormSheet puts focus back in the form, or closes the sheet.
  onCloseAutoFocus: (event: Event) => void;
};

// "Tutup tanpa menyimpan?", asked on a phone before a form sheet with unsaved changes closes.
// Focus starts on the safe button.
export function UnsavedChangesDialog({
  open,
  discarding,
  onKeepEditing,
  onDiscard,
  onCloseAutoFocus,
}: UnsavedChangesDialogProps) {
  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onKeepEditing();
      }}
    >
      <AlertDialogContent
        onCloseAutoFocus={onCloseAutoFocus}
        className={cn(
          "bg-card",
          discarding && "data-[state=closed]:animate-none",
        )}
      >
        <AlertDialogHeader>
          <AlertDialogTitle className="text-xl font-bold">
            {UNSAVED_CHANGES_TITLE}
          </AlertDialogTitle>
          <AlertDialogDescription className="text-base leading-normal">
            {UNSAVED_CHANGES_BODY}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="rounded-xl text-base font-semibold">
            {UNSAVED_CHANGES_KEEP}
          </AlertDialogCancel>
          <Button
            type="button"
            variant="destructive"
            onClick={onDiscard}
            className="rounded-xl text-base font-semibold"
          >
            {UNSAVED_CHANGES_DISCARD}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
