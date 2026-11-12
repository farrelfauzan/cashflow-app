// The phone's push and pop between pages (board HP-Pindah, approved 2026-11-08). A link into a
// deeper page carries NAV_FORWARD, the ‹ link back carries NAV_BACK, and globals.css slides the
// whole screen by that direction on the phone only. A navigation without one (the tab bar, a
// filter, a refresh, the browser's Back and Forward) changes the page at once, as before.
export const NAV_FORWARD = "nav-forward";
export const NAV_BACK = "nav-back";

// For Link's transitionTypes.
export const FORWARD_TYPES: string[] = [NAV_FORWARD];
export const BACK_TYPES: string[] = [NAV_BACK];

// The view-transition class of PageMotion's marker for each type when it updates. "none" for
// anything else, so an untyped navigation starts no view transition.
export const PAGE_MOTION_CLASSES = {
  [NAV_FORWARD]: "page-motion",
  [NAV_BACK]: "page-motion",
  default: "none",
};

// The attribute on <html> that gives globals.css the direction of a view transition started
// outside React (the ‹ link going back in the history): data-page-motion="nav-back".
export const PAGE_MOTION_ATTRIBUTE = "data-page-motion";

// How long the ‹ link waits for the page it went back to before the slide gives up and shows
// whatever is on screen. Going back to a page kept in memory takes one render.
export const BACK_WAIT_MS = 1000;

// The path of an href, without its query or hash ("/o/1/tenants?q=rina" → "/o/1/tenants").
export function pathOf(href: string): string {
  const end = href.search(/[?#]/);
  return end === -1 ? href : href.slice(0, end);
}

// Whether the ‹ link may go back in the browser's history instead of opening its page again: only
// when the page before this one is the page it leads to. Going back restores that page as it was
// left (its scroll position, search and filters) and keeps the history from growing.
export function canGoBackTo(href: string, previous: string | null): boolean {
  return previous !== null && previous === pathOf(href);
}

// The pages this tab has shown as far as the ‹ link needs them: the current path and the one
// before it. Kept in memory, so a reload starts over.
type PathTrail = { previous: string | null; current: string | null };

export function nextTrail(trail: PathTrail, pathname: string): PathTrail {
  if (trail.current === pathname) return trail;
  return { previous: trail.current, current: pathname };
}

let trail: PathTrail = { previous: null, current: null };
let waiters: { path: string; resolve: () => void }[] = [];

// PageMotion reports every page shown, once it is on screen (a layout effect, before the browser
// takes the new snapshot of a view transition).
export function recordPathname(pathname: string): void {
  trail = nextTrail(trail, pathname);
  const ready = waiters.filter((waiter) => waiter.path === pathname);
  waiters = waiters.filter((waiter) => waiter.path !== pathname);
  for (const waiter of ready) waiter.resolve();
}

export function previousPathname(): string | null {
  return trail.previous;
}

// Resolves once the page at path is on screen, or after timeoutMs, whichever comes first.
export function waitForPathname(
  path: string,
  timeoutMs: number,
): Promise<void> {
  return new Promise((resolve) => {
    const waiter = { path, resolve };
    waiters.push(waiter);
    setTimeout(() => {
      waiters = waiters.filter((other) => other !== waiter);
      resolve();
    }, timeoutMs);
  });
}
