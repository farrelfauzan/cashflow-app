import { expect, test, type Page, type TestInfo } from "@playwright/test";

import {
  attachFullPageScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { ORG_HOME_URL } from "./support/shell";
import { HENDRA } from "./support/staff";

// "Anggota tim": the list, and inviting, changing, disabling and enabling a member, against the
// real API with the devlog WhatsApp fake. Pak Hendra is Admin of the whole of Griya Sejahtera
// and Supervisor in Kost Putri Anggrek. Tests change only members they invite themselves, never
// a seeded one.

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";

// Minimum body text per project, CLAUDE.md "UX rules".
const MIN_BODY_FONT_PX: Record<string, number> = { ipad: 16, phone: 14 };

// A number seed-demo never creates, different per project, per test and per retry, so parallel
// tests, CI retries and the API's 3 invitation sends per number per day never collide:
// 0812-9100-<project><test><retry>, e.g. 0812-9100-0110.
function inviteNumber(testInfo: TestInfo, testDigit: number): string {
  const project = testInfo.project.name === "phone" ? "02" : "01";
  return `0812-9100-${project}${testDigit}${testInfo.retry}`;
}

async function openMembers(page: Page, organization: string) {
  await page.goto("/organizations");
  await page
    .getByRole("list", { name: "Daftar usaha" })
    .getByRole("link", { name: organization })
    .click();
  await expect(page).toHaveURL(ORG_HOME_URL);
  await page.goto(`${page.url()}/members`);
  await expect(
    page.getByRole("heading", { level: 1, name: "Anggota tim" }),
  ).toBeVisible();
  const list = page.getByRole("list", { name: "Daftar anggota" });
  await expect(list.getByRole("listitem").first()).toBeVisible();
  return list;
}

// Fills "Tambah anggota" and sends it. `property` limits the Karyawan role to that property.
async function inviteMember(
  page: Page,
  name: string,
  phone: string,
  property?: string,
) {
  await page.getByRole("button", { name: "Tambah anggota" }).click();
  const panel = page.getByRole("dialog", { name: "Tambah anggota" });
  await expect(panel).toBeVisible();
  await panel.getByLabel("Nama").fill(name);
  await panel.getByLabel("Nomor WhatsApp").fill(phone);
  // Karyawan for the whole organization is the default.
  await expect(panel.getByRole("checkbox", { name: "Karyawan" })).toBeChecked();
  if (property !== undefined) {
    await panel.getByRole("radio", { name: "Hanya properti tertentu" }).check();
    await panel.getByRole("checkbox", { name: property }).check();
  }
  await panel
    .getByRole("button", { name: "Tambah dan kirim undangan" })
    .click();
  return panel;
}

test.use({ storageState: HENDRA.storageState });

test.describe("Admin of the whole organization", () => {
  test("lists the members, with the own row marked and not editable", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const list = await openMembers(page, GRIYA);

    const own = list.getByRole("listitem").filter({ hasText: HENDRA.name });
    await expect(own).toContainText("Anda");
    await expect(own).toContainText("0812-0009-0001");
    await expect(own.getByRole("button", { name: /^Atur akses/ })).toHaveCount(
      0,
    );

    const sari = list
      .getByRole("listitem")
      .filter({ hasText: "Mbak Sari Wulandari" });
    await expect(sari).toContainText("Admin· Kos Melati");
    await expect(sari).not.toContainText("Anda");
    await expect(
      sari.getByRole("button", { name: "Atur akses Mbak Sari Wulandari" }),
    ).toBeVisible();
    await expect(
      list.getByRole("listitem").filter({ hasText: "Mas Joko Prasetyo" }),
    ).toContainText("Admin· Kontrakan Mawar");

    const invite = page.getByRole("button", { name: "Tambah anggota" });
    await expect(invite).toBeVisible();
    expect((await invite.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(
      44,
    );
    const nameSize = await sari
      .getByText("Mbak Sari Wulandari")
      .evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
    expect(nameSize).toBeGreaterThanOrEqual(
      MIN_BODY_FONT_PX[testInfo.project.name] ?? 16,
    );
    await attachFullPageScreenshot(page, testInfo, "members-list");

    await page.getByRole("button", { name: "Arti tiap peran" }).click();
    const guide = page.getByRole("dialog", { name: "Arti tiap peran" });
    await expect(guide).toContainText("Aturan untuk semua");
    await guide.getByRole("button", { name: "Tutup" }).click();
    await expect(guide).toHaveCount(0);

    expect(outsideRequests).toEqual([]);
  });

  test("invites a member for one property, changes the roles, disables and enables again", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const name =
      testInfo.project.name === "phone"
        ? "Mbak Dewi Kartika"
        : "Mas Rizal Fauzi";
    const phone = inviteNumber(testInfo, 1);
    const list = await openMembers(page, GRIYA);

    // Invite, as Karyawan for Kontrakan Mawar only.
    await page.getByRole("button", { name: "Tambah anggota" }).click();
    await attachFullPageScreenshot(page, testInfo, "members-invite-panel");
    await page
      .getByRole("dialog", { name: "Tambah anggota" })
      .getByRole("button", { name: "Batal" })
      .click();
    const panel = await inviteMember(page, name, phone, "Kontrakan Mawar");
    const done = page.getByRole("dialog", { name: "Anggota ditambahkan" });
    await expect(done).toContainText("Undangan terkirim lewat WhatsApp");
    await expect(done).toContainText(
      `${name} sudah ditambahkan sebagai Karyawan di Kontrakan Mawar. Pesan undangan dikirim ke ${phone}.`,
    );
    await done.getByRole("button", { name: "Selesai" }).click();
    await expect(panel).toHaveCount(0);

    const row = list.getByRole("listitem").filter({ hasText: phone });
    await expect(row).toContainText(name);
    await expect(row).toContainText("Karyawan· Kontrakan Mawar");
    await expect(row).toContainText("Belum pernah masuk");
    await expect(row).toContainText(/Undangan terkirim \d{1,2} \S+ \d{4}/);

    // Resend the invitation.
    await row
      .getByRole("button", { name: `Kirim ulang undangan ke ${name}` })
      .click();
    await expect(row).toContainText("Undangan sudah dikirim ulang.");

    // Change the roles: Admin for the whole organization instead of Karyawan.
    await row.getByRole("button", { name: `Atur akses ${name}` }).click();
    const access = page.getByRole("dialog", { name: "Atur akses" });
    await expect(access).toContainText(`${name} · ${phone}`);
    await expect(
      access.getByRole("checkbox", { name: "Supervisor" }),
    ).toBeDisabled();
    await expect(
      access.getByRole("checkbox", { name: "Karyawan" }),
    ).toBeChecked();
    await expect(
      access.getByRole("checkbox", { name: "Kontrakan Mawar" }),
    ).toBeChecked();
    await attachFullPageScreenshot(page, testInfo, "members-access-panel");
    await access.getByRole("checkbox", { name: "Admin" }).check();
    await expect(
      access
        .getByRole("radiogroup", { name: "Wilayah Admin" })
        .getByRole("radio", {
          name: "Seluruh usaha",
        }),
    ).toBeChecked();
    await access.getByRole("checkbox", { name: "Karyawan" }).uncheck();
    await access.getByRole("button", { name: "Simpan peran" }).click();
    await expect(access).toHaveCount(0);
    await expect(page.getByText(`Peran ${name} sudah disimpan.`)).toBeVisible();
    await expect(row).toContainText("Admin");
    await expect(row).not.toContainText("Karyawan");
    await expect(row).not.toContainText("Kontrakan Mawar");

    // Disable, after the confirmation.
    await row.getByRole("button", { name: `Atur akses ${name}` }).click();
    await access.getByRole("button", { name: "Nonaktifkan anggota" }).click();
    const confirm = page.getByRole("alertdialog", {
      name: `Nonaktifkan ${name}?`,
    });
    await expect(confirm).toBeVisible();
    await confirm.getByRole("button", { name: "Ya, nonaktifkan" }).click();
    await expect(confirm).toHaveCount(0);
    await expect(
      access.getByRole("heading", { name: "Akses dihentikan" }),
    ).toBeVisible();
    await access.getByRole("button", { name: "Tutup" }).click();
    await expect(access).toHaveCount(0);
    await expect(row).toContainText("Nonaktif");
    await expect(
      row.getByRole("button", { name: `Kirim ulang undangan ke ${name}` }),
    ).toHaveCount(0);

    // Enable again: the same roles come back.
    await row.getByRole("button", { name: `Atur akses ${name}` }).click();
    await access.getByRole("button", { name: "Aktifkan lagi" }).click();
    await expect(
      access.getByRole("heading", { name: "Hentikan akses" }),
    ).toBeVisible();
    await access.getByRole("button", { name: "Batal" }).click();
    await expect(access).toHaveCount(0);
    await expect(row).toContainText("Belum pernah masuk");
    await expect(row).toContainText("Admin");

    expect(outsideRequests).toEqual([]);
  });

  test("checks the invite form and answers a duplicate number in the alert", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const name =
      testInfo.project.name === "phone"
        ? "Mas Teguh Santoso"
        : "Mbak Ratna Kusuma";
    const phone = inviteNumber(testInfo, 2);
    const list = await openMembers(page, GRIYA);

    // Empty fields are caught before anything is sent.
    await page.getByRole("button", { name: "Tambah anggota" }).click();
    const panel = page.getByRole("dialog", { name: "Tambah anggota" });
    await panel
      .getByRole("button", { name: "Tambah dan kirim undangan" })
      .click();
    await expect(panel.getByText("Isi nama anggota.")).toBeVisible();
    await expect(panel.getByText("Isi nomor WhatsApp.")).toBeVisible();
    await expect(panel.getByLabel("Nama")).toBeFocused();
    await panel.getByRole("button", { name: "Batal" }).click();
    await expect(panel).toHaveCount(0);

    await inviteMember(page, name, phone);
    const done = page.getByRole("dialog", { name: "Anggota ditambahkan" });
    await expect(done).toContainText(
      `${name} sudah ditambahkan sebagai Karyawan.`,
    );
    await done.getByRole("button", { name: "Selesai" }).click();
    await expect(
      list.getByRole("listitem").filter({ hasText: phone }),
    ).toBeVisible();

    // The same number again.
    const again = await inviteMember(page, name, phone);
    await expect(again.getByRole("alert")).toContainText(
      "Nomor ini sudah terdaftar di usaha ini.",
    );
    await expect(again).toBeVisible();
    await expect(
      list.getByRole("listitem").filter({ hasText: phone }),
    ).toHaveCount(1);

    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Supervisor", () => {
  test("sees the members read-only", async ({ page }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const list = await openMembers(page, ANGGREK);

    await expect(
      list.getByRole("listitem").filter({ hasText: HENDRA.name }),
    ).toContainText("Supervisor");
    await expect(
      list.getByRole("listitem").filter({ hasText: "Mbak Wulan Sari" }),
    ).toContainText("Admin");
    await expect(
      page.getByRole("button", { name: "Tambah anggota" }),
    ).toHaveCount(0);
    await expect(page.getByRole("button", { name: /^Atur akses/ })).toHaveCount(
      0,
    );
    await expect(
      page.getByRole("button", { name: /^Kirim ulang undangan/ }),
    ).toHaveCount(0);
    await attachFullPageScreenshot(page, testInfo, "members-list-supervisor");

    expect(outsideRequests).toEqual([]);
  });
});
