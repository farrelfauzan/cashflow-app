import { describe, expect, it } from "vitest";

import {
  ApiError,
  GENERIC_ERROR_MESSAGE,
  NETWORK_ERROR_CODE,
  NETWORK_ERROR_MESSAGE,
} from "@/lib/api/errors";

import { staffTelegramErrorView } from "./utils";

describe("staffTelegramErrorView", () => {
  it("hides the menu item when the environment has no bot after all", () => {
    expect(
      staffTelegramErrorView(
        new ApiError({
          status: 404,
          code: "telegram_unavailable",
          message: "Telegram tidak tersedia.",
        }),
      ),
    ).toEqual({ kind: "hidden" });
  });

  it("shows the API's own message otherwise", () => {
    expect(
      staffTelegramErrorView(
        new ApiError({
          status: 429,
          code: "rate_limited",
          message: "Terlalu banyak permintaan. Coba lagi nanti.",
        }),
      ),
    ).toEqual({
      kind: "alert",
      message: "Terlalu banyak permintaan. Coba lagi nanti.",
    });
    expect(
      staffTelegramErrorView(
        new ApiError({
          status: 0,
          code: NETWORK_ERROR_CODE,
          message: NETWORK_ERROR_MESSAGE,
        }),
      ),
    ).toEqual({ kind: "alert", message: NETWORK_ERROR_MESSAGE });
  });

  it("shows the generic message for an error that is not the API's", () => {
    expect(staffTelegramErrorView(new Error("boom"))).toEqual({
      kind: "alert",
      message: GENERIC_ERROR_MESSAGE,
    });
  });
});
