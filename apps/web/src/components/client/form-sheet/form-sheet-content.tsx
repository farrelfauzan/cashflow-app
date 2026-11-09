"use client";

import { cn } from "cn";
import type { ComponentProps } from "react";

import { SheetContent } from "@/components/ui/sheet";
import { useFormSheetContext } from "./form-sheet-context";
import { useSheetSwipe } from "./use-sheet-swipe";

type FormSheetContentProps = Omit<
  ComponentProps<typeof SheetContent>,
  | "side"
  | "ref"
  | "onPointerDown"
  | "onPointerMove"
  | "onPointerUp"
  | "onPointerCancel"
>;

// The panel of a FormSheet, laid out by CSS alone (board HP-Lembar). On the phone: a sheet from
// the bottom, 92% of the screen tall, rounded on top, with a grab handle, above the home
// indicator; swiping down on the handle or the header closes it. From md up: the right-hand
// drawer, 470px wide and full height, sliding from the right, exactly as before.
export function FormSheetContent({
  className,
  children,
  ...props
}: FormSheetContentProps) {
  const { contentRef, requestClose } = useFormSheetContext();
  const swipe = useSheetSwipe(requestClose);

  return (
    <SheetContent
      {...props}
      side="bottom"
      ref={contentRef}
      onPointerDown={swipe.onPointerDown}
      onPointerMove={swipe.onPointerMove}
      onPointerUp={swipe.onPointerUp}
      onPointerCancel={swipe.onPointerCancel}
      className={cn(
        "h-[92dvh] w-full gap-0 rounded-t-[20px] bg-card p-0 pb-[env(safe-area-inset-bottom)] md:inset-y-0 md:left-auto md:h-full md:w-[470px] md:rounded-none md:border-t-0 md:border-l md:pb-0 md:data-[state=closed]:[--tw-exit-translate-y:0] md:data-[state=closed]:slide-out-to-right md:data-[state=open]:[--tw-enter-translate-y:0] md:data-[state=open]:slide-in-from-right max-md:[&_[data-slot=sheet-header]]:touch-none max-md:[&_[data-slot=sheet-header]]:select-none",
        className,
      )}
    >
      {/* -top-px: 8px from the sheet's outer edge, across its 1px top border. */}
      <div
        data-slot="form-sheet-handle"
        aria-hidden="true"
        className="absolute -top-px left-1/2 z-10 flex h-6 w-24 -translate-x-1/2 touch-none justify-center pt-2 md:hidden"
      >
        <span className="h-[5px] w-10 rounded-full bg-border" />
      </div>
      {children}
    </SheetContent>
  );
}
