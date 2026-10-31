import { describe, expect, it } from "vitest";
import { z } from "zod";

import {
  CODE_FORMAT_MESSAGE,
  EMPTY_PHONE_MESSAGE,
  loginCodeSchema,
  loginPhoneSchema,
} from "./utils";

function check(schema: z.ZodType, values: unknown) {
  const result = schema.safeParse(values);
  if (result.success) return { ok: true, input: result.data };
  const { fieldErrors } = z.flattenError(result.error);
  return {
    ok: false,
    errors: Object.fromEntries(
      Object.entries(fieldErrors).map(([field, messages]) => [
        field,
        (messages as string[] | undefined)?.[0],
      ]),
    ),
  };
}

describe("loginPhoneSchema", () => {
  it("sends the number trimmed, as typed otherwise; the API normalizes it", () => {
    expect(check(loginPhoneSchema, { phone: " 0812-3456-7890 " })).toEqual({
      ok: true,
      input: { phone: "0812-3456-7890" },
    });
  });

  it("asks for a number when the field is empty or blank", () => {
    for (const phone of ["", "   "]) {
      expect(check(loginPhoneSchema, { phone })).toEqual({
        ok: false,
        errors: { phone: EMPTY_PHONE_MESSAGE },
      });
    }
  });

  it("leaves a number that is not one to the API", () => {
    expect(check(loginPhoneSchema, { phone: "12345" }).ok).toBe(true);
  });
});

describe("loginCodeSchema", () => {
  it("sends six digits", () => {
    expect(check(loginCodeSchema, { code: "123456" })).toEqual({
      ok: true,
      input: { code: "123456" },
    });
  });

  it("refuses an empty or short code", () => {
    for (const code of ["", "12", "12345"]) {
      expect(check(loginCodeSchema, { code })).toEqual({
        ok: false,
        errors: { code: CODE_FORMAT_MESSAGE },
      });
    }
  });
});
