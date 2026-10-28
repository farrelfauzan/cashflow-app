"use client";

import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";

type DisableMemberDialogProps = {
  memberName: string;
  // The organization's name, or "usaha ini".
  place: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pending: boolean;
  onConfirm: () => void;
};

// "Nonaktifkan anggota" and its confirmation. The dialog stays open while the request runs, so
// a failure is never hidden behind a closed dialog.
export function DisableMemberDialog({
  memberName,
  place,
  open,
  onOpenChange,
  pending,
  onConfirm,
}: DisableMemberDialogProps) {
  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!pending) onOpenChange(next);
      }}
    >
      <AlertDialogTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className="rounded-[10px] border-[1.5px] border-destructive bg-card px-4 text-[15px] font-semibold text-destructive hover:bg-destructive/10 hover:text-destructive"
        >
          Nonaktifkan anggota
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent className="bg-card">
        <AlertDialogHeader>
          <AlertDialogTitle className="text-xl font-bold">
            Nonaktifkan {memberName}?
          </AlertDialogTitle>
          <AlertDialogDescription className="text-base leading-normal">
            {memberName} tidak bisa masuk lagi ke {place} sampai diaktifkan
            lagi. Catatan kerjanya tetap tersimpan.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel
            disabled={pending}
            className="rounded-xl text-base font-semibold"
          >
            Batal
          </AlertDialogCancel>
          <Button
            type="button"
            variant="destructive"
            onClick={onConfirm}
            disabled={pending}
            className="rounded-xl text-base font-semibold"
          >
            {pending ? "Menonaktifkan…" : "Ya, nonaktifkan"}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
