import { afterEach, describe, expect, it, vi } from "vitest";

import {
  API_INTERNAL_URL_ENV,
  apiInternalUrl,
  parseApiInternalUrl,
} from "./api-internal-url";

describe("parseApiInternalUrl", () => {
  it.each([
    {
      name: "a local origin",
      input: "http://localhost:8080",
      want: "http://localhost:8080",
    },
    {
      name: "a trailing slash",
      input: "http://localhost:8080/",
      want: "http://localhost:8080",
    },
    {
      name: "the Compose service",
      input: "http://api:8080",
      want: "http://api:8080",
    },
    {
      name: "https on its default port",
      input: "https://api.example.com:443/",
      want: "https://api.example.com",
    },
    {
      name: "surrounding spaces",
      input: "  http://localhost:8080  ",
      want: "http://localhost:8080",
    },
  ])("accepts $name", ({ input, want }) => {
    expect(parseApiInternalUrl(input)).toBe(want);
  });

  it.each([
    { name: "unset", input: undefined, want: /is not set/ },
    { name: "empty", input: "", want: /is not set/ },
    { name: "blank", input: "   ", want: /is not set/ },
    { name: "not a URL", input: "not a url", want: /is not a valid URL/ },
    { name: "ftp", input: "ftp://api:8080", want: /must use http or https/ },
    {
      name: "a host without a scheme",
      input: "localhost:8080",
      want: /must use http or https/,
    },
    {
      name: "credentials",
      input: "http://user:pass@api:8080",
      want: /must not contain credentials/,
    },
    {
      name: "a username only",
      input: "http://user@api:8080",
      want: /must not contain credentials/,
    },
    { name: "a path", input: "http://api:8080/api", want: /origin only/ },
    { name: "a query", input: "http://api:8080/?x=1", want: /origin only/ },
    { name: "a fragment", input: "http://api:8080/#x", want: /origin only/ },
  ])("rejects $name", ({ input, want }) => {
    expect(() => parseApiInternalUrl(input)).toThrow(want);
    expect(() => parseApiInternalUrl(input)).toThrow(API_INTERNAL_URL_ENV);
  });
});

describe("apiInternalUrl", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("reads the environment variable", () => {
    vi.stubEnv(API_INTERNAL_URL_ENV, "http://api:8080/");
    expect(apiInternalUrl()).toBe("http://api:8080");
  });

  it("throws when the variable is unset", () => {
    vi.stubEnv(API_INTERNAL_URL_ENV, undefined);
    expect(() => apiInternalUrl()).toThrow(/is not set/);
  });
});
