import { ApiError } from "@/lib/api/errors";
import type { Capability } from "@/lib/api/generated/model";

const NO_CAPABILITIES: readonly Capability[] = [];

// The capabilities the ability follows: getOrgMe's latest answer, or the ones the server loaded
// with the page until the browser has its own. A 401 means the session is gone, so nothing is
// granted while the API client sends the member to login.
export function currentCapabilities(
  fromServer: readonly Capability[],
  fromQuery: readonly Capability[] | undefined,
  error: unknown,
): readonly Capability[] {
  if (error instanceof ApiError && error.status === 401) return NO_CAPABILITIES;
  return fromQuery ?? fromServer;
}

// The iPad side menu, wide (228px, icons and labels) or collapsed to a rail of icons (76px). The
// choice is a plain UI preference kept in a cookie, nothing sensitive, so the server shell draws
// the side menu at the right width on the first paint and after a reload.
export const SIDEBAR_COOKIE = "cf_sidebar";
export const SIDEBAR_COLLAPSED = "collapsed";
// A year, in seconds.
const SIDEBAR_COOKIE_MAX_AGE = 60 * 60 * 24 * 365;

export function isSidebarCollapsed(value: string | undefined): boolean {
  return value === SIDEBAR_COLLAPSED;
}

// The cookie the toggle writes: collapsed is remembered for a year; wide is the default, so it
// deletes the cookie. Secure on HTTPS (every environment but local development).
export function sidebarCookie(collapsed: boolean, secure: boolean): string {
  const parts = collapsed
    ? [
        `${SIDEBAR_COOKIE}=${SIDEBAR_COLLAPSED}`,
        `Max-Age=${SIDEBAR_COOKIE_MAX_AGE}`,
      ]
    : [`${SIDEBAR_COOKIE}=`, "Max-Age=0"];
  parts.push("Path=/", "SameSite=Lax");
  if (secure) parts.push("Secure");
  return parts.join("; ");
}

// Whether a document.cookie string holds the collapsed choice.
export function cookieSaysCollapsed(cookie: string): boolean {
  return cookie
    .split(";")
    .some((part) => part.trim() === `${SIDEBAR_COOKIE}=${SIDEBAR_COLLAPSED}`);
}

// The toggle's text, also its accessible name: what pressing it does.
export function sidebarToggleLabel(collapsed: boolean): string {
  return collapsed ? "Lebarkan menu" : "Ciutkan menu";
}

// The attribute on <html> that carries the choice for what the server cannot draw from the
// cookie: the org shell's loading skeleton, which is prerendered without cookies.
export const SIDEBAR_CHOICE_ATTRIBUTE = "data-sidebar-choice";

// Runs inline in the root layout's <head>, before the first paint: marks <html> when the cookie
// says collapsed, so the skeleton's side menu starts at the rail's width. It sits in the root
// layout because React only hydrates that and never renders it again on the client, where a
// <script> would not run (and React warns). Same test as cookieSaysCollapsed.
export const SIDEBAR_CHOICE_SCRIPT = `(function(){try{if(document.cookie.split(";").some(function(p){return p.trim()==="${SIDEBAR_COOKIE}=${SIDEBAR_COLLAPSED}"}))document.documentElement.setAttribute("${SIDEBAR_CHOICE_ATTRIBUTE}","${SIDEBAR_COLLAPSED}")}catch(e){}})()`;

// Keeps <html>'s mark in step with the toggle, so a skeleton shown later on this page (entering
// another organization) has the width the member chose.
export function markSidebarChoice(root: Element, collapsed: boolean): void {
  if (collapsed) root.setAttribute(SIDEBAR_CHOICE_ATTRIBUTE, SIDEBAR_COLLAPSED);
  else root.removeAttribute(SIDEBAR_CHOICE_ATTRIBUTE);
}
