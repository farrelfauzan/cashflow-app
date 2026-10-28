import { describe, expect, it } from "vitest";

import { DEFAULT_POST_LOGIN_PATH, postLoginPath } from "./post-login-path";

describe("postLoginPath", () => {
  it.each([
    { name: "no next", input: null, want: DEFAULT_POST_LOGIN_PATH },
    { name: "undefined", input: undefined, want: DEFAULT_POST_LOGIN_PATH },
    { name: "an empty next", input: "", want: DEFAULT_POST_LOGIN_PATH },
    {
      name: "a safe staff path",
      input: "/organizations",
      want: "/organizations",
    },
    {
      name: "a safe staff path with query and hash",
      input: "/o/org-1/members?status=aktif#daftar",
      want: "/o/org-1/members?status=aktif#daftar",
    },
    { name: "the login page", input: "/login", want: DEFAULT_POST_LOGIN_PATH },
    {
      name: "the login page with its own next",
      input: "/login?next=%2Forganizations",
      want: DEFAULT_POST_LOGIN_PATH,
    },
    {
      name: "a tenant page",
      input: "/t/secret-token",
      want: DEFAULT_POST_LOGIN_PATH,
    },
    {
      name: "the bare tenant root",
      input: "/t",
      want: DEFAULT_POST_LOGIN_PATH,
    },
    {
      name: "a protocol-relative URL",
      input: "//evil.example/organizations",
      want: DEFAULT_POST_LOGIN_PATH,
    },
    {
      name: "a backslash protocol-relative URL",
      input: "/\\evil.example",
      want: DEFAULT_POST_LOGIN_PATH,
    },
    {
      name: "an absolute URL",
      input: "https://evil.example/organizations",
      want: DEFAULT_POST_LOGIN_PATH,
    },
    {
      name: "a javascript: URL",
      input: "javascript:alert(1)",
      want: DEFAULT_POST_LOGIN_PATH,
    },
    {
      name: "a relative path without a leading slash",
      input: "organizations",
      want: DEFAULT_POST_LOGIN_PATH,
    },
  ])("$name gives $want", ({ input, want }) => {
    expect(postLoginPath(input)).toBe(want);
  });
});
