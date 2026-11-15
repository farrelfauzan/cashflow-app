import {
  expect,
  request as playwrightRequest,
  type APIRequestContext,
  type Page,
  type TestInfo,
} from "@playwright/test";

import type {
  CheckQueueItem,
  CheckQueuePage,
  CheckResolution,
  PaymentProof,
  TenantAccount,
} from "../../src/lib/api/generated/model";
import { call, makeLink, tenantClientAddress } from "./payment-page";
import { projectName, type ProjectName } from "./projects";

// Transfer proofs for e2e/check-queue-proofs.spec.ts ("Antrean cek", Bukti transfer): a tenant
// sends a photo from their payment link (sendPaymentProof, no session), the worker scans it with
// the fake scanner (UPLOAD_SCANNER=fake: clean unless the file holds the EICAR test string), and
// Pak Dimas answers it in Kos Bulanan Sejahtera.
//
// Seeded tenants of Kos Bulanan Sejahtera that no other spec uses, one per test and size project,
// so the projects running side by side never see each other's proofs and each test can take its
// tenant's leftover proofs off the queue first. Only LUKMAN's proof becomes a payment (on the
// iPad only); the others are answered without money.
export const MATCH_TENANT = "Lukman Hakim";

export const NOT_PAYMENT_TENANTS: Record<ProjectName, string> = {
  ipad: "Andi Pratama",
  phone: "Bagas Aditya",
  desktop: "Cahyo Wibowo",
};

export const INFECTED_TENANTS: Record<ProjectName, string> = {
  ipad: "Dian Permata Sari",
  phone: "Eka Putri Ramadhani",
  desktop: "Farhan Maulana",
};

export function tenantFor(
  tenants: Record<ProjectName, string>,
  testInfo: TestInfo,
): string {
  return tenants[projectName(testInfo)];
}

// The anti-malware test string every scanner reports, assembled here so this source file never
// holds it in one piece.
const EICAR = [
  "X5O!P%@AP[4\\PZX54(P^)7CC)7}$",
  "EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*",
].join("");

const STORAGE_HINT =
  "The e2e job must start RustFS (infra/compose.dev.yaml, profile uploads) and give the API and the worker STORAGE_* and UPLOAD_SCANNER=fake (.github/workflows/web.yml).";

// A real JPEG of a bank app's "Transfer berhasil" screen, drawn by the browser (no file is
// committed). The page is used only for its canvas; nothing is loaded.
export async function transferPhoto(
  page: Page,
  amountText: string,
): Promise<Buffer> {
  const dataUrl = await page.evaluate((amount) => {
    const canvas = document.createElement("canvas");
    canvas.width = 480;
    canvas.height = 720;
    const ctx = canvas.getContext("2d");
    if (ctx === null) throw new Error("no 2d canvas");
    ctx.fillStyle = "#f2f5f7";
    ctx.fillRect(0, 0, 480, 720);
    ctx.fillStyle = "#0b5aa8";
    ctx.fillRect(0, 0, 480, 110);
    ctx.fillStyle = "#ffffff";
    ctx.font = "bold 30px sans-serif";
    ctx.fillText("m-Transfer", 32, 68);
    ctx.fillStyle = "#1e7d32";
    ctx.font = "bold 34px sans-serif";
    ctx.fillText("Transfer berhasil", 32, 200);
    ctx.fillStyle = "#1e2a2b";
    ctx.font = "bold 44px sans-serif";
    ctx.fillText(amount, 32, 280);
    ctx.font = "24px sans-serif";
    ctx.fillText("Ke: BCA 0123 456 789", 32, 360);
    ctx.fillText("Berita: sewa kamar", 32, 400);
    return canvas.toDataURL("image/jpeg", 0.85);
  }, amountText);
  const base64 = dataUrl.split(",")[1];
  if (base64 === undefined) throw new Error("the canvas gave no JPEG");
  return Buffer.from(base64, "base64");
}

// The same photo with the EICAR string appended: still a JPEG by its content, infected for the
// scanner.
export function infectedPhoto(photo: Buffer): Buffer {
  return Buffer.concat([photo, Buffer.from(EICAR, "latin1")]);
}

// A payment link for one of the tenant's invoices: the newest that still owes something, else the
// newest. Returns its token, which goes only into the proof's path.
export async function linkToken(
  api: APIRequestContext,
  orgId: string,
  account: TenantAccount,
): Promise<{ token: string; invoiceId: string }> {
  const live = account.invoices.filter((each) => each.status !== "cancelled");
  const owing = live.filter((each) => each.outstanding > 0);
  const invoice = owing.at(-1) ?? live.at(-1);
  if (invoice === undefined) throw new Error("the tenant has no invoice");
  const { token } = await makeLink(api, orgId, invoice.id);
  return { token, invoiceId: invoice.id };
}

// The tenant sends the photo from their link, with no session and a client address of the
// test's own (the payment page's limits are per address). Fails loudly when the API has no
// storage.
export async function sendProof(
  testInfo: TestInfo,
  token: string,
  photo: Buffer,
): Promise<PaymentProof> {
  const tenant = await playwrightRequest.newContext({
    baseURL: testInfo.project.use.baseURL,
    extraHTTPHeaders: {
      "X-Forwarded-For": tenantClientAddress(testInfo, 23),
    },
  });
  try {
    const response = await tenant.post(`/api/t/${token}/proof`, {
      multipart: {
        file: { name: "bukti.jpg", mimeType: "image/jpeg", buffer: photo },
      },
    });
    const text = await response.text();
    if (response.status() === 503) {
      throw new Error(
        `sendPaymentProof answered 503 (${text}). ${STORAGE_HINT}`,
      );
    }
    expect(response.status(), text).toBe(201);
    return JSON.parse(text) as PaymentProof;
  } finally {
    await tenant.dispose();
  }
}

async function queue(
  api: APIRequestContext,
  orgId: string,
  kind: "transfer_proof" | "upload_infected",
): Promise<CheckQueueItem[]> {
  const page = await call<CheckQueuePage>(
    api,
    "GET",
    `/orgs/${orgId}/check-queue?kind=${kind}&pageSize=100`,
    200,
  );
  return page.items;
}

function tenantOf(item: CheckQueueItem): string | undefined {
  return item.transferProof?.tenant.id ?? item.uploadInfected?.tenant?.id;
}

// Takes the tenant's proofs and refused proof files that earlier runs or retries left waiting off
// the queue, so the rows on screen are this test's.
export async function clearTenantProofs(
  api: APIRequestContext,
  orgId: string,
  tenantId: string,
): Promise<void> {
  for (const kind of ["transfer_proof", "upload_infected"] as const) {
    for (const item of await queue(api, orgId, kind)) {
      if (tenantOf(item) !== tenantId) continue;
      await call<CheckResolution>(
        api,
        "POST",
        `/orgs/${orgId}/check-queue/${kind}/${item.id}/dismiss`,
        200,
        { note: "Sisa uji sebelumnya" },
      );
    }
  }
}

// Waits until the scan found the proof clean (the queue lists it with uploadStatus clean).
export async function waitForCleanProof(
  api: APIRequestContext,
  orgId: string,
  proofId: string,
): Promise<CheckQueueItem> {
  let found: CheckQueueItem | undefined;
  await expect
    .poll(
      async () => {
        found = (await queue(api, orgId, "transfer_proof")).find(
          (item) => item.id === proofId,
        );
        return found?.transferProof?.uploadStatus;
      },
      {
        message: `The proof was not scanned clean. ${STORAGE_HINT}`,
        timeout: 30_000,
      },
    )
    .toBe("clean");
  if (found === undefined) throw new Error("no proof");
  return found;
}

// Waits until the scan refused the tenant's proof: it leaves the proofs and is listed as an
// infected upload of the tenant.
export async function waitForInfectedProof(
  api: APIRequestContext,
  orgId: string,
  tenantId: string,
  proofId: string,
): Promise<CheckQueueItem> {
  let found: CheckQueueItem | undefined;
  await expect
    .poll(
      async () => {
        const proofs = await queue(api, orgId, "transfer_proof");
        if (proofs.some((item) => item.id === proofId)) return "pending";
        found = (await queue(api, orgId, "upload_infected")).find(
          (item) =>
            item.uploadInfected?.purpose === "transfer_proof" &&
            item.uploadInfected.tenant?.id === tenantId,
        );
        return found === undefined ? "missing" : "infected";
      },
      {
        message: `The infected proof was not refused. ${STORAGE_HINT}`,
        timeout: 30_000,
      },
    )
    .toBe("infected");
  if (found === undefined) throw new Error("no infected upload");
  return found;
}
