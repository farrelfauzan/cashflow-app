import { APP_NAME } from "@/lib/app-name";

// The ReCash logo, option 3 "Koin berputar" (board Logo-Opsi-3, approved by the user on
// 2026-11-08): on a 64×64 accent tile, a cream arrow turning around a coin. public/icon.svg draws
// the same mark for the browser and the PWA icons; utils.test.ts keeps the two in step.
export const MARK_VIEW_BOX = "0 0 64 64";
export const MARK_ARC_PATH = "M49.85 25.5 A19 19 0 1 1 30.34 13.07";
export const MARK_ARROWHEAD_PATH = "M29.82 7.09 L38.31 12.37 L30.86 19.05 Z";

// The wordmark: "Re" in ink and "Cash" in the accent, read together as APP_NAME.
export const WORDMARK_LEAD = APP_NAME.slice(0, 2);
export const WORDMARK_TAIL = APP_NAME.slice(2);
