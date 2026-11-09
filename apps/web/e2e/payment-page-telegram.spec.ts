import {
  expect,
  request as playwrightRequest,
  test,
  type Page,
  type Response,
  type TestInfo,
} from "@playwright/test";

import type { TelegramDeepLink } from "../src/lib/api/generated/model";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import {
  expectTouchTarget,
  makeLink,
  readPage,
  rentSinceLastMonth,
  runId,
  staffApi,
  tenantClientAddress,
  thisMonthInvoiceId,
  trackTokenLeaks,
} from "./support/payment-page";
import { projectDigit } from "./support/projects";
import { HENDRA } from "./support/staff";

// "Hubungkan Telegram" on Bayar-Pilih (decision 42: a demo channel, development and staging).
// The e2e API runs with a fake bot (TELEGRAM_* in the e2e environment): the deep link is made
// without calling Telegram, and the tenant's "Start" is the bot's webhook update, posted here
// with the webhook secret as Telegram would. The bot's reply goes nowhere, which the API logs and
// ignores. This spec's tenants: numbers 0815-5…, rooms "Kamar Telegram".

const API = process.env.API_DEV_ORIGIN ?? "http://127.0.0.1:8080";
const WEBHOOK_SECRET = process.env.TELEGRAM_WEBHOOK_SECRET ?? "";
const BOT_USERNAME = process.env.TELEGRAM_BOT_USERNAME ?? "";
const CARD = "Pengingat lewat Telegram";
const CARD_TEXT = "Terima pengingat tagihan di Telegram, bukan di WhatsApp.";
const OPEN_HINT = "Tekan Start di Telegram. Link ini berlaku 10 menit.";
const LINKED_TEXT = "Telegram terhubung. Pengingat berikutnya dikirim di sini.";

test.use({
  storageState: { cookies: [], origins: [] },
  extraHTTPHeaders: async ({}, provide, testInfo) => {
    await provide({ "X-Forwarded-For": tenantClientAddress(testInfo, 21) });
  },
});

test.beforeAll(() => {
  if (WEBHOOK_SECRET === "" || BOT_USERNAME === "") {
    throw new Error(
      "TELEGRAM_WEBHOOK_SECRET and TELEGRAM_BOT_USERNAME are set in the e2e environment (web.yml)",
    );
  }
});

// A Telegram chat id of the test's own (private chats are positive).
function chatId(testInfo: TestInfo): number {
  return Number(`7${runId()}${projectDigit(testInfo)}${testInfo.retry}`);
}

// A tenant of the test's own (digit: the test's digit in the tenant number) opens their page. With
// clock, the page runs on Playwright's clock, so the test can move time forward.
async function openTelegramPage(
  page: Page,
  testInfo: TestInfo,
  { digit, clock = false }: { digit: number; clock?: boolean },
) {
  const api = await staffApi(HENDRA, testInfo);
  const rented = await rentSinceLastMonth(api, testInfo, {
    digit,
    phonePrefix: "0815-5",
    price: 1_250_000,
    unitBase: "Kamar Telegram",
  });
  const { path, token } = await makeLink(
    api,
    rented.orgId,
    thisMonthInvoiceId(rented.account),
  );
  await api.dispose();

  const expected = await readPage(page, path);
  expect(expected.telegram).toEqual({ available: true, linked: false });

  const outside = trackOutsideRequests(page);
  const leaks = trackTokenLeaks(page, path, token);
  if (clock) await page.clock.install();
  await page.goto(path);
  return { path, outside, leaks };
}

// The tenant presses Start in Telegram: the bot's webhook gets "/start <code>".
async function pressStart(testInfo: TestInfo, deepLink: string) {
  const code = new URL(deepLink).searchParams.get("start");
  if (code === null) throw new Error("the deep link has no start code");
  const telegram = await playwrightRequest.newContext({ baseURL: API });
  const id = chatId(testInfo);
  const answer = await telegram.post("/webhooks/telegram", {
    headers: { "X-Telegram-Bot-Api-Secret-Token": WEBHOOK_SECRET },
    data: {
      update_id: id,
      message: {
        message_id: 1,
        from: { id, is_bot: false },
        chat: { id, type: "private" },
        text: `/start ${code}`,
      },
    },
  });
  expect(answer.status()).toBe(200);
  await telegram.dispose();
}

test("Hubungkan Telegram gives a one-time link, and the page says when it is linked", async ({
  page,
}, testInfo) => {
  const fixture = await openTelegramPage(page, testInfo, { digit: 1 });

  // Bayar-Pilih: a quiet card under "Lanjut bayar", which stays the one primary.
  const card = page.getByRole("region", { name: CARD });
  await expect(card).toContainText(CARD_TEXT);
  const connect = card.getByRole("button", { name: "Hubungkan Telegram" });
  await expectTouchTarget(connect);
  await expect(
    page.getByRole("button", { name: /^Lanjut bayar Rp/ }),
  ).toBeVisible();
  await card.scrollIntoViewIfNeeded();
  await attachViewportScreenshot(page, testInfo, "payment-telegram-offer");

  // The link is asked for once, then opened by the tenant (a new tab or the app, no referrer).
  const asked: Promise<Response> = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname === `/api${fixture.path}/telegram`,
  );
  await connect.click();
  const answer = await asked;
  expect(answer.status()).toBe(200);
  const { deepLink } = (await answer.json()) as TelegramDeepLink;
  expect(deepLink).toMatch(
    new RegExp(`^https://t\\.me/${BOT_USERNAME}\\?start=[A-Za-z0-9_-]+$`),
  );
  const open = card.getByRole("link", { name: "Buka Telegram" });
  await expect(open).toHaveAttribute("href", deepLink);
  await expect(open).toHaveAttribute("target", "_blank");
  await expect(open).toHaveAttribute("rel", /noreferrer/);
  await expectTouchTarget(open);
  await expect(card).toContainText(OPEN_HINT);
  await card.scrollIntoViewIfNeeded();
  await attachViewportScreenshot(page, testInfo, "payment-telegram-open");

  // Start in Telegram; back on the page, the tab turning visible again reads the page again
  // (TanStack Query's focus refetch), with no reload, and the card says so.
  await pressStart(testInfo, deepLink);
  const reread: Promise<Response> = page.waitForResponse(
    (response) =>
      response.request().method() === "GET" &&
      new URL(response.url()).pathname === `/api${fixture.path}`,
  );
  await page.evaluate(() =>
    window.dispatchEvent(new Event("visibilitychange")),
  );
  expect((await reread).status()).toBe(200);
  await expect(card).toContainText(LINKED_TEXT);
  await expect(card.getByRole("button")).toHaveCount(0);
  await expect(card.getByRole("link")).toHaveCount(0);
  await card.scrollIntoViewIfNeeded();
  await attachViewportScreenshot(page, testInfo, "payment-telegram-linked");

  expect(fixture.outside).toEqual([]);
  expect(fixture.leaks).toEqual([]);
});

test("an expired link goes back to Hubungkan Telegram, as the bot tells the tenant", async ({
  page,
}, testInfo) => {
  const fixture = await openTelegramPage(page, testInfo, {
    digit: 2,
    clock: true,
  });

  const card = page.getByRole("region", { name: CARD });
  const connect = card.getByRole("button", { name: "Hubungkan Telegram" });
  await connect.click();
  const open = card.getByRole("link", { name: "Buka Telegram" });
  await expect(open).toBeVisible();

  // Still shown just before the code's 10 minutes are up.
  await page.clock.fastForward("09:30");
  await expect(open).toBeVisible();

  // The code expired: the link and its hint go, and "Hubungkan Telegram" asks for a new one.
  await page.clock.fastForward("00:31");
  await expect(open).toHaveCount(0);
  await expect(card).not.toContainText(OPEN_HINT);
  await expect(connect).toBeEnabled();
  await connect.click();
  await expect(open).toBeVisible();

  expect(fixture.outside).toEqual([]);
  expect(fixture.leaks).toEqual([]);
});
