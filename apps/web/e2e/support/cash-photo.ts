import { expect, type APIRequestContext, type Page } from "@playwright/test";

import type {
  Upload,
  UploadInput,
  UploadTicket,
} from "../../src/lib/api/generated/model";
import { call } from "./payment-page";
import { transferPhoto } from "./proof-fixtures";

const STORAGE_HINT =
  "The e2e job must start RustFS (infra/compose.dev.yaml, profile uploads) and give the API and the worker STORAGE_* and UPLOAD_SCANNER=fake (.github/workflows/web.yml).";

// The photo field cash must come with (decision 50): uploaded as the karyawan api is signed in as,
// for the tenant, before recordCashPayment, which takes the returned id as photoUploadId. The
// photo is a real JPEG drawn on page's canvas; its scan runs later and is not waited for. `bytes`
// replaces it (an infected copy, for "Ambil ulang").
export async function cashPhoto(
  page: Page,
  api: APIRequestContext,
  orgId: string,
  tenantId: string,
  bytes?: Buffer,
): Promise<string> {
  const photo = bytes ?? (await transferPhoto(page, "Uang tunai diterima"));
  const input: UploadInput = {
    purpose: "cash_receipt",
    tenantId,
    contentType: "image/jpeg",
    size: photo.length,
  };
  const started = await api.fetch(`/api/orgs/${orgId}/uploads`, {
    method: "POST",
    data: input,
  });
  const text = await started.text();
  if (started.status() === 503) {
    throw new Error(`createUpload answered 503 (${text}). ${STORAGE_HINT}`);
  }
  expect(started.status(), text).toBe(201);
  const ticket = JSON.parse(text) as UploadTicket;

  const put = await api.fetch(ticket.putUrl, {
    method: "PUT",
    headers: ticket.putHeaders,
    data: photo,
  });
  expect(put.status(), `PUT to storage. ${STORAGE_HINT}`).toBe(200);

  const done = await call<Upload>(
    api,
    "POST",
    `/orgs/${orgId}/uploads/${ticket.upload.id}/complete`,
    200,
  );
  return done.id;
}
