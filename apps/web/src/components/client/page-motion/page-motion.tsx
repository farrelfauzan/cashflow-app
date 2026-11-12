"use client";

import { usePathname } from "next/navigation";
import { useLayoutEffect, ViewTransition } from "react";

import { PAGE_MOTION_CLASSES, recordPathname } from "./utils";

// The push and pop between pages on the phone (board HP-Pindah). Mounted once in the shell.
//
// The whole screen slides, not the page's own element: the browser's snapshot of the screen is
// the screen as it was seen, scrolled where it was, so nothing jumps; the top bar and the tab bar
// have their own view-transition-name and stay still (globals.css). React starts a view
// transition for an update inside a ViewTransition only when its class is not "none" (one that
// mounts always starts one, so this one never remounts). This renders an invisible marker that
// holds the path, so every change of page updates it: a link's types (FORWARD_TYPES, BACK_TYPES)
// give it a class and start the transition; any other navigation (the tab bar, a search, a
// refresh) leaves it at "none" and changes the page at once.
//
// React renders the browser's Back and Forward at once, without a view transition, so a pop that
// goes back in the history is started by BackLink itself; this tells it when the page is shown.
export function PageMotion() {
  const pathname = usePathname();

  useLayoutEffect(() => {
    recordPathname(pathname);
  }, [pathname]);

  return (
    <ViewTransition update={PAGE_MOTION_CLASSES} default="none">
      <span
        aria-hidden="true"
        className="pointer-events-none fixed top-0 left-0 size-px overflow-hidden opacity-0"
      >
        {pathname}
      </span>
    </ViewTransition>
  );
}
