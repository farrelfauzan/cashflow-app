"use client";

import { XIcon } from "lucide-react";
import type { FormEvent, ReactNode } from "react";

import { FormSheet } from "@/components/client/form-sheet/form-sheet";
import { FormSheetContent } from "@/components/client/form-sheet/form-sheet-content";
import { Button } from "@/components/ui/button";
import {
  SheetClose,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";

import { CLOSE_LABEL } from "./utils";

type CheckSheetFrameProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // A request is running: the drawer cannot be closed.
  busy: boolean;
  // A typed note: on the phone, closing asks first.
  dirty: boolean;
  title: string;
  subtitle: string;
  // With a form id, the body is that form (the footer's primary button submits it).
  formId?: string;
  onSubmit?: (event: FormEvent<HTMLFormElement>) => void;
  children: ReactNode;
  footer: ReactNode;
};

// The drawer of one item (the right-hand drawer from md up, the bottom sheet on the phone): its
// title and subtitle with the X, a scrolling body and the footer.
export function CheckSheetFrame({
  open,
  onOpenChange,
  busy,
  dirty,
  title,
  subtitle,
  formId,
  onSubmit,
  children,
  footer,
}: CheckSheetFrameProps) {
  const bodyClass =
    "flex min-h-0 flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-5";
  return (
    <FormSheet
      open={open}
      onOpenChange={onOpenChange}
      dirty={dirty}
      busy={busy}
    >
      <FormSheetContent showCloseButton={false}>
        <SheetHeader className="flex-row items-start gap-3 border-b px-6 pt-6 pb-4">
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <SheetTitle className="text-2xl font-bold">{title}</SheetTitle>
            <SheetDescription className="text-base text-muted-foreground">
              {subtitle}
            </SheetDescription>
          </div>
          <SheetClose asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={CLOSE_LABEL}
              disabled={busy}
              className="-mt-1.5 -mr-2 rounded-full"
            >
              <XIcon aria-hidden="true" className="size-5" />
            </Button>
          </SheetClose>
        </SheetHeader>
        {formId !== undefined ? (
          <form
            id={formId}
            noValidate
            onSubmit={onSubmit}
            data-slot="form-sheet-body"
            className={bodyClass}
          >
            {children}
          </form>
        ) : (
          <div data-slot="form-sheet-body" className={bodyClass}>
            {children}
          </div>
        )}
        <div
          data-slot="form-sheet-footer"
          className="flex gap-2.5 border-t px-6 py-4"
        >
          {footer}
        </div>
      </FormSheetContent>
    </FormSheet>
  );
}
