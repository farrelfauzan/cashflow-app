import type {
  DirtyRegistry,
  FormSheetWideLayout,
  SwipeRelease,
  SwipeReleaseAction,
  SwipeSample,
} from "./type";

// The question before a phone's form sheet closes with unsaved changes (board HP-Lembar).
// Provisional copy, pending the user's confirmation.
export const UNSAVED_CHANGES_TITLE = "Tutup tanpa menyimpan?";
export const UNSAVED_CHANGES_BODY =
  "Yang sudah diisi di formulir ini akan hilang.";
export const UNSAVED_CHANGES_KEEP = "Lanjut mengisi";
export const UNSAVED_CHANGES_DISCARD = "Ya, tutup";

// A swipe down closes the sheet when it went past this share of the sheet's height...
export const SWIPE_CLOSE_DISTANCE_RATIO = 0.25;
// ...or when it was released moving down faster than this (px/ms), a flick...
export const SWIPE_CLOSE_VELOCITY = 0.5;
// ...that went at least this far (px), so a tap that jitters is never a flick.
export const SWIPE_MIN_FLICK_DISTANCE = 8;
// The release speed is measured over the last this many ms of the swipe; a finger that stopped
// longer ago than this releases at speed 0.
export const SWIPE_VELOCITY_WINDOW_MS = 100;
// The samples a swipe keeps; enough for the window at 120 Hz.
export const SWIPE_MAX_SAMPLES = 32;

// Where a swipe can start: the grab handle and the header. Never the form's body, so scrolling a
// form never moves the sheet.
const SWIPE_AREA_SELECTOR =
  '[data-slot="form-sheet-handle"], [data-slot="sheet-header"]';
// A press on a control is the control's, not a swipe.
const INTERACTIVE_SELECTOR =
  'a, button, input, select, textarea, label, [role="button"], [contenteditable="true"]';

// The phone's sheet (board HP-Lembar): from the bottom, 92% of the screen tall, rounded on top,
// above the home indicator and clear of a notch on the right; a finger on the header drags it
// instead of panning the page.
const BOTTOM_SHEET_CLASS =
  "h-[92dvh] w-full gap-0 rounded-t-[20px] bg-card p-0 pr-[env(safe-area-inset-right)] pb-[env(safe-area-inset-bottom)] max-md:[&_[data-slot=sheet-header]]:touch-none max-md:[&_[data-slot=sheet-header]]:select-none";

// From md up, over the bottom sheet's classes. "drawer": the right-hand drawer, 470px wide and
// full height, sliding from the right, keeping the bottom and right safe-area padding as the
// drawers had it. "dialog": the centred dialog of components/ui/dialog.tsx
// as the billing dialogs set it (at most 560px wide and the screen less 2rem tall, rounded, its
// whole content scrolling, no safe-area padding since it sits in the middle), zooming and fading in and out over 200ms instead of sliding.
const WIDE_LAYOUT_CLASS: Record<FormSheetWideLayout, string> = {
  drawer:
    "md:inset-y-0 md:left-auto md:h-full md:w-[470px] md:rounded-none md:border-t-0 md:border-l md:data-[state=closed]:[--tw-exit-translate-y:0] md:data-[state=closed]:slide-out-to-right md:data-[state=open]:[--tw-enter-translate-y:0] md:data-[state=open]:slide-in-from-right",
  dialog:
    "md:top-1/2 md:right-auto md:bottom-auto md:left-1/2 md:h-auto md:max-h-[calc(100dvh-2rem)] md:max-w-[560px] md:-translate-x-1/2 md:-translate-y-1/2 md:overflow-y-auto md:rounded-2xl md:border md:pr-0 md:pb-0 md:outline-none md:data-[state=closed]:duration-200 md:data-[state=closed]:ease-[ease] md:data-[state=closed]:[--tw-exit-translate-y:0] md:data-[state=closed]:fade-out-0 md:data-[state=closed]:zoom-out-95 md:data-[state=open]:duration-200 md:data-[state=open]:ease-[ease] md:data-[state=open]:[--tw-enter-translate-y:0] md:data-[state=open]:fade-in-0 md:data-[state=open]:zoom-in-95",
};

// A FormSheetContent's classes: the bottom sheet, then its layout from md up.
export function formSheetContentClass(wide: FormSheetWideLayout): string {
  return `${BOTTOM_SHEET_CLASS} ${WIDE_LAYOUT_CLASS[wide]}`;
}

export const REDUCED_MOTION_QUERY = "(prefers-reduced-motion: reduce)";

// How a sheet released short of closing goes back up.
export const SNAP_BACK_TRANSITION = "transform 200ms ease-out";

export function createDirtyRegistry(): DirtyRegistry {
  const dirty = new Set<string>();
  return {
    report(formId, isDirty) {
      if (isDirty) dirty.add(formId);
      else dirty.delete(formId);
    },
    isDirty: () => dirty.size > 0,
  };
}

// How far down the sheet follows the pointer: never above where it rests.
export function swipeOffset(startY: number, currentY: number): number {
  return Math.max(0, currentY - startY);
}

// The speed downward (px/ms) at release, over the last SWIPE_VELOCITY_WINDOW_MS: from the oldest
// sample in that window to the release point. 0 when the pointer did not move in the window.
export function swipeVelocity(
  samples: readonly SwipeSample[],
  release: SwipeSample,
): number {
  const windowStart = release.time - SWIPE_VELOCITY_WINDOW_MS;
  const first = samples.find(
    (sample) => sample.time >= windowStart && sample.time < release.time,
  );
  if (first === undefined) return 0;
  return (release.y - first.y) / (release.time - first.time);
}

// Whether a released swipe closes the sheet or snaps it back.
export function swipeReleaseAction({
  distance,
  sheetHeight,
  velocity,
}: SwipeRelease): SwipeReleaseAction {
  if (distance <= 0 || sheetHeight <= 0) return "snap";
  if (distance > sheetHeight * SWIPE_CLOSE_DISTANCE_RATIO) return "close";
  if (distance >= SWIPE_MIN_FLICK_DISTANCE && velocity > SWIPE_CLOSE_VELOCITY) {
    return "close";
  }
  return "snap";
}

// Whether a press on `target`, inside `sheet`, starts a swipe: on the handle or the header, not on
// a control, and not in a portal (a dialog opened from the form) that React bubbles through.
export function startsSwipe(sheet: Element, target: EventTarget): boolean {
  if (!(target instanceof Element) || !sheet.contains(target)) return false;
  return (
    target.closest(SWIPE_AREA_SELECTOR) !== null &&
    target.closest(INTERACTIVE_SELECTOR) === null
  );
}
