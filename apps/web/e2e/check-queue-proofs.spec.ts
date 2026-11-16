import {
  expect,
  test,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";

import {
  NOTE_EMPTY_MESSAGE,
  NOTE_LABEL,
  NO_RECEIPT_TEXT,
  PAYEE_ENDED_TAG,
  PAYEE_INTRO,
  PAYEE_SEARCH_LABEL,
  PAYEE_SENDER_LABEL,
  PAYEE_TITLE,
  PAYEE_USE,
  PAYER_CHANGE,
  PAYER_LABEL,
  PREVIEW_LEAD,
  PROOF_AMOUNT_LABEL,
  PROOF_CALLOUT,
  PROOF_NOTE_LABEL,
  PROOF_REJECT,
  PROOF_REJECT_SUBMIT,
  PROOF_REJECT_TITLE,
  PROOF_SUBMIT,
  PROOF_TITLE,
  shortName,
  UPLOAD_SUBMIT,
  UPLOAD_TITLE,
  ZOOM_LABEL,
  ZOOM_TITLE,
} from "../src/components/client/check-queue/utils";
import type {
  CheckQueueItem,
  CheckQueuePage,
  CheckResolution,
  MatchTransferProofResult,
  Tenant,
  TenantAccount,
  TenantList,
  TenantPaymentPreview,
  Upload,
  UploadInput,
  UploadTicket,
} from "../src/lib/api/generated/model";
import { formatMonthYear, jakartaToday } from "../src/lib/format/date";
import { formatRupiah } from "../src/lib/format/rupiah";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { call, runId } from "./support/payment-page";
import {
  clearTenantProofs,
  ENDED_LEASE_TENANT,
  infectedPhoto,
  INFECTED_TENANTS,
  linkToken,
  MATCH_TENANT,
  NOT_PAYMENT_TENANTS,
  OTHER_PAYER_PAYEES,
  OTHER_PAYER_SENDERS,
  sendProof,
  tenantFor,
  transferPhoto,
  waitForCleanProof,
  waitForInfectedProof,
} from "./support/proof-fixtures";
import { minBodyFontPx, projectName } from "./support/projects";
import { ORG_HOME_URL } from "./support/shell";
import { DIMAS } from "./support/staff";
import { expectToast } from "./support/toast";

// "Antrean cek", Bukti transfer (board Admin-Cek-Bukti), against the real API, its object storage
// (RustFS) and the worker's fake scanner. A seeded tenant of Kos Bulanan Sejahtera sends a real
// JPEG from a payment link with no session; Pak Dimas, Admin of the whole organization
// (payment:check and payment:record), answers it. Each test and size project has its own tenant
// (e2e/support/proof-fixtures.ts) and first takes that tenant's leftover proofs off the queue.
// Only the iPad records money.

test.use({ storageState: DIMAS.storageState });

function orgIdOf(page: Page): string {
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");
  return orgId;
}

// Pak Dimas's one organization, Kos Bulanan Sejahtera: the picker goes straight in.
async function openBulanan(page: Page): Promise<string> {
  await page.goto("/organizations");
  await expect(page).toHaveURL(ORG_HOME_URL);
  return orgIdOf(page);
}

async function tenantNamed(
  page: Page,
  orgId: string,
  name: string,
): Promise<Tenant> {
  const tenants = await call<TenantList>(
    page.request,
    "GET",
    `/orgs/${orgId}/tenants?q=${encodeURIComponent(name)}`,
    200,
  );
  const tenant = tenants.items.find((each) => each.name === name);
  if (tenant === undefined) throw new Error(`${name} not seeded`);
  return tenant;
}

function account(page: Page, orgId: string, tenantId: string) {
  return call<TenantAccount>(
    page.request,
    "GET",
    `/orgs/${orgId}/tenants/${tenantId}/account`,
    200,
  );
}

// The tenant's proof, sent from a fresh link, as the queue lists it once the scan has run.
async function sentProof(
  page: Page,
  testInfo: TestInfo,
  orgId: string,
  tenant: Tenant,
  infected: boolean,
) {
  const owed = await account(page, orgId, tenant.id);
  const link = await linkToken(page.request, orgId, owed);
  await clearTenantProofs(page.request, orgId, tenant.id);
  const photo = await transferPhoto(page, formatRupiah(Number(`1${runId()}0`)));
  const proof = await sendProof(
    testInfo,
    link.token,
    infected ? infectedPhoto(photo) : photo,
  );
  const item = infected
    ? await waitForInfectedProof(page.request, orgId, tenant.id, proof.id)
    : await waitForCleanProof(page.request, orgId, proof.id);
  const invoice = owed.invoices.find((each) => each.id === link.invoiceId);
  if (invoice === undefined) throw new Error("no invoice");
  return { proof, item, invoice };
}

// A proof a member sends for the tenant: the photo goes to storage like the karyawan's app sends
// it, and completeUpload puts it on the queue with the member as its sender. Waits until the scan
// found it clean.
async function sentByMember(
  page: Page,
  orgId: string,
  tenantId: string,
): Promise<CheckQueueItem> {
  const photo = await transferPhoto(page, formatRupiah(Number(`2${runId()}0`)));
  const input: UploadInput = {
    purpose: "transfer_proof",
    tenantId,
    contentType: "image/jpeg",
    size: photo.length,
  };
  const ticket = await call<UploadTicket>(
    page.request,
    "POST",
    `/orgs/${orgId}/uploads`,
    201,
    input,
  );
  const put = await page.request.fetch(ticket.putUrl, {
    method: "PUT",
    headers: ticket.putHeaders,
    data: photo,
  });
  expect(put.status(), "PUT to storage").toBe(200);
  await call<Upload>(
    page.request,
    "POST",
    `/orgs/${orgId}/uploads/${ticket.upload.id}/complete`,
    200,
  );
  let found: CheckQueueItem | undefined;
  await expect
    .poll(
      async () => {
        const queue = await call<CheckQueuePage>(
          page.request,
          "GET",
          `/orgs/${orgId}/check-queue?kind=transfer_proof&pageSize=100`,
          200,
        );
        found = queue.items.find(
          (item) => item.transferProof?.uploadId === ticket.upload.id,
        );
        return found?.transferProof?.uploadStatus;
      },
      { timeout: 30_000 },
    )
    .toBe("clean");
  if (found === undefined) throw new Error("the member's proof is not queued");
  return found;
}

function rows(page: Page): Locator {
  return page
    .getByRole("list", { name: "Item yang perlu dicek" })
    .getByRole("listitem");
}

async function fontSize(target: Locator): Promise<number> {
  return target
    .first()
    .evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

async function expectTall(target: Locator, min: number) {
  const box = await target.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(min);
}

async function expectLoaded(image: Locator) {
  await expect(image).toBeVisible();
  await expect
    .poll(() =>
      image.evaluate(
        (el) =>
          el instanceof HTMLImageElement && el.complete && el.naturalWidth > 0,
      ),
    )
    .toBe(true);
}

// A label that fits its button on one line.
async function expectOneLine(button: Locator) {
  expect(
    await button.evaluate((el) => el.scrollWidth <= el.clientWidth + 1),
  ).toBe(true);
}

// Opens the tenant's one waiting row of a kind, after checking it shows `shows` (the list is
// hidden from assistive technology while the drawer is open).
async function openRow(
  page: Page,
  orgId: string,
  kind: string,
  text: string,
  shows: readonly string[],
) {
  await page.goto(`/o/${orgId}/billing/check-queue?kind=${kind}`);
  const row = rows(page).filter({ hasText: text });
  await expect(row).toHaveCount(1);
  for (const part of shows) await expect(row).toContainText(part);
  await expectTall(row.getByRole("button"), 76);
  await row.getByRole("button").click();
  return row;
}

test("matches a proof to a transfer payment", async ({ page }, testInfo) => {
  test.skip(
    projectName(testInfo) !== "ipad",
    "One tenant's money is shared by every project; only the iPad records it.",
  );
  const outsideRequests = trackOutsideRequests(page);
  const orgId = await openBulanan(page);
  const tenant = await tenantNamed(page, orgId, MATCH_TENANT);
  const { proof, invoice } = await sentProof(
    page,
    testInfo,
    orgId,
    tenant,
    false,
  );

  const row = await openRow(page, orgId, "transfer_proof", MATCH_TENANT, [
    "Siap dicek",
  ]);
  const sheet = page.getByRole("dialog", { name: PROOF_TITLE });
  await expect(sheet).toBeVisible();
  await expect(sheet).toContainText(MATCH_TENANT);

  // The photo, when it was sent and from which invoice's link.
  await expectLoaded(
    sheet.getByRole("img", { name: `${ZOOM_TITLE} dari ${MATCH_TENANT}` }),
  );
  await expect(sheet).toContainText("Dikirim penyewa");
  await expect(sheet).toContainText(/\S+ \d+ \w+, \d\d\.\d\d/);
  if (invoice.kind === "regular") {
    await expect(sheet).toContainText(
      `Dari tautan tagihan${formatMonthYear(invoice.period)}`,
    );
  }
  await expect(sheet.getByText(PROOF_CALLOUT)).toBeVisible();
  await expect(sheet).toContainText("Pembayaran dari");

  // "Perbesar foto" shows it large; Escape and "Tutup" close it and leave the drawer open.
  const zoom = sheet.getByRole("button", { name: ZOOM_LABEL });
  await expectTall(zoom, 44);
  await zoom.click();
  const large = page.getByRole("dialog", { name: ZOOM_TITLE });
  // An Escape the moment the large view opens, before it has taken focus, never closes the
  // drawer behind it (it used to, under load). It may or may not close the large view yet;
  // once focus is inside, Escape closes it.
  await page.keyboard.press("Escape");
  // Exact: the large view's name, "Foto bukti transfer", holds the drawer's.
  await expect(
    page.getByRole("dialog", { name: "Bukti transfer", exact: true }),
  ).toBeVisible();
  if ((await large.count()) > 0) {
    await expectLoaded(large.getByRole("img"));
    await expect
      .poll(() =>
        large.evaluate((dialog) => dialog.contains(document.activeElement)),
      )
      .toBe(true);
    await page.keyboard.press("Escape");
  }
  await expect(large).toHaveCount(0);
  await expect(sheet).toBeVisible();
  await zoom.click();
  await expect(large).toBeVisible();
  await large.getByRole("button", { name: "Tutup" }).click();
  await expect(large).toHaveCount(0);
  await expect(sheet).toBeVisible();

  // The amount is empty when the drawer opens; the API previews what the typed one pays.
  const amountField = sheet.getByLabel(PROOF_AMOUNT_LABEL);
  await expect(amountField).toHaveValue("");
  const amount = 5_000 + Number(runId());
  const previewAnswer = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return (
      url.pathname.endsWith(`/tenants/${tenant.id}/payment-preview`) &&
      url.searchParams.get("amount") === String(amount)
    );
  });
  await amountField.fill(String(amount));
  await expect(amountField).toHaveValue(formatRupiah(amount).slice(2));
  const previewResponse = await previewAnswer;
  expect(previewResponse.status()).toBe(200);
  const preview = (await previewResponse.json()) as TenantPaymentPreview;
  await expect(sheet.getByText(PREVIEW_LEAD)).toBeVisible();
  for (const item of preview.items) {
    await expect(sheet).toContainText(
      item.paidInFull
        ? "Lunas"
        : `Sebagian, sisa ${formatRupiah(item.balanceAfter)}`,
    );
  }
  if (preview.creditAdded > 0) {
    await expect(sheet).toContainText(
      `Kelebihan ${formatRupiah(preview.creditAdded)} jadi saldo`,
    );
  }
  await expect(sheet.getByText(NO_RECEIPT_TEXT)).toBeVisible();
  const note = `Masuk ke BCA usaha, run ${runId()}`;
  await sheet.getByLabel(PROOF_NOTE_LABEL).fill(note);
  await attachViewportScreenshot(page, testInfo, "check-queue-proof");

  // One press records it once, for the proof's own tenant.
  const save = sheet.getByRole("button", { name: PROOF_SUBMIT });
  await expectTall(save, 44);
  const answer = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname.endsWith(
        `/check-queue/transfer_proof/${proof.id}/match`,
      ),
  );
  await save.click();
  const response = await answer;
  expect(response.status(), await response.text()).toBe(201);
  const body = response.request().postDataJSON() as Record<string, unknown>;
  expect(body).toEqual({
    requestId: expect.stringMatching(/^[0-9a-f-]{36}$/),
    amount,
    receivedOn: jakartaToday(new Date()),
    note,
  });
  const result = (await response.json()) as MatchTransferProofResult;
  expect(result.resolution.outcome).toBe("matched");

  await expectToast(
    page,
    "Pembayaran disimpan",
    "create",
    `${formatRupiah(amount)} dari ${MATCH_TENANT}`,
  );
  await expect(sheet).toHaveCount(0);
  await expect(row).toHaveCount(0);

  // The tenant has the transfer, and the proof cannot be matched again.
  const after = await account(page, orgId, tenant.id);
  expect(
    after.payments.find((each) => each.id === result.payment.payment.id),
  ).toMatchObject({ method: "transfer", amount });
  const again = await page.request.post(
    `/api/orgs/${orgId}/check-queue/transfer_proof/${proof.id}/match`,
    {
      data: {
        requestId: crypto.randomUUID(),
        amount,
        receivedOn: jakartaToday(new Date()),
      },
    },
  );
  expect(again.status()).toBe(409);
  expect(await again.json()).toMatchObject({
    error: { code: "check_item_resolved" },
  });
  expect(outsideRequests).toEqual([]);
});

test("marks a proof not a payment, with a note, and names the member who sent one", async ({
  page,
}, testInfo) => {
  const outsideRequests = trackOutsideRequests(page);
  const orgId = await openBulanan(page);
  const name = tenantFor(NOT_PAYMENT_TENANTS, testInfo);
  const tenant = await tenantNamed(page, orgId, name);
  const { proof } = await sentProof(page, testInfo, orgId, tenant, false);

  const row = await openRow(page, orgId, "transfer_proof", name, [
    "Foto bukti dari halaman pembayaran",
  ]);
  const sheet = page.getByRole("dialog", { name: PROOF_TITLE });
  await expect(sheet).toBeVisible();
  await expectLoaded(
    sheet.getByRole("img", { name: `${ZOOM_TITLE} dari ${name}` }),
  );
  await expect(sheet.getByLabel(PROOF_AMOUNT_LABEL)).toHaveValue("");
  expect(await fontSize(sheet.getByText(PROOF_CALLOUT))).toBeGreaterThanOrEqual(
    minBodyFontPx(testInfo),
  );
  const notPayment = sheet.getByRole("button", { name: PROOF_REJECT });
  const save = sheet.getByRole("button", { name: PROOF_SUBMIT });
  for (const button of [notPayment, save]) {
    await expectTall(button, 44);
    await expectOneLine(button);
  }
  await attachViewportScreenshot(page, testInfo, "check-queue-proof-open");

  // "Bukan pembayaran" turns the drawer into the note; "Kembali" goes back.
  await notPayment.click();
  const reject = page.getByRole("dialog", { name: PROOF_REJECT_TITLE });
  await expect(reject).toBeVisible();
  await reject.getByRole("button", { name: "Kembali" }).click();
  await expect(page.getByRole("dialog", { name: PROOF_TITLE })).toBeVisible();
  await page
    .getByRole("dialog", { name: PROOF_TITLE })
    .getByRole("button", { name: PROOF_REJECT })
    .click();
  await expect(reject).toBeVisible();

  // The note is required.
  const submit = reject.getByRole("button", { name: PROOF_REJECT_SUBMIT });
  for (const button of [
    reject.getByRole("button", { name: "Kembali" }),
    submit,
  ]) {
    await expectTall(button, 44);
    await expectOneLine(button);
  }
  await submit.click();
  await expect(reject.getByText(NOTE_EMPTY_MESSAGE)).toBeVisible();
  await attachViewportScreenshot(page, testInfo, "check-queue-proof-reject");

  const note = `Foto struk belanja, run ${runId()}`;
  await reject.getByLabel(NOTE_LABEL).fill(note);
  const answer = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname.endsWith(
        `/check-queue/transfer_proof/${proof.id}/dismiss`,
      ),
  );
  await submit.click();
  const response = await answer;
  expect(response.status(), await response.text()).toBe(200);
  expect(response.request().postDataJSON()).toEqual({ note });
  expect(((await response.json()) as CheckResolution).outcome).toBe(
    "dismissed",
  );
  await expectToast(page, "Ditandai bukan pembayaran", "neutral");
  await expect(reject).toHaveCount(0);
  await expect(row).toHaveCount(0);

  // A proof a member sent for the tenant (decision 49, board Admin-Cek-Bukti-Staf): the row and
  // the drawer say who sent it, and name no invoice link. Pak Dimas sends it through the API, as
  // the karyawan's app does (createUpload transfer_proof, the PUT, completeUpload).
  const staffProof = await sentByMember(page, orgId, tenant.id);
  expect(staffProof.transferProof?.sentBy?.name).toBe(DIMAS.name);
  await openRow(page, orgId, "transfer_proof", name, [
    `Dikirim oleh ${DIMAS.name}`,
  ]);
  const staffSheet = page.getByRole("dialog", { name: PROOF_TITLE });
  await expect(staffSheet).toBeVisible();
  await expectLoaded(
    staffSheet.getByRole("img", { name: `${ZOOM_TITLE} dari ${name}` }),
  );
  await expect(
    staffSheet.getByText("Dikirim oleh", { exact: true }),
  ).toBeVisible();
  await expect(staffSheet.getByText(DIMAS.name, { exact: true })).toBeVisible();
  await expect(staffSheet.getByText("Masuk", { exact: true })).toBeVisible();
  await expect(staffSheet.getByText("Dikirim penyewa")).toHaveCount(0);
  await expect(staffSheet.getByText("Dari tautan tagihan")).toHaveCount(0);
  await attachViewportScreenshot(page, testInfo, "check-queue-proof-sent-by");
  await clearTenantProofs(page.request, orgId, tenant.id);
  expect(outsideRequests).toEqual([]);
});

test("lists an infected proof as a refused file", async ({
  page,
}, testInfo) => {
  const outsideRequests = trackOutsideRequests(page);
  const orgId = await openBulanan(page);
  const name = tenantFor(INFECTED_TENANTS, testInfo);
  const tenant = await tenantNamed(page, orgId, name);
  const { item } = await sentProof(page, testInfo, orgId, tenant, true);

  const row = await openRow(
    page,
    orgId,
    "upload_infected",
    `Bukti transfer ${name}`,
    ["Dikirim penyewa", "Berbahaya"],
  );
  const sheet = page.getByRole("dialog", { name: UPLOAD_TITLE });
  await expect(sheet).toBeVisible();
  await expect(sheet).toContainText("File tidak bisa dibuka");
  await expect(sheet).toContainText(
    "Minta penyewa mengirim foto lagi dari halaman pembayaran.",
  );
  // The file itself is never shown.
  await expect(sheet.getByRole("img")).toHaveCount(0);

  const submit = sheet.getByRole("button", { name: UPLOAD_SUBMIT });
  await submit.click();
  await expect(sheet.getByText(NOTE_EMPTY_MESSAGE)).toBeVisible();
  const note = `Sudah minta foto ulang, run ${runId()}`;
  await sheet.getByLabel(NOTE_LABEL).fill(note);
  const answer = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname.endsWith(
        `/check-queue/upload_infected/${item.id}/dismiss`,
      ),
  );
  await submit.click();
  expect((await answer).status()).toBe(200);
  await expectToast(page, "Ditandai sudah ditangani", "create");
  await expect(sheet).toHaveCount(0);
  await expect(row).toHaveCount(0);
  expect(outsideRequests).toEqual([]);
});

// "Ganti" (boards Admin-Cek-Bukti-Ganti and Admin-Cek-Bukti-Lain): the proof one tenant sent
// becomes another tenant's transfer (a relative paid for them). Each project has its own sender
// and payee, so the money of one project never meets another's.
test("records a proof as another tenant's payment", async ({
  page,
}, testInfo) => {
  const outsideRequests = trackOutsideRequests(page);
  const orgId = await openBulanan(page);
  const senderName = tenantFor(OTHER_PAYER_SENDERS, testInfo);
  const payeeName = tenantFor(OTHER_PAYER_PAYEES, testInfo);
  const sender = await tenantNamed(page, orgId, senderName);
  const payee = await tenantNamed(page, orgId, payeeName);
  await clearTenantProofs(page.request, orgId, payee.id);
  const { proof } = await sentProof(page, testInfo, orgId, sender, false);
  const senderBefore = await account(page, orgId, sender.id);

  await openRow(page, orgId, "transfer_proof", senderName, ["Siap dicek"]);
  const sheet = page.getByRole("dialog", { name: PROOF_TITLE, exact: true });
  await expect(sheet).toBeVisible();
  await expectLoaded(
    sheet.getByRole("img", { name: `${ZOOM_TITLE} dari ${senderName}` }),
  );
  const payer = sheet.getByRole("region", { name: PAYER_LABEL });
  await expect(payer).toContainText(senderName);
  const notice = `Bukan pengirim bukti (${senderName}). Tagihan ${shortName(senderName)} tidak berubah.`;
  await expect(sheet.getByText(notice)).toHaveCount(0);

  // The amount, day and note typed first stay through the picker.
  const amount = 7_000 + Number(runId());
  const amountField = sheet.getByLabel(PROOF_AMOUNT_LABEL);
  await amountField.fill(String(amount));
  const typedAmount = formatRupiah(amount).slice(2);
  await expect(amountField).toHaveValue(typedAmount);
  const note = `Dibayar keluarga, run ${runId()}`;
  await sheet.getByLabel(PROOF_NOTE_LABEL).fill(note);

  const change = payer.getByRole("button", { name: PAYER_CHANGE });
  await expectTall(change, 44);
  await change.click();
  const picker = page.getByRole("dialog", { name: PAYEE_TITLE });
  await expect(picker).toBeVisible();
  await expect(picker).toContainText(`Bukti dari ${senderName}`);
  await expect(picker.getByText(PAYEE_INTRO)).toBeVisible();
  // The sender is always first, and chosen.
  const senderGroup = picker.getByRole("group", { name: PAYEE_SENDER_LABEL });
  await expect(
    senderGroup.getByRole("radio", { name: new RegExp(senderName) }),
  ).toBeChecked();
  const search = picker.getByLabel(PAYEE_SEARCH_LABEL);
  await expectTall(search, 44);

  // A tenant whose lease ended can be found and chosen: they may still owe.
  await search.fill(ENDED_LEASE_TENANT);
  const ended = picker.locator("label").filter({ hasText: ENDED_LEASE_TENANT });
  await expect(ended).toHaveCount(1);
  await expect(ended).toContainText(PAYEE_ENDED_TAG);
  await expectTall(ended, 44);
  await expect(picker).toContainText(/\d+ penyewa cocok/i);
  expect(
    await fontSize(ended.getByText(ENDED_LEASE_TENANT)),
  ).toBeGreaterThanOrEqual(minBodyFontPx(testInfo));
  await ended.click();
  await expect(
    picker.getByRole("radio", { name: new RegExp(ENDED_LEASE_TENANT) }),
  ).toBeChecked();
  await attachViewportScreenshot(page, testInfo, "check-queue-proof-payee");

  // "Kembali" changes nothing: still the sender, the amount as typed, no notice.
  await picker.getByRole("button", { name: "Kembali" }).click();
  await expect(sheet).toBeVisible();
  await expect(payer).toContainText(senderName);
  await expect(payer).not.toContainText(ENDED_LEASE_TENANT);
  await expect(sheet.getByText(notice)).toHaveCount(0);
  await expect(amountField).toHaveValue(typedAmount);

  // Choosing the payee: the frame turns orange with the line about the sender, and the preview is
  // the payee's.
  async function choose(name: string, waitForPreview: boolean) {
    await payer.getByRole("button", { name: PAYER_CHANGE }).click();
    await expect(picker).toBeVisible();
    await picker.getByLabel(PAYEE_SEARCH_LABEL).fill(name);
    const option = picker.getByRole("radio", { name: new RegExp(name) });
    await expect(option).toHaveCount(1);
    await option.click();
    await expect(option).toBeChecked();
    const use = picker.getByRole("button", { name: PAYEE_USE });
    await expectTall(use, 44);
    const previewAnswer = waitForPreview
      ? page.waitForResponse((response) => {
          const url = new URL(response.url());
          return (
            url.pathname.endsWith(`/tenants/${payee.id}/payment-preview`) &&
            url.searchParams.get("amount") === String(amount)
          );
        })
      : null;
    await use.click();
    await expect(sheet).toBeVisible();
    if (previewAnswer !== null)
      expect((await previewAnswer).status()).toBe(200);
  }

  // Choosing a payee only chooses: no payment goes out until "Simpan pembayaran".
  const matchPosts: string[] = [];
  page.on("request", (request) => {
    if (
      request.method() === "POST" &&
      new URL(request.url()).pathname.endsWith(
        `/check-queue/transfer_proof/${proof.id}/match`,
      )
    )
      matchPosts.push(request.url());
  });

  await choose(payeeName, true);
  await expect(payer).toContainText(payeeName);
  const noticeLine = sheet.getByText(notice);
  await expect(noticeLine).toBeVisible();
  expect(await fontSize(noticeLine)).toBeGreaterThanOrEqual(
    minBodyFontPx(testInfo),
  );
  const frame = payer.locator("div").first();
  expect(
    await frame.evaluate((el) => getComputedStyle(el).borderTopColor),
  ).toBe(await noticeLine.evaluate((el) => getComputedStyle(el).color));
  await expect(amountField).toHaveValue(typedAmount);
  await expect(sheet.getByLabel(PROOF_NOTE_LABEL)).toHaveValue(note);
  await expect(sheet.getByText(PREVIEW_LEAD)).toBeVisible();
  await attachViewportScreenshot(page, testInfo, "check-queue-proof-other");

  // Choosing the sender again takes the notice away; then the payee once more.
  await payer.getByRole("button", { name: PAYER_CHANGE }).click();
  await picker
    .getByRole("group", { name: PAYEE_SENDER_LABEL })
    .getByRole("radio")
    .click();
  await picker.getByRole("button", { name: PAYEE_USE }).click();
  await expect(payer).toContainText(senderName);
  await expect(sheet.getByText(notice)).toHaveCount(0);
  await choose(payeeName, false);
  await expect(sheet.getByText(notice)).toBeVisible();

  expect(matchPosts).toEqual([]);

  // "Simpan pembayaran" records the transfer for the payee, once.
  const answer = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname.endsWith(
        `/check-queue/transfer_proof/${proof.id}/match`,
      ),
  );
  await sheet.getByRole("button", { name: PROOF_SUBMIT }).click();
  const response = await answer;
  expect(response.status(), await response.text()).toBe(201);
  expect(response.request().postDataJSON()).toEqual({
    requestId: expect.stringMatching(/^[0-9a-f-]{36}$/),
    tenantId: payee.id,
    amount,
    receivedOn: jakartaToday(new Date()),
    note,
  });
  const result = (await response.json()) as MatchTransferProofResult;
  expect(result.resolution.outcome).toBe("matched");
  await expectToast(
    page,
    "Pembayaran disimpan",
    "create",
    `${formatRupiah(amount)} dari ${payeeName}`,
  );
  await expect(sheet).toHaveCount(0);

  // The payee has the transfer; the sender's account did not change.
  const payeeAfter = await account(page, orgId, payee.id);
  expect(
    payeeAfter.payments.find((each) => each.id === result.payment.payment.id),
  ).toMatchObject({ method: "transfer", amount });
  const senderAfter = await account(page, orgId, sender.id);
  expect(
    senderAfter.payments.find((each) => each.id === result.payment.payment.id),
  ).toBeUndefined();
  expect(senderAfter.outstanding).toBe(senderBefore.outstanding);
  expect(senderAfter.payments).toHaveLength(senderBefore.payments.length);
  expect(outsideRequests).toEqual([]);
});
