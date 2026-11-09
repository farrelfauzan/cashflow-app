import type { RefObject } from "react";

// Which forms inside one FormSheet have unsaved changes. Each form reports under its own id, so a
// sheet that holds two forms is dirty while either is.
export type DirtyRegistry = {
  report: (formId: string, dirty: boolean) => void;
  isDirty: () => boolean;
};

// What a FormSheet shares with its content and the forms inside it.
export type FormSheetContextValue = {
  dirtyForms: DirtyRegistry;
  // Every close the person asks for (Escape, a tap outside, the X, Batal, a swipe down) goes
  // through this. It closes the sheet and returns true, or, on a phone with unsaved changes,
  // asks first and returns false.
  requestClose: () => boolean;
  // The sheet's panel, for giving focus back to it after "Lanjut mengisi".
  contentRef: RefObject<HTMLDivElement | null>;
};

// The "Tutup tanpa menyimpan?" question: not shown, shown, or answered with "Ya, tutup" and
// closing without its exit animation so the sheet can close right after it.
export type UnsavedChangesState = "closed" | "asking" | "discarding";

// One point of a swipe: the pointer's clientY and the event's timeStamp (ms).
export type SwipeSample = { y: number; time: number };

// A swipe as it is released: how far down it went (px), the sheet's height (px) and the speed
// downward at the end (px/ms, negative when moving up).
export type SwipeRelease = {
  distance: number;
  sheetHeight: number;
  velocity: number;
};

export type SwipeReleaseAction = "close" | "snap";

// What a FormSheetContent is from md up: the right-hand drawer (the panels) or a centred dialog
// (the short billing forms, Koreksi and Catat janji). Below md both are the same bottom sheet.
export type FormSheetWideLayout = "drawer" | "dialog";
