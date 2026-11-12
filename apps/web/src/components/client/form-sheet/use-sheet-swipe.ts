"use client";

import { useRef, type PointerEvent } from "react";

import { WIDE_LAYOUT_QUERY } from "@/components/client/form/utils";
import type { SwipeSample } from "./type";
import {
  REDUCED_MOTION_QUERY,
  SNAP_BACK_TRANSITION,
  SWIPE_MAX_SAMPLES,
  startsSwipe,
  swipeOffset,
  swipeReleaseAction,
  swipeVelocity,
} from "./utils";

type Swipe = { pointerId: number; startY: number; samples: SwipeSample[] };

// Swipe down to close a phone's form sheet (board HP-Lembar). It starts only on the grab handle
// or the header, never in the form's body, so scrolling a form never moves the sheet. The sheet
// follows the pointer down; released past a quarter of its height, or flicked down, it asks to
// close (through the same guard as every other close), otherwise it goes back up. Not from md up,
// where the drawer has no handle. The sheet's position is set on the element directly, not
// through React state, so a drag never re-renders the form.
export function useSheetSwipe(requestClose: () => boolean) {
  const swipeRef = useRef<Swipe | null>(null);

  function onPointerDown(event: PointerEvent<HTMLDivElement>) {
    const sheet = event.currentTarget;
    if (!event.isPrimary || event.button !== 0) return;
    if (window.matchMedia(WIDE_LAYOUT_QUERY).matches) return;
    if (!startsSwipe(sheet, event.target)) return;
    sheet.setPointerCapture(event.pointerId);
    sheet.style.transition = "none";
    swipeRef.current = {
      pointerId: event.pointerId,
      startY: event.clientY,
      samples: [{ y: event.clientY, time: event.timeStamp }],
    };
  }

  function onPointerMove(event: PointerEvent<HTMLDivElement>) {
    const swipe = swipeRef.current;
    if (swipe === null || event.pointerId !== swipe.pointerId) return;
    swipe.samples.push({ y: event.clientY, time: event.timeStamp });
    if (swipe.samples.length > SWIPE_MAX_SAMPLES) swipe.samples.shift();
    const offset = swipeOffset(swipe.startY, event.clientY);
    event.currentTarget.style.transform = `translateY(${offset}px)`;
  }

  function finish(event: PointerEvent<HTMLDivElement>, released: boolean) {
    const swipe = swipeRef.current;
    if (swipe === null || event.pointerId !== swipe.pointerId) return;
    swipeRef.current = null;
    const sheet = event.currentTarget;
    const reduceMotion = window.matchMedia(REDUCED_MOTION_QUERY).matches;
    if (released) {
      const release = { y: event.clientY, time: event.timeStamp };
      const action = swipeReleaseAction({
        distance: swipeOffset(swipe.startY, release.y),
        // The layout height: getBoundingClientRect would include the drag's transform.
        sheetHeight: sheet.offsetHeight,
        velocity: swipeVelocity(swipe.samples, release),
      });
      if (action === "close" && requestClose()) {
        // The exit slide continues from where the finger left the sheet. With reduced motion
        // there is none: the sheet goes at once (Radix unmounts it when it has no animation).
        if (reduceMotion) sheet.style.animation = "none";
        return;
      }
    }
    snapBack(sheet, reduceMotion);
  }

  return {
    onPointerDown,
    onPointerMove,
    onPointerUp: (event: PointerEvent<HTMLDivElement>) => finish(event, true),
    onPointerCancel: (event: PointerEvent<HTMLDivElement>) =>
      finish(event, false),
  };
}

// Puts the sheet back where it rests: animated, or at once with reduced motion.
function snapBack(sheet: HTMLElement, reduceMotion: boolean) {
  if (reduceMotion || sheet.style.transform === "") {
    sheet.style.transform = "";
    sheet.style.transition = "";
    return;
  }
  const settled = (event: TransitionEvent) => {
    if (event.target !== sheet) return;
    sheet.style.transition = "";
    sheet.removeEventListener("transitionend", settled);
    sheet.removeEventListener("transitioncancel", settled);
  };
  sheet.addEventListener("transitionend", settled);
  sheet.addEventListener("transitioncancel", settled);
  sheet.style.transition = SNAP_BACK_TRANSITION;
  sheet.style.transform = "";
}
