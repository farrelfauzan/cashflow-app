import { describe, expect, it } from "vitest";

import {
  isLoginPath,
  LOGIN_PATH,
  loginPathWithNext,
  safeNextPath,
} from "./next-path";

describe("safeNextPath", () => {
  it.each([
    { name: "a plain path", input: "/organizations", want: "/organizations" },
    {
      name: "a path with query and hash",
      input: "/o/org-1/members?status=aktif#daftar",
      want: "/o/org-1/members?status=aktif#daftar",
    },
    {
      name: "encoded slashes",
      input: "/%2F%2Fevil.com",
      want: "/%2F%2Fevil.com",
    },
    {
      name: "an encoded slash inside a segment",
      input: "/o/org-1/tenants%2Fx",
      want: "/o/org-1/tenants%2Fx",
    },
    {
      name: "dot segments",
      input: "/o/./org-1/../org-2",
      want: "/o/org-2",
    },
    { name: "a space", input: "/o/kos melati", want: "/o/kos%20melati" },
    {
      name: "the longest allowed path",
      input: `/${"a".repeat(2047)}`,
      want: `/${"a".repeat(2047)}`,
    },
  ])("keeps $name", ({ input, want }) => {
    expect(safeNextPath(input)).toBe(want);
  });

  it.each([
    { name: "null", input: null },
    { name: "undefined", input: undefined },
    { name: "an empty string", input: "" },
    { name: "a protocol-relative URL", input: "//evil.com" },
    { name: "a protocol-relative URL with path", input: "//evil.com/login" },
    { name: "a backslash after the slash", input: "/\\evil.com" },
    { name: "a backslash later on", input: "/o\\..\\..\\evil.com" },
    { name: "a tab between slashes", input: "/\t/evil.com" },
    { name: "a newline between slashes", input: "/\n/evil.com" },
    { name: "a DEL character", input: "/\u007f/evil.com" },
    { name: "dot segments into //", input: "/..//evil.com" },
    { name: "dot segments into // further in", input: "/a/../..//evil.com" },
    { name: "an absolute URL", input: "https://evil.com" },
    { name: "a same-site absolute URL", input: "http://localhost/o/1" },
    { name: "a javascript URL", input: "javascript:alert(1)" },
    { name: "a relative path", input: "organizations" },
    { name: "a leading space", input: " /organizations" },
    { name: "an overly long path", input: `/${"a".repeat(2048)}` },
  ])("rejects $name", ({ input }) => {
    expect(safeNextPath(input)).toBeNull();
  });
});

describe("isLoginPath", () => {
  it.each([
    { input: "/login", want: true },
    { input: "/login?next=%2Fo", want: true },
    { input: "/login/", want: true },
    { input: "/login/verify", want: true },
    { input: "/login#code", want: true },
    { input: "/loginx", want: false },
    { input: "/logins", want: false },
    { input: "/o/login", want: false },
    { input: "/", want: false },
  ])("$input is $want", ({ input, want }) => {
    expect(isLoginPath(input)).toBe(want);
  });
});

describe("loginPathWithNext", () => {
  it.each([
    {
      name: "a safe path",
      input: "/o/org-1/members?status=aktif",
      want: "/login?next=%2Fo%2Forg-1%2Fmembers%3Fstatus%3Daktif",
    },
    {
      name: "the organizations page",
      input: "/organizations",
      want: "/login?next=%2Forganizations",
    },
    { name: "the login page", input: "/login", want: LOGIN_PATH },
    {
      name: "the login page with next",
      input: "/login?next=%2Fo",
      want: LOGIN_PATH,
    },
    { name: "an unsafe path", input: "//evil.com", want: LOGIN_PATH },
    { name: "an absolute URL", input: "https://evil.com", want: LOGIN_PATH },
    { name: "null", input: null, want: LOGIN_PATH },
    { name: "undefined", input: undefined, want: LOGIN_PATH },
  ])("gives $want for $name", ({ input, want }) => {
    expect(loginPathWithNext(input)).toBe(want);
  });

  it("round-trips the next value", () => {
    const path = "/o/org-1/tenants?q=Budi Santoso&page=2#top";
    const next = new URL(
      loginPathWithNext(path),
      "http://x.invalid",
    ).searchParams.get("next");
    expect(next).toBe(safeNextPath(path));
  });
});
