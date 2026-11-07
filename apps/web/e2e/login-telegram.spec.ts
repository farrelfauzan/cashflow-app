import { expect, test, type Page, type TestInfo } from "@playwright/test";

import type {
  LoginCodeAccepted,
  LoginCodeRequest,
} from "../src/lib/api/generated/model";

import {
  devlogFilePath,
  devlogOffset,
  waitForLoginCode,
} from "./support/devlog";
import { LOGIN_TELEGRAM_PHONES } from "./support/login-telegram";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import type { SeededMember } from "./support/staff";
import { HOME_HEADING, ORG_HOME_URL } from "./support/shell";

// The login code step in a demo environment that can send login codes over Telegram (decision
// 42). Codes are read from the API's DEVLOG_FILE: the e2e API has a bot but not
// TELEGRAM_MESSAGES=linked, and these members linked no chat, so every code goes to the devlog.
// login.spec.ts covers the screen as production shows it (telegramAvailable false).

test.use({ storageState: { cookies: [], origins: [] } });

const REQUEST_PATH = "/api/auth/otp/request";
const MIN_TOUCH_TARGET_PX = 44;

function memberFor(testInfo: TestInfo): SeededMember {
  const name = testInfo.project.name;
  if (name !== "ipad" && name !== "phone") {
    throw new Error(`No login phone for project ${name}`);
  }
  return LOGIN_TELEGRAM_PHONES[name];
}

function telegramText(phone: string): string {
  return `Jika nomor ${phone} terdaftar, kode 6 angka sudah dikirim lewat Telegram atau WhatsApp.`;
}

function whatsappText(phone: string): string {
  return `Jika nomor ${phone} terdaftar, kode 6 angka sudah dikirim ke WhatsApp-nya.`;
}

function button(page: Page, name: string) {
  return page.getByRole("button", { name, exact: true });
}

async function expectTouchTargets(page: Page): Promise<void> {
  const controls = page
    .getByRole("main")
    .locator("button:visible, input:visible");
  for (const control of await controls.all()) {
    const box = await control.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
  }
}

test("offers Kirim lewat WhatsApp when Telegram is available, then signs in", async ({
  page,
}, testInfo) => {
  const staff = memberFor(testInfo);
  const outsideRequests = trackOutsideRequests(page);
  const sent: LoginCodeRequest[] = [];
  // The type of each real answer's telegramAvailable, checked in the test body.
  const answeredTypes: string[] = [];
  // Holds the next answer back until released, so the pending state can be checked.
  let holdNext: Promise<void> | undefined;

  // The e2e API lacks a demo environment's Telegram setting, so this emulates it for the
  // browser only: every code request still reaches the real API, and its real answer comes
  // back with telegramAvailable set to true. Nothing else in the answer changes.
  await page.route(`**${REQUEST_PATH}`, async (route) => {
    sent.push(route.request().postDataJSON() as LoginCodeRequest);
    const response = await route.fetch();
    if (holdNext !== undefined) await holdNext;
    if (response.status() !== 202) {
      await route.fulfill({ response });
      return;
    }
    const accepted = (await response.json()) as LoginCodeAccepted;
    answeredTypes.push(typeof accepted.telegramAvailable);
    await route.fulfill({
      response,
      json: { ...accepted, telegramAvailable: true },
    });
  });

  await page.goto("/login");
  await page.getByLabel("Nomor WhatsApp").fill(staff.phone);
  await button(page, "Kirim kode").click();
  await expect(
    page.getByRole("heading", { level: 1, name: "Masukkan kode" }),
  ).toBeVisible();

  // The Telegram wording: it names both channels and never the one used.
  await expect(page.getByText(telegramText(staff.phone))).toBeVisible();
  await expect(page.getByLabel("Kode masuk")).toBeFocused();
  await expect(button(page, "Kirim ulang kode")).toBeVisible();
  await expect(button(page, "Kirim lewat WhatsApp")).toBeVisible();
  await expectTouchTargets(page);
  await attachViewportScreenshot(page, testInfo, "login-code-telegram");
  expect(sent).toEqual([{ phone: staff.phone }]);
  // The real API already sends the field (TG-4); only its value is replaced.
  expect(answeredTypes).toEqual(["boolean"]);

  // While the WhatsApp request is pending both buttons are disabled.
  let release: () => void = () => {};
  holdNext = new Promise((resolve) => {
    release = resolve;
  });
  await page.getByLabel("Kode masuk").fill("12");
  const offset = await devlogOffset(devlogFilePath());
  await button(page, "Kirim lewat WhatsApp").click();
  await expect(button(page, "Mengirim lewat WhatsApp…")).toBeDisabled();
  await expect(button(page, "Kirim ulang kode")).toBeDisabled();
  await expect(button(page, "Masuk")).toBeDisabled();
  release();
  holdNext = undefined;

  await expect(page.getByRole("status")).toHaveText(
    "Kode baru dikirim lewat WhatsApp.",
  );
  expect(sent).toEqual([
    { phone: staff.phone },
    { phone: staff.phone, whatsappOnly: true },
  ]);
  expect(answeredTypes).toEqual(["boolean", "boolean"]);
  // That code went over WhatsApp only, so the screen says so.
  await expect(page.getByText(whatsappText(staff.phone))).toBeVisible();
  const codeField = page.getByLabel("Kode dari WhatsApp");
  await expect(codeField).toHaveValue("");
  await expect(codeField).toBeFocused();
  await expect(button(page, "Kirim lewat WhatsApp")).toBeEnabled();
  await expect(button(page, "Kirim ulang kode")).toBeEnabled();

  const code = await waitForLoginCode(devlogFilePath(), offset, staff.phone);
  await codeField.fill(code);
  await button(page, "Masuk").click();
  await expect(page).toHaveURL(ORG_HOME_URL);
  await expect(
    page.getByRole("heading", { level: 1, name: HOME_HEADING }),
  ).toBeVisible();
  expect(outsideRequests).toEqual([]);
});
