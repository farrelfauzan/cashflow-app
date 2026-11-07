import { describe, expect, it } from "vitest";
import { z } from "zod";

import {
  CODE_FORMAT_MESSAGE,
  CODE_RESENT_MESSAGE,
  CODE_RESENT_WHATSAPP_MESSAGE,
  EMPTY_PHONE_MESSAGE,
  codeFieldLabel,
  codeResentMessage,
  codeSentEnding,
  loginCodeChannel,
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

describe("loginCodeChannel", () => {
  it("says Telegram or WhatsApp only when Telegram is available and the code was not WhatsApp-only", () => {
    expect(loginCodeChannel(true, false)).toBe("telegram-or-whatsapp");
    expect(loginCodeChannel(true, true)).toBe("whatsapp");
  });

  it("says WhatsApp wherever Telegram is not available", () => {
    expect(loginCodeChannel(false, false)).toBe("whatsapp");
    expect(loginCodeChannel(false, true)).toBe("whatsapp");
  });
});

describe("codeSentEnding", () => {
  it("keeps the original WhatsApp wording", () => {
    const { lead, tail } = codeSentEnding("whatsapp");
    expect(`${lead} ${tail}`).toBe(
      "terdaftar, kode 6 angka sudah dikirim ke WhatsApp-nya.",
    );
  });

  it("names both channels without saying which one carried the code", () => {
    const { lead, tail } = codeSentEnding("telegram-or-whatsapp");
    expect(`${lead} ${tail}`).toBe(
      "terdaftar, kode 6 angka sudah dikirim lewat Telegram atau WhatsApp.",
    );
  });
});

describe("codeFieldLabel", () => {
  it("names WhatsApp only when the code went over WhatsApp", () => {
    expect(codeFieldLabel("whatsapp")).toBe("Kode dari WhatsApp");
    expect(codeFieldLabel("telegram-or-whatsapp")).toBe("Kode masuk");
  });
});

describe("codeResentMessage", () => {
  it("says the code went over WhatsApp after Kirim lewat WhatsApp", () => {
    expect(codeResentMessage(true)).toBe(CODE_RESENT_WHATSAPP_MESSAGE);
    expect(CODE_RESENT_WHATSAPP_MESSAGE).toBe(
      "Kode baru dikirim lewat WhatsApp.",
    );
  });

  it("keeps the plain message for Kirim ulang kode", () => {
    expect(codeResentMessage(false)).toBe(CODE_RESENT_MESSAGE);
    expect(CODE_RESENT_MESSAGE).toBe("Kode baru sudah dikirim.");
  });
});
