import { expect, test, type Page, type TestInfo } from "@playwright/test";

import type { ErrorResponse } from "../src/lib/api/generated/model";

import {
  devlogFilePath,
  devlogOffset,
  waitForLoginCode,
} from "./support/devlog";
import {
  attachFullPageScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { LOGIN_PHONES, type LoginPhones } from "./support/staff";

// The staff login screen against the real Go API: codes are read from the API's DEVLOG_FILE,
// nothing is sent to WhatsApp. Every test starts signed out and uses its own phone numbers.

test.use({ storageState: { cookies: [], origins: [] } });

const LOGIN_WITH_NEXT = "/login?next=%2Forganizations";
const REQUEST_PATH = "/api/auth/otp/request";
const VERIFY_PATH = "/api/auth/otp/verify";
// Minimum body text per project, CLAUDE.md "UX rules".
const MIN_BODY_FONT_PX: Record<string, number> = { ipad: 16, phone: 14 };
const MIN_TOUCH_TARGET_PX = 44;

function phonesFor(testInfo: TestInfo): LoginPhones {
  const name = testInfo.project.name;
  if (name !== "ipad" && name !== "phone") {
    throw new Error(`No login phones for project ${name}`);
  }
  return LOGIN_PHONES[name];
}

function codeSentText(phone: string): string {
  return `Jika nomor ${phone} terdaftar, kode 6 angka sudah dikirim ke WhatsApp-nya.`;
}

function phoneField(page: Page) {
  return page.getByLabel("Nomor WhatsApp");
}

function codeField(page: Page) {
  return page.getByLabel("Kode dari WhatsApp");
}

// Our own alerts only (Next.js has a route announcer with role="alert" outside <main>).
function alerts(page: Page) {
  return page.getByRole("main").getByRole("alert");
}

// Step 1 → step 2 through the UI. Returns the devlog offset taken before the request, so the
// code read afterwards is the one this request produced.
async function requestCode(page: Page, phone: string): Promise<number> {
  const offset = await devlogOffset(devlogFilePath());
  await phoneField(page).fill(phone);
  await page.getByRole("button", { name: "Kirim kode" }).click();
  await expect(
    page.getByRole("heading", { level: 1, name: "Masukkan kode" }),
  ).toBeVisible();
  return offset;
}

async function expectMinFontSize(
  page: Page,
  text: string,
  testInfo: TestInfo,
): Promise<void> {
  const min = MIN_BODY_FONT_PX[testInfo.project.name];
  if (min === undefined) {
    throw new Error(`No font-size rule for project ${testInfo.project.name}`);
  }
  const size = await page
    .getByText(text)
    .evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
  expect(size).toBeGreaterThanOrEqual(min);
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

test("signs in with the WhatsApp code, then signs out", async ({
  page,
}, testInfo) => {
  const staff = phonesFor(testInfo).signIn;
  const outsideRequests = trackOutsideRequests(page);

  await page.goto("/login");
  await expect(
    page.getByRole("heading", { level: 1, name: "Masuk" }),
  ).toBeVisible();
  await expect(
    page.getByText("Penyewa tidak perlu masuk.", { exact: false }),
  ).toBeVisible();
  await expectMinFontSize(
    page,
    "Untuk Supervisor, admin, dan karyawan.",
    testInfo,
  );
  await expectTouchTargets(page);
  await attachFullPageScreenshot(page, testInfo, "login-phone-step");

  const offset = await requestCode(page, staff.phone);
  await expect(page.getByText(codeSentText(staff.phone))).toBeVisible();
  await expect(codeField(page)).toBeFocused();
  await expect(page.getByText("Kode berlaku 5 menit.")).toBeVisible();
  await expectTouchTargets(page);
  await attachFullPageScreenshot(page, testInfo, "login-code-step");

  const code = await waitForLoginCode(devlogFilePath(), offset, staff.phone);
  await codeField(page).fill(code);
  await page.getByRole("button", { name: "Masuk", exact: true }).click();

  await expect(page).toHaveURL("/organizations");
  await expect(page.getByText(`Masuk sebagai ${staff.name}`)).toBeVisible();

  await page.getByRole("button", { name: "Keluar" }).click();
  await expect(page).toHaveURL("/login");
  await expect(
    page.getByRole("heading", { level: 1, name: "Masuk" }),
  ).toBeVisible();

  // The session is gone on the API and the cookie with it.
  await page.goto("/organizations");
  await expect(page).toHaveURL(LOGIN_WITH_NEXT);
  expect(outsideRequests).toEqual([]);
});

test("returns to the next path after login, submitting with Enter", async ({
  page,
}, testInfo) => {
  const staff = phonesFor(testInfo).signInWithNext;
  const outsideRequests = trackOutsideRequests(page);
  const next = "/organizations?asal=wa";

  await page.goto(`/login?next=${encodeURIComponent(next)}`);
  const offset = await devlogOffset(devlogFilePath());
  await phoneField(page).fill(staff.phone);
  await phoneField(page).press("Enter");
  await expect(codeField(page)).toBeFocused();

  const code = await waitForLoginCode(devlogFilePath(), offset, staff.phone);
  await codeField(page).fill(code);
  await codeField(page).press("Enter");

  await expect(page).toHaveURL(next);
  await expect(page.getByText(`Masuk sebagai ${staff.name}`)).toBeVisible();
  expect(outsideRequests).toEqual([]);
});

test("checks the phone field before and after asking the API", async ({
  page,
}) => {
  const outsideRequests = trackOutsideRequests(page);
  const codeRequests: string[] = [];
  page.on("request", (request) => {
    if (new URL(request.url()).pathname === REQUEST_PATH) {
      codeRequests.push(request.url());
    }
  });

  await page.goto("/login");
  await page.getByRole("button", { name: "Kirim kode" }).click();
  await expect(alerts(page)).toHaveText("Isi nomor WhatsApp Anda.");
  await expect(phoneField(page)).toHaveAttribute("aria-invalid", "true");
  await expect(phoneField(page)).toBeFocused();
  expect(codeRequests).toEqual([]);

  // Not a number the API accepts: its own message, under the field.
  await phoneField(page).fill("12345");
  await page.getByRole("button", { name: "Kirim kode" }).click();
  await expect(alerts(page)).toHaveText("Nomor WhatsApp tidak valid.");
  await expect(phoneField(page)).toHaveAttribute("aria-invalid", "true");
  const errorId = await alerts(page).getAttribute("id");
  await expect(phoneField(page)).toHaveAttribute(
    "aria-describedby",
    new RegExp(`(^| )${errorId}( |$)`),
  );
  expect(codeRequests).toHaveLength(1);
  expect(outsideRequests).toEqual([]);
});

test("a wrong code shows the API's error under the field; a new code works", async ({
  page,
}, testInfo) => {
  const staff = phonesFor(testInfo).wrongCode;
  const outsideRequests = trackOutsideRequests(page);
  const verifyRequests: string[] = [];
  page.on("request", (request) => {
    if (new URL(request.url()).pathname === VERIFY_PATH) {
      verifyRequests.push(request.url());
    }
  });

  await page.goto("/login");
  const offset = await requestCode(page, staff.phone);
  const code = await waitForLoginCode(devlogFilePath(), offset, staff.phone);

  // Digits only, at most 6, and a short code is caught before any request.
  await codeField(page).fill("12a");
  await expect(codeField(page)).toHaveValue("12");
  await page.getByRole("button", { name: "Masuk", exact: true }).click();
  await expect(alerts(page)).toHaveText("Kode harus 6 angka.");
  expect(verifyRequests).toEqual([]);

  const wrongCode = `${(Number(code[0]) + 1) % 10}${code.slice(1)}`;
  await codeField(page).fill(wrongCode);
  await page.getByRole("button", { name: "Masuk", exact: true }).click();

  const error = alerts(page);
  await expect(error).toHaveText("Kode salah atau sudah kedaluwarsa.");
  await expect(codeField(page)).toHaveAttribute("aria-invalid", "true");
  await expect(codeField(page)).toHaveAttribute(
    "aria-describedby",
    new RegExp(`(^| )${await error.getAttribute("id")}( |$)`),
  );
  await expect(codeField(page)).toBeFocused();
  expect(verifyRequests).toHaveLength(1);
  await attachFullPageScreenshot(page, testInfo, "login-wrong-code");

  // A new code cancels the old one.
  const resendOffset = await devlogOffset(devlogFilePath());
  await page.getByRole("button", { name: "Kirim ulang kode" }).click();
  await expect(page.getByRole("status")).toHaveText("Kode baru sudah dikirim.");
  await expect(codeField(page)).toHaveValue("");
  await expect(codeField(page)).not.toHaveAttribute("aria-invalid", "true");

  const newCode = await waitForLoginCode(
    devlogFilePath(),
    resendOffset,
    staff.phone,
  );
  await codeField(page).fill(newCode);
  await page.getByRole("button", { name: "Masuk", exact: true }).click();
  await expect(page).toHaveURL("/organizations");
  await expect(page.getByText(`Masuk sebagai ${staff.name}`)).toBeVisible();
  expect(outsideRequests).toEqual([]);
});

test("too many code requests show the API's message in the alert", async ({
  page,
}, testInfo) => {
  const phone = phonesFor(testInfo).rateLimited;
  const outsideRequests = trackOutsideRequests(page);

  await page.goto("/login");
  await phoneField(page).fill(phone);

  // The limit is 3 requests per phone per 15 minutes, but a rerun may start at the limit, so
  // ask until the API says no instead of counting.
  let limited: ErrorResponse | undefined;
  for (let attempt = 0; attempt < 4 && limited === undefined; attempt++) {
    const response = page.waitForResponse(
      (r) => new URL(r.url()).pathname === REQUEST_PATH,
    );
    await page.getByRole("button", { name: "Kirim kode" }).click();
    const answered = await response;
    if (answered.status() === 429) {
      limited = (await answered.json()) as ErrorResponse;
      break;
    }
    expect(answered.status()).toBe(202);
    await page.getByRole("button", { name: "Ganti nomor" }).click();
    // Back on step 1 with the number kept and focused.
    await expect(phoneField(page)).toHaveValue(phone);
    await expect(phoneField(page)).toBeFocused();
  }

  expect(limited?.error.code).toBe("rate_limited");
  await expect(alerts(page)).toHaveText(limited?.error.message ?? "");
  await expect(
    page.getByRole("heading", { level: 1, name: "Masuk" }),
  ).toBeVisible();
  await expect(phoneField(page)).not.toHaveAttribute("aria-invalid", "true");
  await attachFullPageScreenshot(page, testInfo, "login-rate-limit");
  expect(outsideRequests).toEqual([]);
});

test("an unregistered number gets the same code step", async ({
  page,
}, testInfo) => {
  const phone = phonesFor(testInfo).unregistered;
  const outsideRequests = trackOutsideRequests(page);

  await page.goto("/login");
  await requestCode(page, phone);

  await expect(page.getByText(codeSentText(phone))).toBeVisible();
  await expect(codeField(page)).toBeFocused();
  await expect(alerts(page)).toHaveCount(0);
  expect(outsideRequests).toEqual([]);
});
