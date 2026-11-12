"use client";

import Link from "next/link";
import type { ComponentProps, MouseEvent } from "react";

import {
  BACK_TYPES,
  BACK_WAIT_MS,
  canGoBackTo,
  NAV_BACK,
  PAGE_MOTION_ATTRIBUTE,
  pathOf,
  previousPathname,
  waitForPathname,
} from "./utils";

type BackLinkProps = Omit<
  ComponentProps<typeof Link>,
  "href" | "transitionTypes"
> & {
  href: string;
};

// The ‹ link to the page above this one (board HP-Pindah: pop, the page slides away to the right).
//
// When that page is the one before in the history it goes back, so the list comes back scrolled
// where it was left, with its search and filters, and the history does not grow. React renders a
// Back without a view transition, so the link starts one itself: the old screen is captured, the
// history goes back inside it, and the new screen is captured once the page is shown; the
// direction reaches globals.css through data-page-motion on <html>. Without view transitions it
// just goes back.
//
// Otherwise (a reload, a shared link) it opens the page, with the pop as its type. A modified
// click (new tab, new window) is the plain link.
export function BackLink({ href, onClick, ...props }: BackLinkProps) {
  const handleClick = (event: MouseEvent<HTMLAnchorElement>) => {
    onClick?.(event);
    if (
      event.defaultPrevented ||
      event.button !== 0 ||
      event.metaKey ||
      event.ctrlKey ||
      event.shiftKey ||
      event.altKey ||
      !canGoBackTo(href, previousPathname())
    ) {
      return;
    }
    event.preventDefault();
    if (typeof document.startViewTransition !== "function") {
      window.history.back();
      return;
    }
    const root = document.documentElement;
    root.setAttribute(PAGE_MOTION_ATTRIBUTE, NAV_BACK);
    const transition = document.startViewTransition(() => {
      const shown = waitForPathname(pathOf(href), BACK_WAIT_MS);
      window.history.back();
      return shown;
    });
    transition.finished.finally(() => {
      root.removeAttribute(PAGE_MOTION_ATTRIBUTE);
    });
  };

  return (
    <Link
      href={href}
      transitionTypes={BACK_TYPES}
      onClick={handleClick}
      {...props}
    />
  );
}
