"use client";

import { useCallback, useMemo, useRef, useState, type ReactNode } from "react";

import { WIDE_LAYOUT_QUERY } from "@/components/client/form/utils";
import { Sheet } from "@/components/ui/sheet";
import { FormSheetContext } from "./form-sheet-context";
import type { FormSheetContextValue, UnsavedChangesState } from "./type";
import { createDirtyRegistry } from "./utils";

import { UnsavedChangesDialog } from "./unsaved-changes-dialog";

type FormSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  children: ReactNode;
};

// The root of a create or edit form's sheet, in place of Sheet: a bottom sheet on the phone, the
// right-hand drawer from md up (FormSheetContent). Every close the person asks for (Escape, a tap
// outside, the X, Batal, a swipe down) comes through here; on the phone, while a form inside
// reports unsaved changes (useApiForm does), it asks "Tutup tanpa menyimpan?" first. iPad and
// desktop close at once, as before. A panel that closes itself after a save sets `open` to false
// and is never asked.
export function FormSheet({ open, onOpenChange, children }: FormSheetProps) {
  const [dirtyForms] = useState(createDirtyRegistry);
  const contentRef = useRef<HTMLDivElement | null>(null);
  // Where focus was when the question opened, for "Lanjut mengisi".
  const keepFocusRef = useRef<HTMLElement | null>(null);
  const [question, setQuestion] = useState<UnsavedChangesState>("closed");
  if (!open && question === "asking") setQuestion("closed");

  const requestClose = useCallback((): boolean => {
    // Phone or not is decided now, not while rendering, so the server and the first client
    // render agree.
    if (!dirtyForms.isDirty() || window.matchMedia(WIDE_LAYOUT_QUERY).matches) {
      onOpenChange(false);
      return true;
    }
    const active = document.activeElement;
    keepFocusRef.current =
      active instanceof HTMLElement && contentRef.current?.contains(active)
        ? active
        : null;
    setQuestion("asking");
    return false;
  }, [dirtyForms, onOpenChange]);

  const context = useMemo<FormSheetContextValue>(
    () => ({ dirtyForms, requestClose, contentRef }),
    [dirtyForms, requestClose],
  );

  // Runs once the question has gone (Radix calls it after the dialog unmounts and releases
  // focus), so the sheet's own focus handling never races the dialog's.
  function afterQuestion(event: Event) {
    event.preventDefault();
    if (question === "discarding") {
      setQuestion("closed");
      onOpenChange(false);
      return;
    }
    const keep = keepFocusRef.current;
    keepFocusRef.current = null;
    (keep?.isConnected === true ? keep : contentRef.current)?.focus();
  }

  return (
    <FormSheetContext value={context}>
      <Sheet
        open={open}
        onOpenChange={(next) => {
          if (next) onOpenChange(true);
          else requestClose();
        }}
      >
        {children}
      </Sheet>
      <UnsavedChangesDialog
        open={question === "asking"}
        discarding={question === "discarding"}
        onKeepEditing={() => setQuestion("closed")}
        onDiscard={() => setQuestion("discarding")}
        onCloseAutoFocus={afterQuestion}
      />
    </FormSheetContext>
  );
}
