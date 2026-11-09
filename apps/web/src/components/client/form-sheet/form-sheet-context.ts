"use client";

import { createContext, useContext } from "react";

import type { FormSheetContextValue } from "./type";

// Null outside a FormSheet: forms on pages and in dialogs report to nobody.
export const FormSheetContext = createContext<FormSheetContextValue | null>(
  null,
);

// The FormSheet around a FormSheetContent; one must be there.
export function useFormSheetContext(): FormSheetContextValue {
  const sheet = useContext(FormSheetContext);
  if (sheet === null) {
    throw new Error("FormSheetContent is rendered inside a FormSheet.");
  }
  return sheet;
}
