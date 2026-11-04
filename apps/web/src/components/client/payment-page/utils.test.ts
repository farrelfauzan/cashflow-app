import { describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api/errors";
import type {
  PaymentPage,
  PaymentPageInvoice,
} from "@/lib/api/generated/model";

import {
  COPY_STATUS_TEXT,
  MESSAGES,
  copyText,
  errorKind,
  isWellFormedToken,
  linkInvoicePaidNote,
  noTransferText,
  pageTitle,
  paymentPageState,
  paymentPageView,
  remainderLabel,
  summaryRowLabel,
} from "./utils";

const TOKEN = "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789-_AbCdE";

function invoice(
  overrides: Partial<PaymentPageInvoice> = {},
): PaymentPageInvoice {
  return {
    id: "inv-nov",
    kind: "regular",
    unitLabel: "Kamar 07",
    propertyName: "Kos Melati",
    period: "2026-11-01",
    periodEnd: "2026-11-30",
    dueDate: "2026-11-01",
    amount: 1_500_000,
    balance: 1_500_000,
    lines: [{ label: "Sewa 1 Nov 2026 – 30 Nov 2026", amount: 1_500_000 }],
    ...overrides,
  };
}

const OCTOBER_PARTIAL = invoice({
  id: "inv-okt",
  period: "2026-10-01",
  periodEnd: "2026-10-31",
  dueDate: "2026-10-01",
  balance: 750_000,
});

function page(overrides: Partial<PaymentPage> = {}): PaymentPage {
  const linkInvoice = invoice();
  return {
    organizationName: "Griya Sejahtera",
    tenantName: "Bu Rina",
    invoice: linkInvoice,
    outstanding: 2_250_000,
    credit: 0,
    paid: false,
    openInvoices: [OCTOBER_PARTIAL, linkInvoice],
    bankAccount: {
      bankName: "BCA",
      accountNumber: "0123 456 789",
      accountHolder: "Ani Rahmawati",
    },
    ...overrides,
  };
}

function apiError(status: number, code: string): ApiError {
  return new ApiError({ status, code, message: "Pesan." });
}

describe("isWellFormedToken", () => {
  it.each([
    { name: "43 base64url characters", token: TOKEN, want: true },
    {
      name: "only dashes and underscores",
      token: "-_".repeat(21) + "-",
      want: true,
    },
    { name: "42 characters", token: TOKEN.slice(1), want: false },
    { name: "44 characters", token: `${TOKEN}A`, want: false },
    { name: "empty", token: "", want: false },
    { name: "short", token: "abc", want: false },
    { name: "a plus sign", token: `+${TOKEN.slice(1)}`, want: false },
    { name: "a slash", token: `/${TOKEN.slice(1)}`, want: false },
    { name: "padding", token: `${TOKEN.slice(1)}=`, want: false },
    { name: "a dot", token: `.${TOKEN.slice(1)}`, want: false },
    { name: "a newline at the end", token: `${TOKEN}\n`, want: false },
    { name: "a percent escape", token: `%2e${TOKEN.slice(3)}`, want: false },
  ])("$name: $want", ({ token, want }) => {
    expect(isWellFormedToken(token)).toBe(want);
  });
});

describe("errorKind", () => {
  it.each([
    {
      name: "an unknown or revoked link",
      error: apiError(404, "payment_link_not_found"),
      want: "not-found",
    },
    {
      name: "an expired link",
      error: apiError(410, "payment_link_expired"),
      want: "not-found",
    },
    {
      name: "a path the client refused",
      error: apiError(400, "invalid_path"),
      want: "not-found",
    },
    {
      name: "a 404 without the API's body",
      error: apiError(404, "unknown"),
      want: "not-found",
    },
    {
      name: "the rate limit",
      error: apiError(429, "rate_limited"),
      want: "rate-limited",
    },
    {
      name: "a 429 without the API's body",
      error: apiError(429, "unknown"),
      want: "rate-limited",
    },
    {
      name: "no connection",
      error: apiError(0, "network_error"),
      want: "failed",
    },
    { name: "a 500", error: apiError(500, "internal"), want: "failed" },
    { name: "a 503", error: apiError(503, "unknown"), want: "failed" },
    { name: "a plain Error", error: new Error("bug"), want: "failed" },
    { name: "a string", error: "x", want: "failed" },
  ])("$name: $want", ({ error, want }) => {
    expect(errorKind(error)).toBe(want);
  });
});

describe("paymentPageState", () => {
  it("shows not found for a malformed token, whatever the query holds", () => {
    expect(
      paymentPageState({ wellFormed: false, data: page(), error: null }),
    ).toEqual({ kind: "error", error: "not-found" });
  });

  it("is loading before the first answer", () => {
    expect(
      paymentPageState({ wellFormed: true, data: undefined, error: null }),
    ).toEqual({ kind: "loading" });
  });

  it("is ready with the page", () => {
    const state = paymentPageState({
      wellFormed: true,
      data: page(),
      error: null,
    });
    expect(state.kind).toBe("ready");
  });

  it.each([
    { code: "payment_link_not_found", status: 404, want: "not-found" },
    { code: "payment_link_expired", status: 410, want: "not-found" },
    { code: "rate_limited", status: 429, want: "rate-limited" },
    { code: "network_error", status: 0, want: "failed" },
  ])(
    "shows $want for a first read that failed with $code",
    ({ code, status, want }) => {
      expect(
        paymentPageState({
          wellFormed: true,
          data: undefined,
          error: apiError(status, code),
        }),
      ).toEqual({ kind: "error", error: want });
    },
  );

  it("keeps the page on screen when a re-read hits the rate limit or the network", () => {
    for (const error of [
      apiError(429, "rate_limited"),
      apiError(0, "network_error"),
    ]) {
      expect(
        paymentPageState({ wellFormed: true, data: page(), error }).kind,
      ).toBe("ready");
    }
  });

  it("replaces the page when a re-read finds the link revoked or expired", () => {
    for (const error of [
      apiError(404, "payment_link_not_found"),
      apiError(410, "payment_link_expired"),
    ]) {
      expect(
        paymentPageState({ wellFormed: true, data: page(), error }),
      ).toEqual({ kind: "error", error: "not-found" });
    }
  });
});

describe("MESSAGES", () => {
  it("offers Coba lagi only where trying again can help", () => {
    expect(MESSAGES["not-found"].retry).toBe(false);
    expect(MESSAGES["rate-limited"].retry).toBe(true);
    expect(MESSAGES.failed.retry).toBe(true);
  });

  it("never says whether the link existed", () => {
    expect(MESSAGES["not-found"]).toEqual({
      title: "Link tidak berlaku",
      text: "Link pembayaran ini tidak ditemukan atau sudah tidak berlaku. Minta link baru ke pemilik.",
      retry: false,
    });
  });
});

describe("pageTitle", () => {
  it.each([
    {
      name: "a regular invoice",
      kind: "regular" as const,
      want: "Bayar tagihan November 2026",
    },
    {
      name: "the final invoice",
      kind: "final" as const,
      want: "Bayar tagihan akhir November 2026",
    },
  ])("$name: $want", ({ kind, want }) => {
    expect(pageTitle(invoice({ kind }))).toBe(want);
  });
});

describe("summaryRowLabel", () => {
  it.each([
    {
      name: "an unpaid regular invoice is named",
      invoice: invoice(),
      want: "Tagihan November 2026",
    },
    {
      name: "a partly paid regular invoice shows what is left",
      invoice: invoice({ period: "2026-10-01", balance: 750_000 }),
      want: "Sisa Oktober 2026",
    },
    {
      name: "an unpaid final invoice is named",
      invoice: invoice({ kind: "final", period: "2026-10-31" }),
      want: "Tagihan akhir Oktober 2026",
    },
    {
      name: "a partly paid final invoice shows what is left",
      invoice: invoice({ kind: "final", period: "2026-10-31", balance: 1 }),
      want: "Sisa tagihan akhir Oktober 2026",
    },
  ])("$name", ({ invoice, want }) => {
    expect(summaryRowLabel(invoice)).toBe(want);
  });

  it("remainderLabel names the month with its year", () => {
    expect(remainderLabel(invoice({ period: "2026-12-15" }))).toBe(
      "Sisa Desember 2026",
    );
  });
});

describe("paymentPageView, open", () => {
  it("lists older partial debt first, marked from an earlier month, then the link's invoice", () => {
    const view = paymentPageView(page());
    expect(view).toEqual({
      status: "open",
      header: {
        organizationName: "Griya Sejahtera",
        title: "Bayar tagihan November 2026",
        subtitle: "Bu Rina · Kamar 07 Kos Melati",
      },
      linkInvoicePaidNote: null,
      rows: [
        {
          id: "inv-okt",
          label: "Sisa Oktober 2026",
          amount: "Rp750.000",
          fromEarlierMonth: true,
        },
        {
          id: "inv-nov",
          label: "Tagihan November 2026",
          amount: "Rp1.500.000",
          fromEarlierMonth: false,
        },
      ],
      total: "Rp2.250.000",
      credit: null,
      transfer: {
        bankName: "BCA",
        accountNumber: "0123 456 789",
        holderLine: "a.n. Ani Rahmawati",
        noteLine: "Tulis nama Anda di berita transfer: Bu Rina",
      },
      noTransferText:
        "Cara bayar belum diatur. Hubungi Griya Sejahtera untuk membayar.",
    });
  });

  it("shows the API's outstanding as the total, never a sum of the rows", () => {
    const view = paymentPageView(page({ outstanding: 999 }));
    expect(view.status === "open" && view.total).toBe("Rp999");
  });

  it("names a final invoice in the title, subtitle and rows", () => {
    const final = invoice({
      id: "inv-final",
      kind: "final",
      period: "2026-11-20",
      unitLabel: "Kamar 3",
      propertyName: "Kos Putri Anggrek",
      amount: 230_000,
      balance: 100_000,
    });
    const view = paymentPageView(
      page({ invoice: final, openInvoices: [final], outstanding: 100_000 }),
    );
    expect(view.header.title).toBe("Bayar tagihan akhir November 2026");
    expect(view.header.subtitle).toBe("Bu Rina · Kamar 3 Kos Putri Anggrek");
    expect(view.status === "open" && view.rows).toEqual([
      {
        id: "inv-final",
        label: "Sisa tagihan akhir November 2026",
        amount: "Rp100.000",
        fromEarlierMonth: false,
      },
    ]);
  });

  it("says the link's invoice is paid while older debt is still open", () => {
    const paidNovember = invoice({ balance: 0 });
    const view = paymentPageView(
      page({
        invoice: paidNovember,
        openInvoices: [OCTOBER_PARTIAL],
        outstanding: 750_000,
      }),
    );
    expect(view.status).toBe("open");
    if (view.status !== "open") return;
    expect(view.linkInvoicePaidNote).toBe("Tagihan November 2026 sudah lunas.");
    expect(view.header.title).toBe("Bayar tagihan November 2026");
    expect(view.rows.map((row) => row.label)).toEqual(["Sisa Oktober 2026"]);
    expect(view.total).toBe("Rp750.000");
  });

  it("gives no paid note for a link invoice that was cancelled", () => {
    expect(
      linkInvoicePaidNote(
        page({ invoice: invoice({ amount: 0, balance: 0 }) }),
      ),
    ).toBeNull();
  });

  it("marks only invoices whose period is before the link invoice's", () => {
    const december = invoice({ id: "inv-des", period: "2026-12-01" });
    const view = paymentPageView(
      page({ openInvoices: [OCTOBER_PARTIAL, invoice(), december] }),
    );
    expect(
      view.status === "open" && view.rows.map((row) => row.fromEarlierMonth),
    ).toEqual([true, false, false]);
  });

  it("shows credit only when there is some", () => {
    const withCredit = paymentPageView(page({ credit: 50_000 }));
    expect(withCredit.status === "open" && withCredit.credit).toBe("Rp50.000");
    const without = paymentPageView(page({ credit: 0 }));
    expect(without.status === "open" && without.credit).toBeNull();
  });

  it("has no transfer block when the organization set no bank account", () => {
    const view = paymentPageView(
      page({ bankAccount: null, organizationName: "Kost Putri Anggrek" }),
    );
    expect(view.status === "open" && view.transfer).toBeNull();
    expect(view.status === "open" && view.noTransferText).toBe(
      "Cara bayar belum diatur. Hubungi Kost Putri Anggrek untuk membayar.",
    );
  });

  it("noTransferText names the organization", () => {
    expect(noTransferText("Griya Sejahtera")).toBe(
      "Cara bayar belum diatur. Hubungi Griya Sejahtera untuk membayar.",
    );
  });
});

describe("paymentPageView, paid", () => {
  const paidNovember = invoice({ balance: 0 });

  it("shows the link's invoice and amount, without credit", () => {
    expect(
      paymentPageView(
        page({
          paid: true,
          invoice: paidNovember,
          openInvoices: [],
          outstanding: 0,
          credit: 0,
        }),
      ),
    ).toEqual({
      status: "paid",
      header: {
        organizationName: "Griya Sejahtera",
        title: "Bayar tagihan November 2026",
        subtitle: "Bu Rina · Kamar 07 Kos Melati",
      },
      invoiceName: "Tagihan November 2026",
      amount: "Rp1.500.000",
      credit: null,
    });
  });

  it("shows credit when there is some", () => {
    const view = paymentPageView(
      page({
        paid: true,
        invoice: paidNovember,
        openInvoices: [],
        outstanding: 0,
        credit: 200_000,
      }),
    );
    expect(view.status === "paid" && view.credit).toBe("Rp200.000");
  });

  it("names a paid final invoice", () => {
    const view = paymentPageView(
      page({
        paid: true,
        invoice: invoice({ kind: "final", balance: 0, amount: 230_000 }),
        openInvoices: [],
        outstanding: 0,
      }),
    );
    expect(view.status === "paid" && view.invoiceName).toBe(
      "Tagihan akhir November 2026",
    );
    expect(view.status === "paid" && view.amount).toBe("Rp230.000");
  });

  it("follows the API's paid flag, not the invoices", () => {
    expect(paymentPageView(page({ paid: true })).status).toBe("paid");
  });
});

describe("copyText", () => {
  it("copies the number", async () => {
    const written: string[] = [];
    const status = await copyText("0123 456 789", {
      writeText: async (text) => {
        written.push(text);
      },
    });
    expect(status).toBe("copied");
    expect(written).toEqual(["0123 456 789"]);
    expect(COPY_STATUS_TEXT[status]).toBe("Nomor rekening disalin");
  });

  it("fails when the browser refuses", async () => {
    const status = await copyText("0123 456 789", {
      writeText: async () => {
        throw new Error("denied");
      },
    });
    expect(status).toBe("failed");
    expect(COPY_STATUS_TEXT[status]).toBe(
      "Tidak bisa menyalin. Salin nomor di atas secara manual.",
    );
  });

  it("fails when there is no clipboard (an insecure page)", async () => {
    expect(await copyText("0123 456 789", undefined)).toBe("failed");
  });
});
