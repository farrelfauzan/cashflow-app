import { describe, expect, it } from "vitest";

import {
  ApiError,
  apiErrorFromResponse,
  GENERIC_ERROR_MESSAGE,
  networkError,
  UNKNOWN_ERROR_CODE,
  unexpectedResponseError,
} from "./errors";

describe("apiErrorFromResponse", () => {
  it.each([
    {
      name: "a full error body",
      body: '{"error":{"code":"validation_failed","message":"Nomor tidak valid.","field":"phone"}}',
      want: {
        code: "validation_failed",
        message: "Nomor tidak valid.",
        field: "phone",
      },
    },
    {
      name: "a body without field",
      body: '{"error":{"code":"forbidden","message":"Anda tidak punya akses."}}',
      want: {
        code: "forbidden",
        message: "Anda tidak punya akses.",
        field: undefined,
      },
    },
    {
      name: "an empty field",
      body: '{"error":{"code":"conflict","message":"Sudah ada.","field":""}}',
      want: { code: "conflict", message: "Sudah ada.", field: undefined },
    },
    {
      name: "a non-string message",
      body: '{"error":{"code":"not_found","message":42}}',
      want: {
        code: "not_found",
        message: GENERIC_ERROR_MESSAGE,
        field: undefined,
      },
    },
    {
      name: "a blank message",
      body: '{"error":{"code":"not_found","message":"  "}}',
      want: {
        code: "not_found",
        message: GENERIC_ERROR_MESSAGE,
        field: undefined,
      },
    },
  ])("reads $name", ({ body, want }) => {
    const error = apiErrorFromResponse(422, body);
    expect(error).toBeInstanceOf(ApiError);
    expect(error.name).toBe("ApiError");
    expect(error.status).toBe(422);
    expect(error.code).toBe(want.code);
    expect(error.message).toBe(want.message);
    expect(error.field).toBe(want.field);
  });

  it.each([
    {
      name: "an empty code",
      body: '{"error":{"code":"","message":"Pesan."}}',
    },
    {
      name: "a whitespace code",
      body: '{"error":{"code":"   ","message":"Pesan."}}',
    },
    { name: "a missing code", body: '{"error":{"message":"Pesan."}}' },
    { name: "a numeric code", body: '{"error":{"code":500}}' },
    { name: "error as a string", body: '{"error":"bad"}' },
    { name: "error as null", body: '{"error":null}' },
    { name: "no error key", body: '{"message":"Pesan."}' },
    { name: "a JSON array", body: "[]" },
    { name: "a JSON string", body: '"error"' },
    { name: "JSON null", body: "null" },
    { name: "invalid JSON", body: '{"error":{"code":' },
    { name: "an HTML page", body: "<html>502 Bad Gateway</html>" },
    { name: "an empty body", body: "" },
  ])("gives unknown for $name", ({ body }) => {
    const error = apiErrorFromResponse(500, body);
    expect(error.status).toBe(500);
    expect(error.code).toBe(UNKNOWN_ERROR_CODE);
    expect(error.message).toBe(GENERIC_ERROR_MESSAGE);
    expect(error.field).toBeUndefined();
  });
});

describe("networkError", () => {
  it("has status 0 and the network code", () => {
    const error = networkError();
    expect(error.status).toBe(0);
    expect(error.code).toBe("network_error");
  });
});

describe("unexpectedResponseError", () => {
  it("keeps the status with the unknown code", () => {
    const error = unexpectedResponseError(200);
    expect(error.status).toBe(200);
    expect(error.code).toBe(UNKNOWN_ERROR_CODE);
    expect(error.message).toBe(GENERIC_ERROR_MESSAGE);
  });
});
