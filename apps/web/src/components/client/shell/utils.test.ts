import { describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api/errors";
import type { Capability } from "@/lib/api/generated/model";

import {
  currentCapabilities,
  cookieSaysCollapsed,
  isSidebarCollapsed,
  SIDEBAR_SKELETON_SCRIPT,
  sidebarCookie,
  sidebarToggleLabel,
} from "./utils";

const SERVER: Capability[] = [{ action: "member:read", access: "allow" }];
const QUERY: Capability[] = [{ action: "member:read", access: "deny" }];

describe("currentCapabilities", () => {
  it("uses the server's until the query answers", () => {
    expect(currentCapabilities(SERVER, undefined, null)).toBe(SERVER);
  });

  it("follows the query once it answers", () => {
    expect(currentCapabilities(SERVER, QUERY, null)).toBe(QUERY);
  });

  it("keeps the last answer on another error", () => {
    const error = new ApiError({
      status: 0,
      code: "network_error",
      message: "x",
    });
    expect(currentCapabilities(SERVER, QUERY, error)).toBe(QUERY);
  });

  it("grants nothing after a 401", () => {
    const error = new ApiError({
      status: 401,
      code: "unauthorized",
      message: "x",
    });
    expect(currentCapabilities(SERVER, QUERY, error)).toEqual([]);
  });
});

describe("the side menu's collapsed choice", () => {
  it("reads the cookie value", () => {
    expect(isSidebarCollapsed("collapsed")).toBe(true);
    expect(isSidebarCollapsed(undefined)).toBe(false);
    expect(isSidebarCollapsed("")).toBe(false);
    expect(isSidebarCollapsed("wide")).toBe(false);
  });

  it("remembers collapsed for a year and deletes the cookie for wide", () => {
    expect(sidebarCookie(true, false)).toBe(
      "cf_sidebar=collapsed; Max-Age=31536000; Path=/; SameSite=Lax",
    );
    expect(sidebarCookie(false, false)).toBe(
      "cf_sidebar=; Max-Age=0; Path=/; SameSite=Lax",
    );
    expect(sidebarCookie(true, true)).toBe(
      "cf_sidebar=collapsed; Max-Age=31536000; Path=/; SameSite=Lax; Secure",
    );
  });

  it.each([
    { cookie: "", want: false },
    { cookie: "cf_sidebar=collapsed", want: true },
    { cookie: "a=1; cf_sidebar=collapsed; b=2", want: true },
    { cookie: "cf_sidebar=", want: false },
    { cookie: "xcf_sidebar=collapsed", want: false },
    { cookie: "cf_sidebar=collapsedx", want: false },
  ])("finds the choice in document.cookie $cookie", ({ cookie, want }) => {
    expect(cookieSaysCollapsed(cookie)).toBe(want);
  });

  it("marks the skeleton only when the cookie says collapsed", () => {
    // Runs the inline script against a stand-in document, as the browser would.
    const run = (cookie: string) => {
      const attributes: Record<string, string> = {};
      const previous = {
        setAttribute: (name: string, value: string) => {
          attributes[name] = value;
        },
      };
      const document = {
        cookie,
        currentScript: { previousElementSibling: previous },
      };
      new Function("document", SIDEBAR_SKELETON_SCRIPT)(document);
      return attributes["data-sidebar"];
    };
    expect(run("a=1; cf_sidebar=collapsed")).toBe("collapsed");
    expect(run("cf_sidebar=")).toBeUndefined();
    expect(run("")).toBeUndefined();
  });

  it("words the toggle by what it does", () => {
    expect(sidebarToggleLabel(false)).toBe("Ciutkan menu");
    expect(sidebarToggleLabel(true)).toBe("Lebarkan menu");
  });
});
