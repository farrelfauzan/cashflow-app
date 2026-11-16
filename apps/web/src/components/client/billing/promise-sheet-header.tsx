"use client";

import { XIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  SheetClose,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";

type PromiseSheetHeaderProps = {
  // "Catat janji" or "Catat janji bayar".
  title: string;
  subtitle: string;
  // "Janji aktif Sab, 12 Des, Rp1.250.000 akan diganti."; null for none.
  replacesLine: string | null;
  // The header's classes for the dialog or the drawer (PROMISE_SHEET_CLASSES).
  className: string;
  // While the promise is saved the sheet cannot be closed.
  closeDisabled: boolean;
};

// The top of a promise form: its title, who it is for, what saving replaces, and the close button.
// Shown on its own while the field app's sheet loads what a promise may name.
export function PromiseSheetHeader({
  title,
  subtitle,
  replacesLine,
  className,
  closeDisabled,
}: PromiseSheetHeaderProps) {
  return (
    <SheetHeader className={className}>
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <SheetTitle className="text-xl leading-tight font-bold md:text-[22px]">
          {title}
        </SheetTitle>
        <SheetDescription className="text-sm text-muted-foreground md:text-base">
          {subtitle}
        </SheetDescription>
        {replacesLine !== null && (
          <p className="text-sm font-semibold text-warning md:text-base">
            {replacesLine}
          </p>
        )}
      </div>
      <SheetClose asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label="Tutup"
          disabled={closeDisabled}
          className="-mt-1.5 -mr-2 rounded-full"
        >
          <XIcon aria-hidden="true" className="size-5" />
        </Button>
      </SheetClose>
    </SheetHeader>
  );
}
