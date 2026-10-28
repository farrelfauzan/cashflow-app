import { describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api/errors";
import type { Capability } from "@/lib/api/generated/model";

import { currentCapabilities } from "./utils";

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
