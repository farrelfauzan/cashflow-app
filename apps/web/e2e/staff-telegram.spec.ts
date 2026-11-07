import {
  expect,
  request as playwrightRequest,
  test,
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
  orgIdByName,
  projectDigit,
  runId,
  staffApi,
} from "./support/payment-page";
import { openMenu } from "./support/shell";
import { LESTARI, SARI, type SeededStaff } from "./support/staff";

// A staff member's "Telegram" in the side menu (decision 42: a demo channel, development and
// staging). The e2e API runs with a fake bot and TELEGRAM_MESSAGES=linked (web.yml): the deep
// link is made without calling Telegram, and "Start" is the bot's webhook update, posted here
// with the webhook secret as Telegram would. The bot's reply goes nowhere. Each project links its
// own member (SARI on iPad, LESTARI on the phone), so the two never race. A linked member's login
// codes would go to the fake bot, so the member is unlinked before and after the test, even when
// it fails: the setup project signs them in at the start of every run.

const API = process.env.API_DEV_ORIGIN ?? "http://127.0.0.1:8080";
const WEBHOOK_SECRET = process.env.TELEGRAM_WEBHOOK_SECRET ?? "";
const BOT_USERNAME = process.env.TELEGRAM_BOT_USERNAME ?? "";
const TEXT =
  "Terima kode masuk dan pemberitahuan di Telegram, bukan di WhatsApp.";
const LINKED_TEXT =
  "Telegram terhubung. Kode masuk berikutnya dikirim di sini.";
const OPEN_HINT = "Tekan Start di Telegram. Link ini berlaku 10 menit.";

test.beforeAll(() => {
  if (WEBHOOK_SECRET === "" || BOT_USERNAME === "") {
    throw new Error(
      "TELEGRAM_WEBHOOK_SECRET and TELEGRAM_BOT_USERNAME are set in the e2e environment (web.yml)",
    );
  }
});

function member(testInfo: TestInfo): SeededStaff {
  return testInfo.project.name === "phone" ? LESTARI : SARI;
}

// Both tests link and unlink the same member, so they run one after the other in one worker
// (fullyParallel would race them); a failure does not skip the next one.
test.describe.configure({ mode: "default" });

test.use({
  storageState: async ({}, provide, testInfo) => {
    await provide(member(testInfo).storageState);
  },
});

// A Telegram chat id of the test's own (private chats are positive; the payment page spec's
// start with 7).
// test is the test's own digit, so two tests never share a chat.
function chatId(testInfo: TestInfo, test: number): number {
  return Number(`8${runId()}${projectDigit(testInfo)}${test}${testInfo.retry}`);
}

// The member presses Start in Telegram: the bot's webhook gets "/start <code>".
async function pressStart(testInfo: TestInfo, deepLink: string, test = 1) {
  const code = new URL(deepLink).searchParams.get("start");
  if (code === null) throw new Error("the deep link has no start code");
  const telegram = await playwrightRequest.newContext({ baseURL: API });
  const id = chatId(testInfo, test);
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

async function unlinkMember(testInfo: TestInfo): Promise<void> {
  const api = await staffApi(member(testInfo), testInfo);
  const answer = await api.delete("/api/me/telegram");
  expect(answer.status()).toBe(204);
  await api.dispose();
}

test.afterEach(async ({}, testInfo) => {
  await unlinkMember(testInfo);
});

test("a member links Telegram from the menu, sees it linked, and unlinks it", async ({
  page,
}, testInfo) => {
  const staff = member(testInfo);
  // Unlinked to start with, whatever an earlier run left.
  await unlinkMember(testInfo);
  const api = await staffApi(staff, testInfo);
  const orgId = await orgIdByName(api, staff.organizations[0] ?? "");
  await api.dispose();

  const outside = trackOutsideRequests(page);
  await page.goto(`/o/${orgId}`);

  const menu = await openMenu(page, testInfo);
  const item = menu.getByRole("button", { name: "Telegram", exact: true });
  await expectTouchTarget(item);
  await item.click();
  const dialog = page.getByRole("dialog", { name: "Telegram" });
  await expect(dialog).toContainText(TEXT);
  const connect = dialog.getByRole("button", { name: "Hubungkan Telegram" });
  await expectTouchTarget(connect);
  await attachViewportScreenshot(page, testInfo, "staff-telegram-offer");

  // One request, then the member opens the link (a new tab or the app, no referrer).
  const asked: Promise<Response> = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname === "/api/me/telegram",
  );
  await connect.click();
  const answer = await asked;
  expect(answer.status()).toBe(201);
  const { deepLink } = (await answer.json()) as TelegramDeepLink;
  expect(deepLink).toMatch(
    new RegExp(`^https://t\\.me/${BOT_USERNAME}\\?start=[A-Za-z0-9_-]+$`),
  );
  const open = dialog.getByRole("link", { name: "Buka Telegram" });
  await expect(open).toHaveAttribute("href", deepLink);
  await expect(open).toHaveAttribute("target", "_blank");
  await expect(open).toHaveAttribute("rel", /noreferrer/);
  await expectTouchTarget(open);
  await expect(dialog).toContainText(OPEN_HINT);
  await attachViewportScreenshot(page, testInfo, "staff-telegram-open");

  // Start in Telegram; back in the tab, the focus refetch reads it again, with no reload.
  await pressStart(testInfo, deepLink);
  const reread: Promise<Response> = page.waitForResponse(
    (response) =>
      response.request().method() === "GET" &&
      new URL(response.url()).pathname === "/api/me/telegram",
  );
  await page.evaluate(() =>
    window.dispatchEvent(new Event("visibilitychange")),
  );
  expect((await reread).status()).toBe(200);
  await expect(dialog).toContainText(LINKED_TEXT);
  await expect(dialog.getByRole("link")).toHaveCount(0);
  const unlink = dialog.getByRole("button", { name: "Putuskan Telegram" });
  await expectTouchTarget(unlink);
  await attachViewportScreenshot(page, testInfo, "staff-telegram-linked");

  // "Putuskan Telegram": unlinked at once, and "Hubungkan Telegram" is offered again.
  await unlink.click();
  await expect(dialog).toContainText(TEXT);
  await expect(connect).toBeEnabled();

  expect(outside).toEqual([]);
});

test("closing the dialog while it unlinks still shows the member unlinked", async ({
  page,
}, testInfo) => {
  const staff = member(testInfo);
  // Linked through the API, as the first test does through the dialog.
  await unlinkMember(testInfo);
  const api = await staffApi(staff, testInfo);
  const orgId = await orgIdByName(api, staff.organizations[0] ?? "");
  const asked = await api.post("/api/me/telegram");
  expect(asked.status()).toBe(201);
  const { deepLink } = (await asked.json()) as TelegramDeepLink;
  await api.dispose();
  await pressStart(testInfo, deepLink, 2);

  // The unlink answer is held until the dialog is closed.
  let release: () => void = () => {};
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/me/telegram", async (route) => {
    if (route.request().method() !== "DELETE") return route.fallback();
    await held;
    await route.fallback();
  });

  await page.goto(`/o/${orgId}`);
  const menu = await openMenu(page, testInfo);
  const item = menu.getByRole("button", { name: "Telegram", exact: true });
  await item.click();
  const dialog = page.getByRole("dialog", { name: "Telegram" });
  await expect(dialog).toContainText(LINKED_TEXT);
  await dialog.getByRole("button", { name: "Putuskan Telegram" }).click();
  await expect(
    dialog.getByRole("button", { name: "Memutuskan…" }),
  ).toBeDisabled();
  await dialog.getByRole("button", { name: "Tutup" }).click();
  await expect(dialog).toBeHidden();

  // The answer arrives after the dialog closed; the menu item still reads getMyTelegram again.
  const unlinked = page.waitForResponse(
    (response) =>
      response.request().method() === "DELETE" &&
      new URL(response.url()).pathname === "/api/me/telegram",
  );
  const reread = page.waitForResponse(
    (response) =>
      response.request().method() === "GET" &&
      new URL(response.url()).pathname === "/api/me/telegram",
  );
  release();
  expect((await unlinked).status()).toBe(204);
  expect((await reread).status()).toBe(200);

  await item.click();
  await expect(dialog).toContainText(TEXT);
  await expect(
    dialog.getByRole("button", { name: "Hubungkan Telegram" }),
  ).toBeEnabled();
});
