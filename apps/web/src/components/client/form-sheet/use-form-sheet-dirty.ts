"use client";

import { useContext, useEffect, useId } from "react";

import { FormSheetContext } from "./form-sheet-context";

// Tells the nearest FormSheet whether this form has unsaved changes, so closing the sheet on a
// phone asks first. A form that unmounts (a save that shows another panel) reports clean. Does
// nothing outside a FormSheet.
export function useFormSheetDirty(dirty: boolean): void {
  const dirtyForms = useContext(FormSheetContext)?.dirtyForms;
  const formId = useId();
  useEffect(() => {
    if (dirtyForms === undefined) return;
    dirtyForms.report(formId, dirty);
    return () => dirtyForms.report(formId, false);
  }, [dirtyForms, formId, dirty]);
}
