import { describe, expect, it } from "vitest";

import { ApiError, GENERIC_ERROR_MESSAGE } from "@/lib/api/errors";
import type {
  PaymentPage,
  PaymentPageInvoice,
  PaymentPreview,
  PaymentPromise,
  QrisStatus,
} from "@/lib/api/generated/model";

import type {
  ChosenFlowMode,
  OpenPaymentPageView,
  PayFlowMode,
  PromiseDraft,
  PromiseOfferView,
  PromisePaying,
  PreviewView,
  QrisCard,
  QrisEvent,
  QrisModel,
} from "./type";
import {
  BUSY_WAIT_MS,
  PAY_MODE,
  PARTIAL_ABOVE_LIMIT_TEXT,
  PROMISE_AMOUNT_MIN_TEXT,
  PROMISE_DATE_REQUIRED_TEXT,
  activePromiseView,
  backFromPromise,
  canChangePartialAmount,
  changePartialAmount,
  choiceMode,
  choicePrimaryLabel,
  chooseFromEntry,
  chosenAmountView,
  chosenPromiseLine,
  chosenRemainder,
  chosenRemainderLine,
  defaultPromiseDraft,
  editPromise,
  entryLeadsToPromise,
  entryNextLine,
  entryPrimaryLabel,
  flowModeAfterCard,
  initialFlowMode,
  nextFromChoice,
  offeredChoice,
  offeredPromiseKind,
  partialAmountCheck,
  partialFieldText,
  partialPayLabel,
  payChoices,
  prefilledPromiseDraft,
  promiseErrorView,
  promiseFormView,
  promiseOfferView,
  promiseRecordedView,
  promiseFormError,
  promiseSchema,
  checkPromiseDraft,
  promiseSaved,
  showsPayBack,
  toPayChoice,
  toPromiseKind,
  previewErrorView,
  previewView,
  qrisPartialAmount,
  quickAmounts,
  COPY_STATUS_TEXT,
  INITIAL_QRIS_MODEL,
  MESSAGES,
  QRIS_CREATE_FAILED_TEXT,
  canCheckQris,
  canStartQris,
  copyText,
  countdownText,
  formatCountdown,
  qrisAnnouncement,
  qrisCheckingRows,
  qrisCreateAmount,
  qrisDownloadName,
  qrisImageAlt,
  qrisImageUrl,
  qrisLimitNote,
  qrisOfferView,
  qrisStartLabel,
  qrisStep,
  readsStatus,
  remainingSeconds,
  showsNewQrUnderLive,
  statusPollInterval,
  toPaymentTab,
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
    qrisAvailable: false,
    promise: null,
    promiseDates: { earliest: "2026-11-03", latest: "2027-01-01" },
    nextInvoiceDueDate: "2026-12-01",
    telegram: { available: false, linked: false },
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
      qris: null,
      tenantName: "Bu Rina",
      outstanding: 2_250_000,
      promise: {
        outstanding: 2_250_000,
        earliest: "2026-11-03",
        latest: "2027-01-01",
        nextInvoiceDueDate: "2026-12-01",
      },
      activePromise: null,
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
      invoice: { name: "Tagihan November 2026", amount: "Rp1.500.000" },
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
    expect(view.status === "paid" && view.invoice).toEqual({
      name: "Tagihan akhir November 2026",
      amount: "Rp230.000",
    });
  });

  it("does not list a link invoice cancelled by a correction as paid Rp0", () => {
    const view = paymentPageView(
      page({
        paid: true,
        invoice: invoice({ amount: 0, balance: 0 }),
        openInvoices: [],
        outstanding: 0,
        credit: 0,
      }),
    );
    expect(view.status === "paid" && view.invoice).toBeNull();
    expect(view.status === "paid" && view.credit).toBeNull();
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

describe("paymentPageState, a link a QRIS request found gone", () => {
  it("shows Link tidak berlaku over a loaded page", () => {
    expect(
      paymentPageState({
        wellFormed: true,
        data: page(),
        error: null,
        linkGone: true,
      }),
    ).toEqual({ kind: "error", error: "not-found" });
  });
});

describe("qrisOfferView", () => {
  it("gives no QRIS (no tabs) when the organization has not set it up", () => {
    expect(qrisOfferView(page({ qrisAvailable: false }))).toBeNull();
    const view = paymentPageView(page({ qrisAvailable: false }));
    expect(view.status === "open" && view.qris).toBeNull();
  });

  it("offers QRIS for the total when it is set up", () => {
    expect(qrisOfferView(page({ qrisAvailable: true }))).toEqual({
      outstanding: 2_250_000,
      startLabel: "Bayar QRIS Rp2.250.000",
      limitNote: null,
      downloadName: "qris-kamar-07.png",
      totalLine: "Total tagihan Rp2.250.000",
      quickAmounts: [
        { amount: 750_000, label: "Sisa Oktober 2026 · Rp750.000" },
        { amount: 250_000, label: "Rp250.000" },
        { amount: 500_000, label: "Rp500.000" },
        { amount: 1_000_000, label: "Rp1.000.000" },
      ],
    });
    const view = paymentPageView(page({ qrisAvailable: true }));
    expect(view.status === "open" && view.qris?.outstanding).toBe(2_250_000);
  });

  it("offers a Rp10.000.000 QR with a note when more is owed", () => {
    const offer = qrisOfferView(
      page({ qrisAvailable: true, outstanding: 12_500_000 }),
    );
    expect(offer?.outstanding).toBe(12_500_000);
    expect(offer?.startLabel).toBe("Bayar QRIS Rp10.000.000");
    expect(offer?.limitNote).toBe(
      "QRIS paling banyak Rp10.000.000 sekali bayar. Sisanya bayar dengan QR berikutnya atau transfer bank.",
    );
  });
});

describe("the Rp10.000.000 rule", () => {
  it.each([
    { outstanding: 1, label: "Bayar QRIS Rp1", amount: 1, note: false },
    {
      outstanding: 10_000_000,
      label: "Bayar QRIS Rp10.000.000",
      amount: 10_000_000,
      note: false,
    },
    {
      outstanding: 10_000_001,
      label: "Bayar QRIS Rp10.000.000",
      amount: 10_000_000,
      note: true,
    },
    {
      outstanding: 25_000_000,
      label: "Bayar QRIS Rp10.000.000",
      amount: 10_000_000,
      note: true,
    },
  ])("$outstanding", ({ outstanding, label, amount, note }) => {
    expect(qrisStartLabel(outstanding)).toBe(label);
    expect(qrisCreateAmount(outstanding)).toBe(amount);
    expect(qrisLimitNote(outstanding) !== null).toBe(note);
  });

  it("asks no QR when nothing is owed", () => {
    expect(qrisCreateAmount(0)).toBeNull();
  });
});

describe("qrisDownloadName", () => {
  it.each([
    { unit: "Kamar 07", want: "qris-kamar-07.png" },
    { unit: "Ruko A/3", want: "qris-ruko-a-3.png" },
    { unit: "  ", want: "qris-tagihan.png" },
    { unit: "", want: "qris-tagihan.png" },
  ])("$unit gives $want", ({ unit, want }) => {
    expect(qrisDownloadName(unit)).toBe(want);
  });
});

describe("qrisImageUrl", () => {
  it("keeps the token in the path and versions the query by the expiry", () => {
    const url = qrisImageUrl(TOKEN, "2026-11-06T05:15:00Z");
    const [path, query] = url.split("?");
    expect(path).toBe(`/api/t/${TOKEN}/qris.png`);
    expect(query).toBe(`v=${Date.parse("2026-11-06T05:15:00Z")}`);
    expect(query).not.toContain(TOKEN);
  });

  it("changes for a new QR", () => {
    expect(qrisImageUrl(TOKEN, "2026-11-06T05:15:00Z")).not.toBe(
      qrisImageUrl(TOKEN, "2026-11-06T05:30:00Z"),
    );
  });

  it("names the amount in the alt text", () => {
    expect(qrisImageAlt(2_250_000)).toBe("Kode QRIS Rp2.250.000");
    expect(qrisImageAlt(null)).toBe("Kode QRIS");
  });
});

const NOW = Date.parse("2026-11-06T05:00:00Z");

function at(offsetMs: number): string {
  return new Date(NOW + offsetMs).toISOString();
}

describe("the countdown", () => {
  it.each([
    { seconds: 872, want: "14:32" },
    { seconds: 900, want: "15:00" },
    { seconds: 59, want: "0:59" },
    { seconds: 0, want: "0:00" },
    { seconds: -5, want: "0:00" },
  ])("$seconds s is $want", ({ seconds, want }) => {
    expect(formatCountdown(seconds)).toBe(want);
  });

  it("reads Berlaku … lagi", () => {
    expect(countdownText(872)).toBe("Berlaku 14:32 lagi");
    expect(countdownText(0)).toBe("Berlaku 0:00 lagi");
  });

  it.each([
    { name: "15 minutes left", expiresAt: at(900_000), want: 900 },
    { name: "a part second is a whole one", expiresAt: at(400), want: 1 },
    { name: "at expiry", expiresAt: at(0), want: 0 },
    { name: "past expiry", expiresAt: at(-60_000), want: 0 },
  ])("remainingSeconds, $name", ({ expiresAt, want }) => {
    expect(remainingSeconds(expiresAt, NOW)).toBe(want);
  });
});

describe("Buat QR baru under a live QR", () => {
  it.each([
    { name: "more than 2 minutes left", left: 120_001, want: false },
    { name: "14 minutes left", left: 14 * 60_000, want: false },
    { name: "exactly 2 minutes left", left: 120_000, want: true },
    { name: "under 2 minutes left", left: 90_000, want: true },
    { name: "expired", left: -1_000, want: true },
  ])("$name: $want", ({ left, want }) => {
    expect(showsNewQrUnderLive(at(left), NOW)).toBe(want);
  });
});

const PENDING: QrisCard = {
  kind: "pending",
  amount: 2_250_000,
  expiresAt: at(600_000),
};
const CHECKING: QrisCard = {
  kind: "checking",
  amount: 2_250_000,
  checkedAt: at(-60_000),
};

describe("status reads", () => {
  it.each([
    { card: { kind: "resuming" } as QrisCard, reads: true, every: false },
    { card: { kind: "idle" } as QrisCard, reads: false, every: false },
    { card: { kind: "creating" } as QrisCard, reads: false, every: false },
    {
      card: { kind: "waiting", until: NOW } as QrisCard,
      reads: true,
      every: 5_000,
    },
    { card: PENDING, reads: true, every: 5_000 },
    { card: CHECKING, reads: true, every: 15_000 },
    {
      card: { kind: "expired", amount: 1 } as QrisCard,
      reads: false,
      every: false,
    },
    { card: { kind: "unavailable" } as QrisCard, reads: false, every: false },
  ])("$card.kind", ({ card, reads, every }) => {
    expect(readsStatus(card)).toBe(reads);
    expect(statusPollInterval(card)).toBe(every);
  });
});

describe("qrisAnnouncement", () => {
  it.each([
    { card: { kind: "creating" } as QrisCard, want: "Membuat QR…" },
    {
      card: PENDING,
      want: "Menunggu pembayaran. Halaman ini berubah sendiri jadi Lunas.",
    },
    { card: CHECKING, want: "Pembayaran sedang dicek" },
    {
      card: { kind: "expired", amount: 1 } as QrisCard,
      want: "QR kedaluwarsa",
    },
    { card: { kind: "idle" } as QrisCard, want: "" },
  ])("$card.kind", ({ card, want }) => {
    expect(qrisAnnouncement(card)).toBe(want);
  });
});

describe("qrisCheckingRows", () => {
  it("shows the amount, the time of the check in Jakarta, and QRIS", () => {
    expect(
      qrisCheckingRows({
        amount: 2_250_000,
        checkedAt: "2026-10-31T12:42:00Z",
      }),
    ).toEqual([
      { label: "Jumlah", value: "Rp2.250.000" },
      { label: "Dicek sejak", value: "31 Okt 2026, 19.42" },
      { label: "Metode", value: "QRIS" },
    ]);
  });

  it("leaves out what the API did not send", () => {
    expect(qrisCheckingRows({ amount: null, checkedAt: null })).toEqual([
      { label: "Metode", value: "QRIS" },
    ]);
  });
});

describe("toPaymentTab", () => {
  it("maps Radix's value", () => {
    expect(toPaymentTab("transfer")).toBe("transfer");
    expect(toPaymentTab("qris")).toBe("qris");
    expect(toPaymentTab("other")).toBe("qris");
  });
});

function model(card: QrisCard, overrides: Partial<QrisModel> = {}): QrisModel {
  return { ...INITIAL_QRIS_MODEL, card, ...overrides };
}

function status(overrides: Partial<QrisStatus>): QrisStatus {
  return { status: "none", ...overrides };
}

function qrisError(status: number, code: string, field?: string): ApiError {
  return new ApiError({ status, code, message: `Pesan ${code}.`, field });
}

const NO_EFFECTS = {
  refetchPage: false,
  linkGone: false,
  pollNow: false,
  retryCreate: false,
};

describe("qrisStep, resuming on load", () => {
  const resuming = model({ kind: "resuming" });

  function read(answer: QrisStatus) {
    return qrisStep(resuming, {
      type: "status-read",
      checks: 0,
      status: answer,
      now: NOW,
    });
  }

  it("resumes a QR that can still be paid", () => {
    const step = read(
      status({ status: "pending", amount: 700_000, expiresAt: at(300_000) }),
    );
    expect(step).toEqual({
      model: model({
        kind: "pending",
        amount: 700_000,
        expiresAt: at(300_000),
      }),
      ...NO_EFFECTS,
    });
  });

  it("resumes a QR being checked", () => {
    expect(
      read(
        status({
          status: "checking",
          amount: 700_000,
          expiresAt: at(300_000),
          checkedAt: at(-30_000),
        }),
      ).model.card,
    ).toEqual({ kind: "checking", amount: 700_000, checkedAt: at(-30_000) });
  });

  it.each([
    {
      name: "a pending QR at its expiry",
      answer: status({ status: "pending", amount: 1, expiresAt: at(0) }),
    },
    {
      name: "a pending QR past its expiry",
      answer: status({ status: "pending", amount: 1, expiresAt: at(-1) }),
    },
    {
      name: "a paid QR from before the visit",
      answer: status({ status: "paid", amount: 1, expiresAt: at(-9e8) }),
    },
    {
      name: "an expired QR from before the visit",
      answer: status({ status: "expired", amount: 1, expiresAt: at(-9e8) }),
    },
    { name: "no QR yet", answer: status({ status: "none" }) },
  ])("starts from Bayar QRIS for $name, and never shows it", ({ answer }) => {
    expect(read(answer)).toEqual({
      model: model({ kind: "idle" }),
      ...NO_EFFECTS,
    });
  });

  it.each([
    { name: "the rate limit", error: qrisError(429, "rate_limited") },
    { name: "no connection", error: qrisError(0, "network_error") },
    { name: "a 500", error: qrisError(500, "internal") },
  ])("starts from Bayar QRIS when the read fails with $name", ({ error }) => {
    expect(
      qrisStep(resuming, { type: "status-failed", error }).model.card,
    ).toEqual({ kind: "idle" });
  });

  it.each([
    { name: "a revoked link", error: qrisError(404, "payment_link_not_found") },
    { name: "an expired link", error: qrisError(410, "payment_link_expired") },
  ])("shows Link tidak berlaku for $name", ({ error }) => {
    expect(qrisStep(resuming, { type: "status-failed", error }).linkGone).toBe(
      true,
    );
  });
});

describe("qrisStep, starting a QR", () => {
  it.each([
    { card: { kind: "idle" } as QrisCard },
    { card: { kind: "expired", amount: 1 } as QrisCard },
    { card: PENDING },
    { card: { kind: "unavailable" } as QrisCard },
  ])("starts from $card.kind, on the QRIS tab", ({ card }) => {
    const step = qrisStep(model(card, { tab: "transfer", alert: "Lama." }), {
      type: "create-started",
    });
    expect(step.model).toEqual(model({ kind: "creating" }));
  });

  it.each([
    { card: { kind: "resuming" } as QrisCard },
    { card: { kind: "creating" } as QrisCard },
    { card: { kind: "waiting", until: NOW } as QrisCard },
    { card: CHECKING },
  ])("does not start twice or from $card.kind", ({ card }) => {
    const before = model(card);
    expect(qrisStep(before, { type: "create-started" }).model).toBe(before);
    expect(canStartQris(before)).toBe(false);
  });

  it("does not start while Saya sudah bayar runs", () => {
    const before = model(PENDING, { checkRunning: true });
    expect(canStartQris(before)).toBe(false);
    expect(qrisStep(before, { type: "create-started" }).model).toBe(before);
  });

  it("goes back to idle when the amount cannot go to QRIS", () => {
    expect(
      qrisStep(model({ kind: "creating" }), { type: "create-skipped" }).model
        .card,
    ).toEqual({ kind: "idle" });
  });
});

describe("qrisStep, createPaymentQris answers", () => {
  const creating = model({ kind: "creating" });

  function answer(reply: QrisStatus) {
    return qrisStep(creating, { type: "created", status: reply, now: NOW });
  }

  it("shows a pending QR", () => {
    expect(
      answer(
        status({ status: "pending", amount: 700_000, expiresAt: at(900_000) }),
      ),
    ).toEqual({
      model: model({
        kind: "pending",
        amount: 700_000,
        expiresAt: at(900_000),
      }),
      ...NO_EFFECTS,
    });
  });

  it("shows Sedang dicek for a reused QR being checked", () => {
    expect(
      answer(
        status({
          status: "checking",
          amount: 700_000,
          expiresAt: at(300_000),
          checkedAt: at(-1_000),
        }),
      ).model.card,
    ).toEqual({ kind: "checking", amount: 700_000, checkedAt: at(-1_000) });
  });

  it("reads the page again when the previous QR turned out paid", () => {
    expect(
      answer(status({ status: "paid", amount: 700_000, expiresAt: at(-1) })),
    ).toEqual({
      model: model({ kind: "idle", paid: true }),
      ...NO_EFFECTS,
      refetchPage: true,
    });
  });

  it.each([
    { reply: status({ status: "none" }) },
    { reply: status({ status: "expired", amount: 1, expiresAt: at(-1) }) },
    { reply: status({ status: "pending", amount: 1 }) },
  ])(
    "treats an answer it cannot show ($reply.status) as a failure",
    ({ reply }) => {
      expect(answer(reply).model).toEqual(
        model({ kind: "idle" }, { alert: QRIS_CREATE_FAILED_TEXT }),
      );
    },
  );

  it("ignores an answer when no QR is being made", () => {
    const idle = model({ kind: "idle" });
    expect(
      qrisStep(idle, {
        type: "created",
        status: status({ status: "pending", expiresAt: at(1) }),
        now: NOW,
      }).model,
    ).toBe(idle);
  });

  function failed(error: unknown) {
    return qrisStep(creating, { type: "create-failed", error, now: NOW });
  }

  it("polls as if pending on 409 qris_busy", () => {
    const step = failed(qrisError(409, "qris_busy"));
    expect(step.model.card).toEqual({
      kind: "waiting",
      until: NOW + BUSY_WAIT_MS,
    });
    expect(readsStatus(step.model.card)).toBe(true);
    expect(statusPollInterval(step.model.card)).toBe(5_000);
  });

  it("reads the page again on 409 nothing_to_pay", () => {
    expect(failed(qrisError(409, "nothing_to_pay"))).toEqual({
      model: model({ kind: "idle" }),
      ...NO_EFFECTS,
      refetchPage: true,
    });
  });

  it("retries once on 503 qris_overloaded, keeping the button busy", () => {
    expect(failed(qrisError(503, "qris_overloaded"))).toEqual({
      model: model({ kind: "creating", retried: true }),
      ...NO_EFFECTS,
      retryCreate: true,
    });
  });

  it("switches to Transfer bank when the retry is overloaded too", () => {
    expect(
      qrisStep(model({ kind: "creating", retried: true }), {
        type: "create-failed",
        error: qrisError(503, "qris_overloaded"),
        now: NOW,
      }),
    ).toEqual({
      model: model({ kind: "unavailable" }, { tab: "transfer" }),
      ...NO_EFFECTS,
    });
  });

  it("starts with a fresh retry when the tenant presses again", () => {
    const next = qrisStep(model({ kind: "idle" }), { type: "create-started" });
    expect(next.model.card).toEqual({ kind: "creating" });
  });

  it("switches to Transfer bank with the banner on 503 qris_unavailable", () => {
    expect(failed(qrisError(503, "qris_unavailable"))).toEqual({
      model: model({ kind: "unavailable" }, { tab: "transfer" }),
      ...NO_EFFECTS,
    });
  });

  it.each([
    { name: "5 QRs in 24 hours", error: qrisError(429, "qris_limit") },
    { name: "the rate limit", error: qrisError(429, "rate_limited") },
    {
      name: "an amount above Rp10.000.000",
      error: qrisError(400, "qris_amount_above_limit", "amount"),
    },
    {
      name: "an amount below Rp1",
      error: qrisError(400, "invalid_input", "amount"),
    },
  ])("stays idle with the API's message for $name", ({ error }) => {
    expect(failed(error)).toEqual({
      model: model({ kind: "idle" }, { alert: error.message }),
      ...NO_EFFECTS,
    });
  });

  it("also reads the page again when the amount is above what is owed", () => {
    const error = qrisError(400, "qris_amount_above_outstanding", "amount");
    expect(failed(error)).toEqual({
      model: model({ kind: "idle" }, { alert: error.message }),
      ...NO_EFFECTS,
      refetchPage: true,
    });
  });

  it.each([
    { name: "a revoked link", error: qrisError(404, "payment_link_not_found") },
    { name: "an expired link", error: qrisError(410, "payment_link_expired") },
    { name: "a bare 404", error: qrisError(404, "unknown") },
  ])("shows Link tidak berlaku for $name", ({ error }) => {
    expect(failed(error)).toEqual({
      model: model({ kind: "idle" }),
      ...NO_EFFECTS,
      linkGone: true,
    });
  });

  it.each([
    { name: "no connection", error: qrisError(0, "network_error") },
    { name: "a 500", error: qrisError(500, "internal") },
    { name: "a 502", error: qrisError(502, "unknown") },
    {
      name: "a 503 without qris_unavailable",
      error: qrisError(503, "unknown"),
    },
    {
      name: "invalid_json without a field",
      error: qrisError(400, "invalid_json"),
    },
    { name: "a plain Error", error: new Error("bug") },
  ])("offers to try again after $name", ({ error }) => {
    expect(failed(error)).toEqual({
      model: model({ kind: "idle" }, { alert: QRIS_CREATE_FAILED_TEXT }),
      ...NO_EFFECTS,
    });
  });
});

describe("qrisStep, waiting after qris_busy", () => {
  const waiting = model({ kind: "waiting", until: NOW + BUSY_WAIT_MS });

  function read(answer: QrisStatus, now = NOW) {
    return qrisStep(waiting, {
      type: "status-read",
      checks: 0,
      status: answer,
      now,
    });
  }

  it("shows the other request's QR once it is there", () => {
    expect(
      read(status({ status: "pending", amount: 5, expiresAt: at(900_000) }))
        .model.card,
    ).toEqual({ kind: "pending", amount: 5, expiresAt: at(900_000) });
  });

  it("shows Sedang dicek when that QR is being checked", () => {
    expect(
      read(status({ status: "checking", amount: 5, checkedAt: at(-1) })).model
        .card.kind,
    ).toBe("checking");
  });

  it("reads the page again on paid", () => {
    expect(read(status({ status: "paid", amount: 5 }))).toEqual({
      model: model({ kind: "idle", paid: true }),
      ...NO_EFFECTS,
      refetchPage: true,
    });
  });

  it.each([
    { answer: status({ status: "expired", amount: 5, expiresAt: at(-1) }) },
    { answer: status({ status: "none" }) },
  ])("keeps waiting on an older $answer.status answer", ({ answer }) => {
    expect(read(answer).model).toBe(waiting);
  });

  it("gives up at the deadline", () => {
    expect(read(status({ status: "none" }), NOW + BUSY_WAIT_MS).model).toEqual(
      model({ kind: "idle" }, { alert: QRIS_CREATE_FAILED_TEXT }),
    );
  });
});

describe("qrisStep, polling a QR of this visit", () => {
  it.each([
    {
      name: "pending stays pending, with the API's newest values",
      from: PENDING,
      answer: status({
        status: "pending",
        amount: 2_250_000,
        expiresAt: at(1),
      }),
      card: { kind: "pending", amount: 2_250_000, expiresAt: at(1) },
      refetchPage: false,
    },
    {
      name: "pending past expiry by the device's clock stays pending until the API says expired",
      from: PENDING,
      answer: status({
        status: "pending",
        amount: 2_250_000,
        expiresAt: at(-5_000),
      }),
      card: { kind: "pending", amount: 2_250_000, expiresAt: at(-5_000) },
      refetchPage: false,
    },
    {
      name: "pending becomes checking",
      from: PENDING,
      answer: status({
        status: "checking",
        amount: 2_250_000,
        checkedAt: at(-1),
      }),
      card: { kind: "checking", amount: 2_250_000, checkedAt: at(-1) },
      refetchPage: false,
    },
    {
      name: "pending becomes paid: the page is read again",
      from: PENDING,
      answer: status({ status: "paid", amount: 2_250_000 }),
      card: { kind: "idle", paid: true },
      refetchPage: true,
    },
    {
      name: "pending becomes expired",
      from: PENDING,
      answer: status({
        status: "expired",
        amount: 2_250_000,
        expiresAt: at(-1),
      }),
      card: { kind: "expired", amount: 2_250_000 },
      refetchPage: false,
    },
    {
      name: "an expired answer without an amount keeps the QR's",
      from: PENDING,
      answer: status({ status: "expired" }),
      card: { kind: "expired", amount: 2_250_000 },
      refetchPage: false,
    },
    {
      name: "pending with no QR at all goes idle",
      from: PENDING,
      answer: status({ status: "none" }),
      card: { kind: "idle" },
      refetchPage: false,
    },
    {
      name: "checking stays checking",
      from: CHECKING,
      answer: status({
        status: "checking",
        amount: 2_250_000,
        checkedAt: at(-2),
      }),
      card: { kind: "checking", amount: 2_250_000, checkedAt: at(-2) },
      refetchPage: false,
    },
    {
      name: "checking becomes paid",
      from: CHECKING,
      answer: status({ status: "paid", amount: 2_250_000 }),
      card: { kind: "idle", paid: true },
      refetchPage: true,
    },
    {
      name: "checking becomes expired",
      from: CHECKING,
      answer: status({ status: "expired", amount: 2_250_000 }),
      card: { kind: "expired", amount: 2_250_000 },
      refetchPage: false,
    },
  ])("$name", ({ from, answer, card, refetchPage }) => {
    const step = qrisStep(model(from), {
      type: "status-read",
      checks: 0,
      status: answer,
      now: NOW,
    });
    expect(step.model.card).toEqual(card);
    expect(step.refetchPage).toBe(refetchPage);
    expect(step.linkGone).toBe(false);
  });

  it("keeps the alert while the card stays the same", () => {
    const step = qrisStep(model(PENDING, { alert: "Pesan." }), {
      type: "status-read",
      checks: 0,
      status: status({ status: "pending", amount: 1, expiresAt: at(1) }),
      now: NOW,
    });
    expect(step.model.alert).toBe("Pesan.");
  });

  it.each([
    { name: "the rate limit", error: qrisError(429, "rate_limited") },
    { name: "no connection", error: qrisError(0, "network_error") },
  ])("keeps the QR and waits for the next tick after $name", ({ error }) => {
    const before = model(PENDING);
    expect(qrisStep(before, { type: "status-failed", error })).toEqual({
      model: before,
      ...NO_EFFECTS,
    });
  });

  it("shows Link tidak berlaku when the link stops working", () => {
    expect(
      qrisStep(model(CHECKING), {
        type: "status-failed",
        error: qrisError(410, "payment_link_expired"),
      }).linkGone,
    ).toBe(true);
  });

  it.each([
    { card: { kind: "idle" } as QrisCard },
    { card: { kind: "expired", amount: 1 } as QrisCard },
    { card: { kind: "creating" } as QrisCard },
    { card: { kind: "unavailable" } as QrisCard },
  ])(
    "never shows a status read in $card.kind (an old paid or expired answer)",
    ({ card }) => {
      const before = model(card);
      for (const answer of [
        status({ status: "paid", amount: 1 }),
        status({ status: "expired", amount: 1 }),
        status({ status: "pending", amount: 1, expiresAt: at(600_000) }),
      ]) {
        expect(
          qrisStep(before, {
            type: "status-read",
            checks: 0,
            status: answer,
            now: NOW,
          }),
        ).toEqual({ model: before, ...NO_EFFECTS });
      }
    },
  );

  it("ignores status reads while Saya sudah bayar runs", () => {
    const before = model(PENDING, { checkRunning: true });
    expect(
      qrisStep(before, {
        type: "status-read",
        checks: 0,
        status: status({ status: "expired", amount: 1 }),
        now: NOW,
      }).model,
    ).toBe(before);
  });
});

describe("qrisStep, Saya sudah bayar", () => {
  // One check started (checks 1); its answer ends it (checks 2).
  const running = model(PENDING, { checkRunning: true, checks: 1 });
  const DONE = { checks: 2 };

  it("runs only under a live QR, once at a time", () => {
    expect(canCheckQris(model(PENDING))).toBe(true);
    expect(canCheckQris(running)).toBe(false);
    expect(canCheckQris(model(CHECKING))).toBe(false);
    expect(
      qrisStep(model(PENDING, { alert: "Lama." }), { type: "check-started" })
        .model,
    ).toEqual(running);
    const idle = model({ kind: "idle" });
    expect(qrisStep(idle, { type: "check-started" }).model).toBe(idle);
  });

  it.each([
    {
      name: "checking shows Sedang dicek",
      answer: status({
        status: "checking",
        amount: 2_250_000,
        checkedAt: at(0),
      }),
      card: { kind: "checking", amount: 2_250_000, checkedAt: at(0) },
      refetchPage: false,
    },
    {
      name: "paid reads the page again",
      answer: status({ status: "paid", amount: 2_250_000 }),
      card: { kind: "idle", paid: true },
      refetchPage: true,
    },
    {
      name: "expired shows QR kedaluwarsa",
      answer: status({ status: "expired", amount: 2_250_000 }),
      card: { kind: "expired", amount: 2_250_000 },
      refetchPage: false,
    },
    {
      name: "no QR goes idle",
      answer: status({ status: "none" }),
      card: { kind: "idle" },
      refetchPage: false,
    },
  ])("$name", ({ answer, card, refetchPage }) => {
    const step = qrisStep(running, {
      type: "checked",
      status: answer,
      now: NOW,
    });
    expect(step.model).toEqual(model(card as QrisCard, DONE));
    expect(step.refetchPage).toBe(refetchPage);
  });

  function failed(error: unknown) {
    return qrisStep(running, { type: "check-failed", error });
  }

  it("polls on 409 qris_busy", () => {
    expect(failed(qrisError(409, "qris_busy"))).toEqual({
      model: model(PENDING, DONE),
      ...NO_EFFECTS,
      pollNow: true,
    });
  });

  it("goes idle on 404 qris_not_found, which is not a dead link", () => {
    expect(failed(qrisError(404, "qris_not_found"))).toEqual({
      model: model({ kind: "idle" }, DONE),
      ...NO_EFFECTS,
    });
  });

  it("shows the API's message on 429", () => {
    const error = qrisError(429, "rate_limited");
    expect(failed(error)).toEqual({
      model: model(PENDING, { ...DONE, alert: error.message }),
      ...NO_EFFECTS,
    });
  });

  it("shows Link tidak berlaku for a revoked link", () => {
    expect(failed(qrisError(404, "payment_link_not_found")).linkGone).toBe(
      true,
    );
  });

  it("keeps the QR and says what went wrong otherwise", () => {
    const error = qrisError(0, "network_error");
    expect(failed(error).model).toEqual(
      model(PENDING, { ...DONE, alert: error.message }),
    );
    expect(failed(new Error("bug")).model).toEqual(
      model(PENDING, { ...DONE, alert: "Terjadi kesalahan. Coba lagi." }),
    );
  });

  it("ignores a late answer when no check is running", () => {
    const before = model(PENDING);
    const events: QrisEvent[] = [
      { type: "checked", status: status({ status: "paid" }), now: NOW },
      { type: "check-failed", error: qrisError(429, "rate_limited") },
    ];
    for (const event of events) {
      expect(qrisStep(before, event).model).toBe(before);
    }
  });

  it("drops a status read that began before the check, even when it lands after", () => {
    // A 5 s poll left (checks 0), the tenant tapped Saya sudah bayar, and the check answered
    // checking before the poll's older pending came back.
    const checking = qrisStep(running, {
      type: "checked",
      status: status({
        status: "checking",
        amount: 2_250_000,
        checkedAt: at(0),
      }),
      now: NOW,
    }).model;
    const stale = qrisStep(checking, {
      type: "status-read",
      checks: 0,
      status: status({
        status: "pending",
        amount: 2_250_000,
        expiresAt: at(10 * 60),
      }),
      now: NOW,
    });
    expect(stale.model).toBe(checking);
    // The same read begun during the check is stale too.
    expect(
      qrisStep(checking, {
        type: "status-read",
        checks: 1,
        status: status({
          status: "pending",
          amount: 2_250_000,
          expiresAt: at(10 * 60),
        }),
        now: NOW,
      }).model,
    ).toBe(checking);
  });

  it("follows a read that began after the check, such as a QR made in another tab", () => {
    const checking = model(CHECKING, DONE);
    const next = qrisStep(checking, {
      type: "status-read",
      checks: 2,
      status: status({
        status: "pending",
        amount: 2_250_000,
        expiresAt: at(14 * 60),
      }),
      now: NOW,
    });
    expect(next.model.card).toEqual({
      kind: "pending",
      amount: 2_250_000,
      expiresAt: at(14 * 60),
    });
  });
});

describe("qrisStep, tabs", () => {
  it("switches tabs and keeps the card", () => {
    const before = model({ kind: "unavailable" }, { tab: "transfer" });
    expect(
      qrisStep(before, { type: "tab-selected", tab: "qris" }).model,
    ).toEqual(model({ kind: "unavailable" }, { tab: "qris" }));
  });
});

describe("quickAmounts", () => {
  it("offers what is left of the oldest invoice first when several are open", () => {
    expect(quickAmounts(page())).toEqual([
      { amount: 750_000, label: "Sisa Oktober 2026 · Rp750.000" },
      { amount: 250_000, label: "Rp250.000" },
      { amount: 500_000, label: "Rp500.000" },
      { amount: 1_000_000, label: "Rp1.000.000" },
    ]);
  });

  it("names an untouched oldest invoice as in the summary", () => {
    const october = invoice({
      id: "inv-okt",
      period: "2026-10-01",
      balance: 1_500_000,
    });
    const chips = quickAmounts(
      page({ outstanding: 3_000_000, openInvoices: [october, invoice()] }),
    );
    expect(chips[0]).toEqual({
      amount: 1_500_000,
      label: "Tagihan Oktober 2026 · Rp1.500.000",
    });
  });

  it("offers only round amounts when one invoice is open", () => {
    const only = invoice();
    expect(
      quickAmounts(page({ outstanding: 1_500_000, openInvoices: [only] })),
    ).toEqual([
      { amount: 250_000, label: "Rp250.000" },
      { amount: 500_000, label: "Rp500.000" },
      { amount: 1_000_000, label: "Rp1.000.000" },
    ]);
  });

  it("drops round amounts at or above the total", () => {
    const only = invoice({ amount: 500_000, balance: 500_000 });
    expect(
      quickAmounts(page({ outstanding: 500_000, openInvoices: [only] })),
    ).toEqual([{ amount: 250_000, label: "Rp250.000" }]);
    expect(
      quickAmounts(
        page({
          outstanding: 250_000,
          openInvoices: [invoice({ amount: 250_000, balance: 250_000 })],
        }),
      ),
    ).toEqual([]);
  });

  it("does not offer a round amount twice", () => {
    const october = invoice({
      id: "inv-okt",
      period: "2026-10-01",
      balance: 500_000,
    });
    const amounts = quickAmounts(
      page({ outstanding: 2_000_000, openInvoices: [october, invoice()] }),
    ).map((chip) => chip.amount);
    expect(amounts).toEqual([500_000, 250_000, 1_000_000]);
  });

  it("leaves out an oldest balance one QR cannot take", () => {
    const october = invoice({
      id: "inv-okt",
      period: "2026-10-01",
      amount: 12_000_000,
      balance: 12_000_000,
    });
    const amounts = quickAmounts(
      page({ outstanding: 13_500_000, openInvoices: [october, invoice()] }),
    ).map((chip) => chip.amount);
    expect(amounts).toEqual([250_000, 500_000, 1_000_000]);
  });

  it("is part of the QRIS offer, with the total line", () => {
    const offer = qrisOfferView(page({ qrisAvailable: true }));
    expect(offer?.totalLine).toBe("Total tagihan Rp2.250.000");
    expect(offer?.quickAmounts).toHaveLength(4);
  });
});

describe("partialAmountCheck", () => {
  it.each([
    { name: "empty", text: "", want: { kind: "empty" } },
    { name: "zero", text: "0", want: { kind: "empty" } },
    { name: "Rp1", text: "1", want: { kind: "valid", amount: 1 } },
    {
      name: "grouped",
      text: "1.000.000",
      want: { kind: "valid", amount: 1_000_000 },
    },
    {
      name: "one QR's most",
      text: "10.000.000",
      want: { kind: "valid", amount: 10_000_000 },
    },
    {
      name: "above one QR's most",
      text: "10.000.001",
      want: { kind: "invalid", message: PARTIAL_ABOVE_LIMIT_TEXT },
    },
  ])("$name", ({ text, want }) => {
    expect(partialAmountCheck(text)).toEqual(want);
  });

  it("says the limit in Bahasa Indonesia", () => {
    expect(PARTIAL_ABOVE_LIMIT_TEXT).toBe(
      "QRIS paling banyak Rp10.000.000 sekali bayar.",
    );
  });

  it("partialFieldText groups a chip's amount for the field", () => {
    expect(partialFieldText(750_000)).toBe("750.000");
    expect(partialAmountCheck(partialFieldText(750_000))).toEqual({
      kind: "valid",
      amount: 750_000,
    });
  });
});

describe("qrisPartialAmount", () => {
  it.each([
    {
      name: "below what is owed",
      chosen: 500_000,
      owed: 2_250_000,
      want: 500_000,
    },
    {
      name: "equal to what is owed",
      chosen: 2_250_000,
      owed: 2_250_000,
      want: 2_250_000,
    },
    {
      name: "above what is owed now",
      chosen: 1_000_000,
      owed: 750_000,
      want: 750_000,
    },
    {
      name: "above one QR's most",
      chosen: 12_000_000,
      owed: 20_000_000,
      want: 10_000_000,
    },
    { name: "nothing owed", chosen: 500_000, owed: 0, want: null },
    { name: "no amount", chosen: 0, owed: 500_000, want: null },
  ])("$name", ({ chosen, owed, want }) => {
    expect(qrisPartialAmount(chosen, owed)).toBe(want);
  });
});

describe("partial labels", () => {
  it("names the amount on the primary", () => {
    expect(partialPayLabel(1_000_000)).toBe("Bayar QRIS Rp1.000.000");
    expect(partialPayLabel(null)).toBe("Bayar QRIS");
  });

  it("says what stays owed only when something does", () => {
    expect(chosenRemainderLine(1_250_000)).toBe(
      "Sisa Rp1.250.000 tetap jadi tagihan.",
    );
    expect(chosenRemainderLine(0)).toBeNull();
  });
});

describe("chosenAmountView", () => {
  const chosenMode = (
    promise: PaymentPromise | null = null,
  ): ChosenFlowMode => ({
    kind: "chosen",
    text: "1.000.000",
    amount: 1_000_000,
    outstandingAfter: 500_000,
    promise,
  });
  const live = (amount: number): QrisCard => ({
    kind: "pending",
    amount,
    expiresAt: "2026-11-06T12:15:00Z",
  });

  it("shows the chosen amount and what stays owed before any QR", () => {
    expect(chosenAmountView(chosenMode(), { kind: "idle" })).toEqual({
      amount: "Rp1.000.000",
      remainderLine: "Sisa Rp500.000 tetap jadi tagihan.",
    });
  });

  it("keeps the line while the QR is for the chosen amount", () => {
    expect(chosenAmountView(chosenMode(), live(1_000_000))).toEqual({
      amount: "Rp1.000.000",
      remainderLine: "Sisa Rp500.000 tetap jadi tagihan.",
    });
    expect(
      chosenAmountView(
        chosenMode(promise({ promisedDate: "2026-12-01" })),
        live(1_000_000),
      ),
    ).toEqual({
      amount: "Rp1.000.000",
      remainderLine: "Sisa Rp500.000 dijanjikan Selasa, 1 Desember 2026.",
    });
  });

  it("shows the QR's amount and no stale line when a payment made the QR smaller", () => {
    // Owed Rp1.500.000, chose Rp1.000.000; the QR expired, Rp800.000 cash was recorded, and the
    // new QR is capped at the Rp700.000 still owed.
    const view = { amount: "Rp700.000", remainderLine: null };
    expect(chosenAmountView(chosenMode(), live(700_000))).toEqual(view);
    expect(
      chosenAmountView(chosenMode(), { kind: "expired", amount: 700_000 }),
    ).toEqual(view);
    expect(
      chosenAmountView(
        chosenMode(promise({ promisedDate: "2026-12-01" })),
        live(700_000),
      ),
    ).toEqual(view);
  });

  it("falls back to the chosen amount when the QR's amount is unknown", () => {
    expect(
      chosenAmountView(chosenMode(), { kind: "expired", amount: null }),
    ).toEqual({
      amount: "Rp1.000.000",
      remainderLine: "Sisa Rp500.000 tetap jadi tagihan.",
    });
  });
});

const PREVIEW: PaymentPreview = {
  amount: 1_000_000,
  outstandingBefore: 2_250_000,
  outstandingAfter: 1_250_000,
  items: [
    {
      invoiceId: "inv-okt",
      kind: "regular",
      unitLabel: "Kamar 07",
      propertyName: "Kos Melati",
      period: "2026-10-01",
      periodEnd: "2026-10-31",
      dueDate: "2026-10-01",
      balance: 750_000,
      applied: 750_000,
      balanceAfter: 0,
      paidInFull: true,
    },
    {
      invoiceId: "inv-nov",
      kind: "regular",
      unitLabel: "Kamar 07",
      propertyName: "Kos Melati",
      period: "2026-11-01",
      periodEnd: "2026-11-30",
      dueDate: "2026-11-01",
      balance: 1_500_000,
      applied: 250_000,
      balanceAfter: 1_250_000,
      paidInFull: false,
    },
  ],
};

describe("previewView", () => {
  it("shows the API's items as they are", () => {
    expect(previewView(PREVIEW)).toEqual({
      amount: 1_000_000,
      rows: [
        {
          id: "inv-okt",
          name: "Oktober 2026",
          chip: "Lunas",
          paidInFull: true,
          applied: "Rp750.000",
        },
        {
          id: "inv-nov",
          name: "November 2026",
          chip: "sisa Rp1.250.000",
          paidInFull: false,
          applied: "Rp250.000",
        },
      ],
      outstandingAfterLine: "Sisa tagihan setelah ini Rp1.250.000",
      outstandingAfter: 1_250_000,
    });
  });

  it("has no remainder line when the amount pays everything", () => {
    expect(
      previewView({ ...PREVIEW, outstandingAfter: 0 }).outstandingAfterLine,
    ).toBeNull();
  });
});

describe("previewErrorView", () => {
  it("replaces the page when the link stopped working", () => {
    expect(previewErrorView(apiError(404, "payment_link_not_found"))).toEqual({
      kind: "link-gone",
    });
    expect(previewErrorView(apiError(410, "payment_link_expired"))).toEqual({
      kind: "link-gone",
    });
  });

  it("puts the API's refusal of the amount under the field", () => {
    const error = new ApiError({
      status: 400,
      code: "amount_above_outstanding",
      message: "Nominal lebih dari total tagihan.",
      field: "amount",
    });
    expect(previewErrorView(error)).toEqual({
      kind: "field",
      message: "Nominal lebih dari total tagihan.",
    });
  });

  it("shows the API's message for the rate limit", () => {
    expect(previewErrorView(apiError(429, "rate_limited"))).toEqual({
      kind: "alert",
      message: "Pesan.",
    });
  });

  it("shows the generic message for anything else", () => {
    expect(previewErrorView(apiError(500, "internal"))).toEqual({
      kind: "alert",
      message: GENERIC_ERROR_MESSAGE,
    });
    expect(previewErrorView(new Error("boom"))).toEqual({
      kind: "alert",
      message: GENERIC_ERROR_MESSAGE,
    });
  });
});

function openView(overrides: Partial<PaymentPage> = {}): OpenPaymentPageView {
  const view = paymentPageView(page(overrides));
  if (view.status !== "open") throw new Error("expected an open page");
  return view;
}

const OFFER: PromiseOfferView = {
  outstanding: 2_250_000,
  earliest: "2026-11-03",
  latest: "2027-01-01",
  nextInvoiceDueDate: "2026-12-01",
};

const NO_NEXT_OFFER: PromiseOfferView = { ...OFFER, nextInvoiceDueDate: null };

function promise(overrides: Partial<PaymentPromise> = {}): PaymentPromise {
  return {
    status: "active",
    version: 1,
    kind: "with_next_invoice",
    promisedDate: "2026-12-01",
    promisedAmount: null,
    payingNow: 0,
    createdAt: "2026-11-02T03:00:00Z",
    ...overrides,
  };
}

const PAYING = {
  amount: 1_000_000,
  text: "1.000.000",
  outstandingAfter: 1_250_000,
};

const IDLE_MODEL: QrisModel = { ...INITIAL_QRIS_MODEL, card: { kind: "idle" } };
const PAID_MODEL: QrisModel = {
  ...INITIAL_QRIS_MODEL,
  card: { kind: "idle", paid: true },
};

describe("Bayar-Pilih choices", () => {
  it("offers all three with QRIS and promise dates", () => {
    expect(payChoices(openView({ qrisAvailable: true }))).toEqual([
      {
        value: "all",
        label: "Bayar semua",
        sub: "Rp2.250.000, langsung lunas",
      },
      {
        value: "partial",
        label: "Bayar sebagian",
        sub: "Tentukan sendiri nominalnya, berapa pun",
      },
      {
        value: "later",
        label: "Belum bisa bayar sekarang",
        sub: "Catat kapan Anda akan melunasi",
      },
    ]);
  });

  it("leaves out Bayar sebagian without QRIS", () => {
    expect(
      payChoices(openView({ qrisAvailable: false })).map((c) => c.value),
    ).toEqual(["all", "later"]);
  });

  it("leaves out Belum bisa bayar without promise dates", () => {
    expect(
      payChoices(openView({ qrisAvailable: true, promiseDates: null })).map(
        (c) => c.value,
      ),
    ).toEqual(["all", "partial"]);
  });

  it("names the total on the primary of Bayar semua, Lanjut otherwise", () => {
    expect(choicePrimaryLabel("all", "Rp2.250.000")).toBe(
      "Lanjut bayar Rp2.250.000",
    );
    expect(choicePrimaryLabel("partial", "Rp2.250.000")).toBe("Lanjut");
    expect(choicePrimaryLabel("later", "Rp2.250.000")).toBe("Lanjut");
  });

  it("reads only the three choices from the radio group", () => {
    expect(toPayChoice("all")).toBe("all");
    expect(toPayChoice("later")).toBe("later");
    expect(toPayChoice("semua")).toBeNull();
  });

  it("falls back to Bayar semua when the picked choice is gone", () => {
    const choices = payChoices(openView({ qrisAvailable: false }));
    expect(offeredChoice("partial", choices)).toBe("all");
    expect(offeredChoice("later", choices)).toBe("later");
  });

  it("starts on the choice, or on paying when it is the only one", () => {
    expect(initialFlowMode(openView({ qrisAvailable: true }))).toEqual(
      choiceMode("all"),
    );
    expect(
      initialFlowMode(openView({ qrisAvailable: false, promiseDates: null })),
    ).toBe(PAY_MODE);
  });

  it("offers Kembali on the pay view only with a choice and no QR in flight", () => {
    const view = openView({ qrisAvailable: true });
    expect(showsPayBack(view, { kind: "idle" })).toBe(true);
    expect(showsPayBack(view, { kind: "expired", amount: 1 })).toBe(true);
    expect(
      showsPayBack(view, {
        kind: "pending",
        amount: 1,
        expiresAt: "2026-11-06T10:00:00Z",
      }),
    ).toBe(false);
    expect(showsPayBack(view, { kind: "creating" })).toBe(false);
    expect(showsPayBack(openView({ qrisAvailable: false }), null)).toBe(true);
    expect(
      showsPayBack(
        openView({ qrisAvailable: false, promiseDates: null }),
        null,
      ),
    ).toBe(false);
  });
});

describe("Bayar-Pilih transitions", () => {
  const view = openView({ qrisAvailable: true });

  it("goes to the pay view, the entry or the Janji screen", () => {
    expect(nextFromChoice("all", view)).toBe(PAY_MODE);
    expect(nextFromChoice("partial", view)).toEqual({
      kind: "entry",
      text: "",
    });
    expect(nextFromChoice("later", view)).toEqual({
      kind: "promise",
      draft: { kind: "with_next_invoice", date: "", amountText: "" },
      paying: null,
      editing: false,
      back: choiceMode("later"),
    });
  });

  it("never goes to a view that is not offered", () => {
    expect(nextFromChoice("partial", openView({ qrisAvailable: false }))).toBe(
      PAY_MODE,
    );
    expect(
      nextFromChoice(
        "later",
        openView({ qrisAvailable: true, promiseDates: null }),
      ),
    ).toBe(PAY_MODE);
  });

  it("skips the choice when a QR comes back on load", () => {
    const pending: QrisModel = {
      ...INITIAL_QRIS_MODEL,
      card: { kind: "pending", amount: 1, expiresAt: "2026-11-06T10:00:00Z" },
    };
    const checking: QrisModel = {
      ...INITIAL_QRIS_MODEL,
      card: { kind: "checking", amount: 1, checkedAt: null },
    };
    expect(
      flowModeAfterCard(choiceMode("all"), { kind: "resuming" }, pending),
    ).toBe(PAY_MODE);
    expect(
      flowModeAfterCard(choiceMode("all"), { kind: "resuming" }, checking),
    ).toBe(PAY_MODE);
    const choice = choiceMode("all");
    expect(flowModeAfterCard(choice, { kind: "resuming" }, IDLE_MODEL)).toBe(
      choice,
    );
    expect(flowModeAfterCard(choice, { kind: "idle" }, pending)).toBe(choice);
  });
});

describe("Bayar sebagian, then the promise", () => {
  const preview: PreviewView = previewView(PREVIEW);
  const entry: PayFlowMode = { kind: "entry", text: "1.000.000" };

  it("goes on to the Janji screen when something stays owed", () => {
    expect(chooseFromEntry(entry, preview, OFFER)).toEqual({
      kind: "promise",
      draft: { kind: "with_next_invoice", date: "", amountText: "" },
      paying: PAYING,
      editing: false,
      back: entry,
    });
  });

  it("goes straight to the QR when the amount pays everything", () => {
    expect(
      chooseFromEntry(entry, { ...preview, outstandingAfter: 0 }, OFFER),
    ).toEqual({
      kind: "chosen",
      text: "1.000.000",
      amount: 1_000_000,
      outstandingAfter: 0,
      promise: null,
    });
  });

  it("goes straight to the QR when no promise can be made", () => {
    expect(chooseFromEntry(entry, preview, null)).toEqual({
      kind: "chosen",
      text: "1.000.000",
      amount: 1_000_000,
      outstandingAfter: 1_250_000,
      promise: null,
    });
    expect(chooseFromEntry(PAY_MODE, preview, OFFER)).toBe(PAY_MODE);
  });

  it("names the next step on the primary while something stays owed", () => {
    const base = { outstanding: 2_250_000, promiseOffered: true };
    const toPromise = entryLeadsToPromise({
      ...base,
      amount: 1_000_000,
      preview,
    });
    expect(toPromise).toBe(true);
    expect(entryPrimaryLabel(1_000_000, toPromise)).toBe(
      "Lanjut pilih tanggal pelunasan",
    );
    expect(entryNextLine(preview, toPromise)).toBe(
      "Berikutnya: pilih kapan sisa Rp1.250.000 dilunasi.",
    );
  });

  it("keeps Bayar QRIS when the amount pays everything", () => {
    const paysAll = { ...preview, amount: 2_250_000, outstandingAfter: 0 };
    const toPromise = entryLeadsToPromise({
      amount: 2_250_000,
      preview: paysAll,
      outstanding: 2_250_000,
      promiseOffered: true,
    });
    expect(toPromise).toBe(false);
    expect(entryPrimaryLabel(2_250_000, toPromise)).toBe(
      "Bayar QRIS Rp2.250.000",
    );
    expect(entryNextLine(paysAll, toPromise)).toBeNull();
  });

  it("compares with what is owed while the preview is for another amount", () => {
    const input = { preview, outstanding: 2_250_000, promiseOffered: true };
    expect(entryLeadsToPromise({ ...input, amount: 500_000 })).toBe(true);
    expect(entryLeadsToPromise({ ...input, amount: 2_250_000 })).toBe(false);
    expect(entryLeadsToPromise({ ...input, amount: null })).toBe(false);
  });

  it("never leads to a promise when none can be made", () => {
    expect(
      entryLeadsToPromise({
        amount: 1_000_000,
        preview,
        outstanding: 2_250_000,
        promiseOffered: false,
      }),
    ).toBe(false);
  });

  it("goes back from the Janji screen to the entry with the amount kept", () => {
    const mode = chooseFromEntry(entry, preview, OFFER);
    expect(backFromPromise(mode)).toEqual(entry);
    expect(backFromPromise(PAY_MODE)).toBe(PAY_MODE);
  });
});

describe("the promise offer and the active promise", () => {
  it("is built from the page as is", () => {
    expect(promiseOfferView(page())).toEqual(OFFER);
    expect(promiseOfferView(page({ promiseDates: null }))).toBeNull();
    expect(promiseOfferView(page({ nextInvoiceDueDate: null }))).toEqual(
      NO_NEXT_OFFER,
    );
  });

  it("shows the active promise's date and amount", () => {
    const active = promise();
    expect(activePromiseView(active)).toEqual({
      text: "Janji pelunasan 1 Des 2026 · Semua sisa tagihan",
      promise: active,
    });
    expect(
      activePromiseView(
        promise({ kind: "specific_date", promisedAmount: 500_000 }),
      )?.text,
    ).toBe("Janji pelunasan 1 Des 2026 · Rp500.000");
  });

  it("shows no card without an active promise", () => {
    expect(activePromiseView(null)).toBeNull();
    expect(activePromiseView(promise({ status: "kept" }))).toBeNull();
    expect(openView({ promise: promise() }).activePromise?.text).toBe(
      "Janji pelunasan 1 Des 2026 · Semua sisa tagihan",
    );
  });
});

describe("the Janji screen", () => {
  it("offers Bareng tagihan with the next due date, then Pilih tanggal sendiri", () => {
    expect(promiseFormView(OFFER, null, false, null)).toEqual({
      replacesLine: null,
      subline: "Total tagihan Rp2.250.000",
      kindOptions: [
        {
          value: "with_next_invoice",
          label: "Bareng tagihan berikutnya",
          sub: "Jatuh tempo 1 Des 2026",
        },
        {
          value: "specific_date",
          label: "Pilih tanggal sendiri",
          sub: "Paling lambat 1 Jan 2027, tanpa denda",
        },
      ],
      dateMin: "2026-11-03",
      dateMax: "2027-01-01",
      amountHint: "Kosongkan untuk semua sisa, Rp2.250.000.",
      primaryLabel: "Simpan janji",
    });
  });

  it("hides Bareng tagihan without a next due date", () => {
    expect(
      promiseFormView(NO_NEXT_OFFER, null, false, null).kindOptions.map(
        (o) => o.value,
      ),
    ).toEqual(["specific_date"]);
  });

  it("speaks of the rest after paying now", () => {
    const view = promiseFormView(OFFER, PAYING, false, null);
    expect(view.subline).toBe("Sisa setelah bayar hari ini: Rp1.250.000");
    expect(view.amountHint).toBe("Kosongkan untuk semua sisa, Rp1.250.000.");
    expect(view.primaryLabel).toBe("Simpan janji, lalu bayar Rp1.000.000");
  });

  it("saves changes when editing", () => {
    expect(promiseFormView(OFFER, null, true, null).primaryLabel).toBe(
      "Simpan perubahan",
    );
  });

  it("says a new form replaces the active promise, but not on Ubah janji", () => {
    const all = promise({ promisedDate: "2026-12-01", promisedAmount: null });
    expect(promiseFormView(OFFER, null, false, all).replacesLine).toBe(
      "Janji sebelumnya 1 Des 2026, semua sisa tagihan akan diganti.",
    );
    expect(
      promiseFormView(
        OFFER,
        PAYING,
        false,
        promise({ promisedDate: "2026-12-01", promisedAmount: 500_000 }),
      ).replacesLine,
    ).toBe("Janji sebelumnya 1 Des 2026, Rp500.000 akan diganti.");
    expect(promiseFormView(OFFER, null, true, all).replacesLine).toBeNull();
    expect(promiseFormView(OFFER, null, false, null).replacesLine).toBeNull();
  });

  it("defaults to Bareng tagihan when offered, else Pilih tanggal sendiri", () => {
    expect(defaultPromiseDraft(OFFER)).toEqual({
      kind: "with_next_invoice",
      date: "",
      amountText: "",
    });
    expect(defaultPromiseDraft(NO_NEXT_OFFER).kind).toBe("specific_date");
  });

  it("falls back to Pilih tanggal sendiri when Bareng tagihan goes away", () => {
    expect(offeredPromiseKind("with_next_invoice", NO_NEXT_OFFER)).toBe(
      "specific_date",
    );
    expect(offeredPromiseKind("with_next_invoice", OFFER)).toBe(
      "with_next_invoice",
    );
  });

  it("reads only the two kinds from the radio group", () => {
    expect(toPromiseKind("specific_date")).toBe("specific_date");
    expect(toPromiseKind("next")).toBeNull();
  });

  it("fills Ubah janji from the active promise", () => {
    expect(
      prefilledPromiseDraft(
        promise({
          kind: "specific_date",
          promisedDate: "2026-11-20",
          promisedAmount: 500_000,
        }),
        OFFER,
      ),
    ).toEqual({
      kind: "specific_date",
      date: "2026-11-20",
      amountText: "500.000",
    });
    expect(prefilledPromiseDraft(promise(), OFFER)).toEqual({
      kind: "with_next_invoice",
      date: "",
      amountText: "",
    });
    expect(prefilledPromiseDraft(promise(), NO_NEXT_OFFER)).toEqual({
      kind: "specific_date",
      date: "2026-12-01",
      amountText: "",
    });
  });
});

// What "Simpan janji" sends for these fields, or each message by the field it goes under.
function checkPromise(
  draft: PromiseDraft,
  offer: PromiseOfferView,
  paying: PromisePaying | null,
) {
  const result = promiseSchema(offer, paying).safeParse(draft);
  if (result.success) return { ok: true, body: result.data };
  return {
    ok: false,
    errors: Object.fromEntries(
      result.error.issues.map((issue) => [issue.path.join("."), issue.message]),
    ),
  };
}

describe("promiseSchema", () => {
  it("sends Bareng tagihan without a date, the amount only when typed", () => {
    expect(
      checkPromise(
        { kind: "with_next_invoice", date: "2026-11-20", amountText: "" },
        OFFER,
        null,
      ),
    ).toEqual({ ok: true, body: { kind: "with_next_invoice" } });
    expect(
      checkPromise(
        { kind: "with_next_invoice", date: "", amountText: "500.000" },
        OFFER,
        null,
      ),
    ).toEqual({
      ok: true,
      body: { kind: "with_next_invoice", promisedAmount: 500_000 },
    });
  });

  it("sends the chosen date and what is paid now", () => {
    expect(
      checkPromise(
        { kind: "specific_date", date: "2026-11-20", amountText: "" },
        OFFER,
        PAYING,
      ),
    ).toEqual({
      ok: true,
      body: {
        kind: "specific_date",
        promisedDate: "2026-11-20",
        payingNow: 1_000_000,
      },
    });
  });

  it("asks for a date", () => {
    expect(
      checkPromise(
        { kind: "specific_date", date: "", amountText: "" },
        OFFER,
        null,
      ),
    ).toEqual({ ok: false, errors: { date: PROMISE_DATE_REQUIRED_TEXT } });
  });

  it.each([
    { name: "before the earliest", date: "2026-11-02" },
    { name: "after the latest", date: "2027-01-02" },
  ])("refuses a date $name", ({ date }) => {
    expect(
      checkPromise(
        { kind: "specific_date", date, amountText: "" },
        OFFER,
        null,
      ),
    ).toEqual({
      ok: false,
      errors: { date: "Pilih tanggal dari 3 Nov 2026 sampai 1 Jan 2027." },
    });
  });

  it("takes both bounds", () => {
    for (const date of ["2026-11-03", "2027-01-01"]) {
      expect(
        checkPromise(
          { kind: "specific_date", date, amountText: "" },
          OFFER,
          null,
        ).ok,
      ).toBe(true);
    }
  });

  it("sends a date when Bareng tagihan is no longer offered", () => {
    expect(
      checkPromise(
        { kind: "with_next_invoice", date: "", amountText: "" },
        NO_NEXT_OFFER,
        null,
      ),
    ).toEqual({ ok: false, errors: { date: PROMISE_DATE_REQUIRED_TEXT } });
    expect(
      checkPromise(
        { kind: "with_next_invoice", date: "2026-11-20", amountText: "" },
        NO_NEXT_OFFER,
        null,
      ),
    ).toEqual({
      ok: true,
      body: { kind: "specific_date", promisedDate: "2026-11-20" },
    });
  });

  it("shows only the first wrong field", () => {
    expect(
      checkPromise(
        { kind: "specific_date", date: "", amountText: "0" },
        OFFER,
        null,
      ),
    ).toEqual({ ok: false, errors: { date: PROMISE_DATE_REQUIRED_TEXT } });
  });

  it("refuses an amount of 0", () => {
    expect(
      checkPromise(
        { kind: "with_next_invoice", date: "", amountText: "0" },
        OFFER,
        null,
      ),
    ).toEqual({ ok: false, errors: { amountText: PROMISE_AMOUNT_MIN_TEXT } });
  });

  it("refuses an amount above the rest", () => {
    expect(
      checkPromise(
        { kind: "with_next_invoice", date: "", amountText: "1.250.001" },
        OFFER,
        PAYING,
      ),
    ).toEqual({
      ok: false,
      errors: { amountText: "Nominal janji paling banyak Rp1.250.000." },
    });
    expect(
      checkPromise(
        { kind: "with_next_invoice", date: "", amountText: "1.250.000" },
        OFFER,
        PAYING,
      ).ok,
    ).toBe(true);
  });
});

describe("promiseFormError", () => {
  it("puts each field the API names under the form field showing it", () => {
    const refused = (field: string) =>
      new ApiError({
        status: 400,
        code: "validation_failed",
        message: "Periksa lagi.",
        field,
      });
    expect(promiseFormError(refused("kind"), "specific_date").field).toBe(
      "kind",
    );
    expect(
      promiseFormError(refused("promisedDate"), "specific_date").field,
    ).toBe("date");
    expect(
      promiseFormError(refused("promisedDate"), "with_next_invoice").field,
    ).toBeNull();
    expect(
      promiseFormError(refused("promisedAmount"), "specific_date"),
    ).toEqual({ field: "amountText", message: "Periksa lagi." });
    expect(
      promiseFormError(refused("payingNow"), "specific_date").field,
    ).toBeNull();
  });

  it("shows anything that is not the API's answer in the alert", () => {
    expect(promiseFormError(new Error("boom"), "specific_date")).toEqual({
      field: null,
      message: GENERIC_ERROR_MESSAGE,
    });
  });
});

describe("promiseErrorView", () => {
  function fieldError(status: number, code: string, field?: string) {
    return new ApiError({ status, code, message: "Pesan API.", field });
  }

  it.each([
    { field: "kind", target: "kind" },
    { field: "promisedDate", target: "promisedDate" },
    { field: "promisedAmount", target: "promisedAmount" },
    { field: "payingNow", target: "alert" },
  ])("puts a 400 on $field under $target", ({ field, target }) => {
    expect(
      promiseErrorView(
        fieldError(400, "invalid_input", field),
        "specific_date",
      ),
    ).toEqual({
      kind: "shown",
      error: { target, message: "Pesan API." },
      refetchPage: false,
    });
  });

  it("puts a date error in the alert while the date input is hidden", () => {
    const view = promiseErrorView(
      fieldError(400, "promise_date_out_of_range", "promisedDate"),
      "with_next_invoice",
    );
    expect(view).toEqual({
      kind: "shown",
      error: { target: "alert", message: "Pesan API." },
      refetchPage: true,
    });
  });

  it("reads the page again when it is out of date", () => {
    expect(
      promiseErrorView(
        fieldError(400, "promise_no_next_invoice", "kind"),
        "with_next_invoice",
      ),
    ).toEqual({
      kind: "shown",
      error: { target: "kind", message: "Pesan API." },
      refetchPage: true,
    });
    expect(
      promiseErrorView(
        fieldError(400, "promise_amount_above_outstanding", "payingNow"),
        "with_next_invoice",
      ),
    ).toEqual({
      kind: "shown",
      error: { target: "alert", message: "Pesan API." },
      refetchPage: true,
    });
    expect(
      promiseErrorView(
        fieldError(409, "promise_nothing_owed"),
        "with_next_invoice",
      ),
    ).toEqual({
      kind: "shown",
      error: { target: "alert", message: "Pesan API." },
      refetchPage: true,
    });
  });

  it("shows a rate limit's message in the alert, without reading again", () => {
    expect(
      promiseErrorView(fieldError(429, "rate_limited"), "specific_date"),
    ).toEqual({
      kind: "shown",
      error: { target: "alert", message: "Pesan API." },
      refetchPage: false,
    });
  });

  it("replaces the page when the link stopped working", () => {
    expect(
      promiseErrorView(
        apiError(404, "payment_link_not_found"),
        "specific_date",
      ),
    ).toEqual({ kind: "link-gone" });
    expect(
      promiseErrorView(apiError(410, "payment_link_expired"), "specific_date"),
    ).toEqual({ kind: "link-gone" });
  });

  it("shows the generic message for anything that is not the API's", () => {
    expect(promiseErrorView(new Error("boom"), "specific_date")).toEqual({
      kind: "shown",
      error: { target: "alert", message: GENERIC_ERROR_MESSAGE },
      refetchPage: false,
    });
  });
});

describe("after Simpan janji", () => {
  const saved = promise({
    kind: "specific_date",
    promisedDate: "2026-11-20",
    payingNow: 1_000_000,
  });
  const entry: PayFlowMode = { kind: "entry", text: "1.000.000" };

  it("pays the chosen amount next, with the promise on its card", () => {
    const mode: PayFlowMode = {
      kind: "promise",
      draft: defaultPromiseDraft(OFFER),
      paying: PAYING,
      editing: false,
      back: entry,
    };
    expect(promiseSaved(mode, saved)).toEqual({
      kind: "chosen",
      text: "1.000.000",
      amount: 1_000_000,
      outstandingAfter: 1_250_000,
      promise: saved,
    });
  });

  it("shows Janji tercatat when nothing is paid now", () => {
    const mode = nextFromChoice("later", openView({ qrisAvailable: true }));
    expect(promiseSaved(mode, saved)).toEqual({
      kind: "recorded",
      paidAmount: null,
      promise: saved,
    });
    expect(promiseSaved(PAY_MODE, saved)).toBe(PAY_MODE);
  });

  it("opens Ubah janji prefilled, then shows Janji tercatat again", () => {
    const recorded: PayFlowMode = {
      kind: "recorded",
      paidAmount: 1_000_000,
      promise: saved,
    };
    const editing = editPromise(saved, OFFER, recorded);
    expect(editing).toEqual({
      kind: "promise",
      draft: { kind: "specific_date", date: "2026-11-20", amountText: "" },
      paying: null,
      editing: true,
      back: recorded,
    });
    expect(backFromPromise(editing)).toBe(recorded);
    const edited = promise({ promisedDate: "2026-11-25", version: 2 });
    expect(promiseSaved(editing, edited)).toEqual({
      kind: "recorded",
      paidAmount: 1_000_000,
      promise: edited,
    });
  });

  it("opens Ubah janji from the active promise card and returns to the choice", () => {
    const active = promise();
    const editing = editPromise(active, OFFER, choiceMode("all"));
    expect(backFromPromise(editing)).toEqual(choiceMode("all"));
    expect(promiseSaved(editing, active)).toEqual({
      kind: "recorded",
      paidAmount: null,
      promise: active,
    });
  });
});

describe("the chosen amount with a promise", () => {
  it("says when the rest is promised", () => {
    expect(
      chosenPromiseLine(
        1_250_000,
        promise({ promisedDate: "2026-12-01", promisedAmount: null }),
      ),
    ).toBe("Sisa Rp1.250.000 dijanjikan Selasa, 1 Desember 2026.");
  });

  it("names a promise of part of the rest", () => {
    expect(
      chosenPromiseLine(
        1_250_000,
        promise({ promisedDate: "2026-12-01", promisedAmount: 500_000 }),
      ),
    ).toBe("Sisa Rp1.250.000, Rp500.000 dijanjikan Selasa, 1 Desember 2026.");
  });

  it("keeps the old line without a promise", () => {
    expect(
      chosenRemainder({
        kind: "chosen",
        text: "1.000.000",
        amount: 1_000_000,
        outstandingAfter: 1_250_000,
        promise: null,
      }),
    ).toBe("Sisa Rp1.250.000 tetap jadi tagihan.");
    expect(
      chosenRemainder({
        kind: "chosen",
        text: "1.000.000",
        amount: 1_000_000,
        outstandingAfter: 1_250_000,
        promise: promise(),
      }),
    ).toBe("Sisa Rp1.250.000 dijanjikan Selasa, 1 Desember 2026.");
  });
});

describe("chosen amount modes", () => {
  const entry: PayFlowMode = { kind: "entry", text: "1.000.000" };
  const chosen: PayFlowMode = {
    kind: "chosen",
    text: "1.000.000",
    amount: 1_000_000,
    outstandingAfter: 1_250_000,
    promise: null,
  };
  const saved = promise({ promisedDate: "2026-12-01" });
  const promised: PayFlowMode = { ...chosen, promise: saved };

  it("goes back to the entry with the amount kept", () => {
    expect(changePartialAmount(chosen)).toEqual(entry);
    expect(changePartialAmount(PAY_MODE)).toBe(PAY_MODE);
  });

  it("offers Ubah nominal only while no QR is in flight", () => {
    expect(canChangePartialAmount({ kind: "idle" })).toBe(true);
    expect(canChangePartialAmount({ kind: "expired", amount: 1 })).toBe(true);
    expect(canChangePartialAmount({ kind: "unavailable" })).toBe(true);
    expect(canChangePartialAmount({ kind: "creating" })).toBe(false);
    expect(
      canChangePartialAmount({
        kind: "pending",
        amount: 1,
        expiresAt: "2026-11-06T10:00:00Z",
      }),
    ).toBe(false);
    expect(
      canChangePartialAmount({ kind: "checking", amount: 1, checkedAt: null }),
    ).toBe(false);
  });

  it.each<{ name: string; previous: QrisCard }>([
    {
      name: "pending",
      previous: {
        kind: "pending",
        amount: 1_000_000,
        expiresAt: "2026-11-06T10:00:00Z",
      },
    },
    {
      name: "checking",
      previous: { kind: "checking", amount: 1_000_000, checkedAt: null },
    },
    { name: "creating", previous: { kind: "creating" } },
    { name: "waiting", previous: { kind: "waiting", until: 0 } },
  ])("returns to the choice when a $name QR ends paid", ({ previous }) => {
    expect(flowModeAfterCard(chosen, previous, PAID_MODEL)).toEqual(
      choiceMode("all"),
    );
  });

  it("shows Janji tercatat with what was paid when the rest was promised", () => {
    expect(
      flowModeAfterCard(
        promised,
        { kind: "pending", amount: 999_000, expiresAt: "2026-11-06T10:00:00Z" },
        PAID_MODEL,
      ),
    ).toEqual({ kind: "recorded", paidAmount: 999_000, promise: saved });
    expect(
      flowModeAfterCard(promised, { kind: "creating" }, PAID_MODEL),
    ).toEqual({ kind: "recorded", paidAmount: 1_000_000, promise: saved });
  });

  it("keeps the chosen amount when its QR is gone unpaid, so nothing is called paid", () => {
    // Status "none" or a check's qris_not_found: idle, not paid, no alert.
    const live: QrisCard = {
      kind: "pending",
      amount: 1_000_000,
      expiresAt: "2026-11-06T10:00:00Z",
    };
    expect(flowModeAfterCard(promised, live, IDLE_MODEL)).toBe(promised);
    expect(flowModeAfterCard(chosen, live, IDLE_MODEL)).toBe(chosen);
    expect(
      flowModeAfterCard(
        chosen,
        { kind: "checking", amount: 1_000_000, checkedAt: null },
        IDLE_MODEL,
      ),
    ).toBe(chosen);
  });

  it("keeps the chosen amount when a create fails with an alert", () => {
    expect(
      flowModeAfterCard(
        promised,
        { kind: "creating" },
        { ...IDLE_MODEL, alert: QRIS_CREATE_FAILED_TEXT },
      ),
    ).toBe(promised);
  });

  it("keeps the chosen amount while the QR goes on or expires", () => {
    const pending: QrisModel = {
      ...INITIAL_QRIS_MODEL,
      card: { kind: "pending", amount: 1, expiresAt: "2026-11-06T10:00:00Z" },
    };
    expect(flowModeAfterCard(chosen, { kind: "creating" }, pending)).toBe(
      chosen,
    );
    expect(
      flowModeAfterCard(chosen, pending.card, {
        ...INITIAL_QRIS_MODEL,
        card: { kind: "expired", amount: 1 },
      }),
    ).toBe(chosen);
  });

  it("leaves other views alone", () => {
    expect(flowModeAfterCard(entry, { kind: "creating" }, IDLE_MODEL)).toBe(
      entry,
    );
    expect(flowModeAfterCard(PAY_MODE, { kind: "creating" }, IDLE_MODEL)).toBe(
      PAY_MODE,
    );
    expect(flowModeAfterCard(chosen, { kind: "resuming" }, IDLE_MODEL)).toBe(
      chosen,
    );
  });
});

describe("Janji tercatat", () => {
  const saved = promise({
    kind: "specific_date",
    promisedDate: "2026-11-20",
    promisedAmount: 500_000,
  });

  it("thanks the tenant for what this visit paid and shows what is left", () => {
    expect(
      promiseRecordedView(openView({ outstanding: 1_250_000 }), {
        kind: "recorded",
        paidAmount: 1_000_000,
        promise: saved,
      }),
    ).toEqual({
      paidLine: "Pembayaran Rp1.000.000 sudah diterima. Terima kasih, Bu Rina.",
      outstandingLine: "Sisa tagihan Rp1.250.000",
      date: "Jumat, 20 November 2026",
      amount: "Rp500.000",
    });
  });

  it("has no paid line when nothing was paid, and promises all the rest", () => {
    const view = promiseRecordedView(openView(), {
      kind: "recorded",
      paidAmount: null,
      promise: promise({ promisedAmount: null }),
    });
    expect(view.paidLine).toBeNull();
    expect(view.amount).toBe("Semua sisa tagihan");
    expect(view.outstandingLine).toBe("Sisa tagihan Rp2.250.000");
  });

  it("leaves out what is left when nothing is", () => {
    expect(
      promiseRecordedView(openView({ outstanding: 0 }), {
        kind: "recorded",
        paidAmount: null,
        promise: saved,
      }).outstandingLine,
    ).toBeNull();
  });
});

describe("checkPromiseDraft", () => {
  it("makes the request, or names the first wrong field", () => {
    expect(
      checkPromiseDraft(
        { kind: "specific_date", date: OFFER.earliest, amountText: "1.000" },
        OFFER,
        null,
      ),
    ).toEqual({
      ok: true,
      body: {
        kind: "specific_date",
        promisedDate: OFFER.earliest,
        promisedAmount: 1_000,
      },
    });
    expect(
      checkPromiseDraft(
        { kind: "specific_date", date: "", amountText: "0" },
        OFFER,
        null,
      ),
    ).toEqual({
      ok: false,
      field: "date",
      message: "Pilih tanggal pelunasan.",
    });
  });

  it("sends a kind no longer offered as a date of the tenant's own", () => {
    expect(
      checkPromiseDraft(
        { kind: "with_next_invoice", date: "", amountText: "" },
        NO_NEXT_OFFER,
        null,
      ),
    ).toMatchObject({ ok: false, field: "date" });
  });
});
