import { describe, expect, it } from "vitest";

import {
  ApiError,
  GENERIC_ERROR_MESSAGE,
  NETWORK_ERROR_CODE,
} from "@/lib/api/errors";
import type {
  CapabilityAccess,
  FieldCashPayment,
  FieldPromiseVersion,
  FieldVisit,
  VisitOutcome,
} from "@/lib/api/generated/model";
import { createAppAbility } from "@/lib/capabilities";

import type { TenantOpening } from "./type";
import {
  CHOOSE_OUTCOME_MESSAGE,
  EMPTY_PHONE_MESSAGE,
  NOTE_TOO_LONG_MESSAGE,
  OUTCOME_OPTIONS,
  UNKNOWN_SAVE_TEXT,
  canRecordVisit,
  canSaveOutcome,
  cashChoices,
  defaultOutcome,
  directionsUrl,
  doneChip,
  doneLine,
  fieldVisitRow,
  fieldVisitSections,
  headerDateLine,
  markOpening,
  nextOpenings,
  notHomeLine,
  promiseSummary,
  reasonChip,
  receiveCashTenant,
  selectedPaymentId,
  visitErrorView,
  visitOutcomeBody,
  visitOutcomeSchema,
} from "./utils";

const REQUEST_ID = "6f0c1d2e-3a4b-4c5d-8e6f-7a8b9c0d1e2f";

function cash(
  paymentId: string,
  amount: number,
  recordedAt: string,
  status: FieldCashPayment["photo"]["status"] = "clean",
): FieldCashPayment {
  return { paymentId, amount, recordedAt, photo: { status } };
}

function outcome(
  kind: VisitOutcome["outcome"],
  recordedAt = "2026-11-16T03:20:00Z",
): VisitOutcome {
  return {
    id: "o-1",
    tenantId: "t-1",
    outcome: kind,
    recordedAt,
    resolved: [],
  };
}

const PROMISE: FieldPromiseVersion = {
  promiseVersionId: "v-2",
  promiseId: "p-1",
  version: 2,
  kind: "specific_date",
  promisedDate: "2026-11-20",
  promisedAmount: null,
  createdAt: "2026-11-16T03:31:00Z",
};

// Pak Hendra Gunawan as on the board: two broken promises and debt past due.
const HENDRA: FieldVisit = {
  tenantId: "t-1",
  tenantName: "Pak Hendra Gunawan",
  units: [
    {
      unitLabel: "Kontrakan No. 4",
      propertyName: "Kontrakan Seturan",
      propertyAddress: "Jl. Seturan Raya No. 4, Sleman",
    },
  ],
  reasons: [
    { code: "overdue", daysOverdue: 9 },
    { code: "broken_promises", brokenPromises: 2 },
  ],
  outstanding: 2_900_000,
  overdue: 2_200_000,
  oldestUnpaidDueDate: "2026-11-07",
  status: "open",
  unlinkedCashToday: [],
  unlinkedPromiseToday: null,
};

function apiError(
  status: number,
  code: string,
  message: string,
  field?: string,
): ApiError {
  return new ApiError({ status, code, message, field });
}

describe("canRecordVisit", () => {
  it.each<[CapabilityAccess, boolean]>([
    ["allow", true],
    ["conditional", true],
    ["deny", false],
  ])("visit:record %s gives %s", (access, shown) => {
    const ability = createAppAbility([{ action: "visit:record", access }]);
    expect(canRecordVisit(ability)).toBe(shown);
  });

  it("is hidden for a member who only reads visits", () => {
    const ability = createAppAbility([
      { action: "visit:read", access: "allow" },
    ]);
    expect(canRecordVisit(ability)).toBe(false);
  });
});

describe("headerDateLine", () => {
  it("names today in Jakarta and how the list is made", () => {
    expect(headerDateLine(new Date("2026-11-16T01:00:00Z"))).toBe(
      "Senin, 16 Nov · disusun otomatis setiap pagi",
    );
  });
});

describe("reasonChip", () => {
  it.each([
    {
      name: "overdue with its days",
      reason: { code: "overdue", daysOverdue: 9 },
      want: "Telat 9 hari",
    },
    {
      name: "overdue no more",
      reason: { code: "overdue", daysOverdue: 0 },
      want: "Telat",
    },
    {
      name: "broken promises with their count",
      reason: { code: "broken_promises", brokenPromises: 2 },
      want: "Janji lewat 2×",
    },
    {
      name: "broken promises without a count",
      reason: { code: "broken_promises" },
      want: "Janji lewat",
    },
    {
      name: "a number WhatsApp cannot reach",
      reason: { code: "undeliverable" },
      want: "WA gagal terkirim",
    },
  ] as const)("$name", ({ reason, want }) => {
    expect(reasonChip(reason)).toEqual({ label: want, tone: "warning" });
  });
});

describe("outcome lines", () => {
  it("says when nobody was found, in Jakarta time", () => {
    expect(notHomeLine(outcome("not_home", "2026-11-16T03:20:00Z"))).toBe(
      "Tadi tidak bertemu, 10.20",
    );
    expect(notHomeLine(outcome("paid"))).toBeNull();
    expect(notHomeLine(undefined)).toBeNull();
  });

  it.each([
    { kind: "paid", want: { label: "Sudah bayar", tone: "active" } },
    { kind: "promise", want: { label: "Janji bayar", tone: "neutral" } },
    { kind: "number_changed", want: { label: "Nomor baru", tone: "neutral" } },
    { kind: "not_home", want: null },
  ] as const)("chips a done $kind row", ({ kind, want }) => {
    expect(doneChip(outcome(kind))).toEqual(want);
  });

  it("names the cash amount of a paid row with the time it was recorded", () => {
    expect(
      doneLine({
        ...outcome("paid", "2026-11-16T02:15:00Z"),
        amount: 950_000,
      }),
    ).toBe("Rp950.000 tunai · 09.15");
  });

  it("says only when it was recorded without an amount, and for other outcomes", () => {
    expect(doneLine(outcome("paid", "2026-11-16T02:15:00Z"))).toBe(
      "Tunai dicatat 09.15",
    );
    expect(
      doneLine({
        ...outcome("promise", "2026-11-16T02:15:00Z"),
        amount: 950_000,
      }),
    ).toBe("Dicatat 09.15");
    expect(doneLine(outcome("promise", "2026-11-16T02:15:00Z"))).toBe(
      "Dicatat 09.15",
    );
    expect(doneLine(undefined)).toBeNull();
  });
});

describe("directionsUrl", () => {
  it("searches the address in Google Maps, encoded", () => {
    expect(directionsUrl(HENDRA.units)).toBe(
      "https://www.google.com/maps/search/?api=1&query=Jl.%20Seturan%20Raya%20No.%204%2C%20Sleman",
    );
  });

  it("takes the first unit with an address", () => {
    expect(
      directionsUrl([
        {
          unitLabel: "Ruko 1",
          propertyName: "Ruko Baru",
          propertyAddress: " ",
        },
        ...HENDRA.units,
      ]),
    ).toContain("Jl.%20Seturan");
  });

  it("is null without an address", () => {
    expect(
      directionsUrl([
        {
          unitLabel: "Kamar 2",
          propertyName: "Kos Melati",
          propertyAddress: "",
        },
      ]),
    ).toBeNull();
    expect(directionsUrl([])).toBeNull();
  });
});

describe("fieldVisitRow", () => {
  it("shows an open visit as on the board", () => {
    expect(fieldVisitRow(HENDRA)).toEqual({
      tenantId: "t-1",
      tenantName: "Pak Hendra Gunawan",
      place: "Kontrakan No. 4 · Kontrakan Seturan",
      owed: "Sisa Rp2.900.000 · telat Rp2.200.000",
      reasons: [
        { label: "Telat 9 hari", tone: "warning" },
        { label: "Janji lewat 2×", tone: "warning" },
      ],
      notHomeLine: null,
      doneChip: null,
      doneLine: null,
      directionsUrl: directionsUrl(HENDRA.units),
      done: false,
      visit: HENDRA,
    });
  });

  it("says only Sisa when nothing is overdue", () => {
    expect(fieldVisitRow({ ...HENDRA, overdue: 0 }).owed).toBe(
      "Sisa Rp2.900.000",
    );
  });

  it("keeps a not_home row open with its line", () => {
    const row = fieldVisitRow({
      ...HENDRA,
      lastOutcome: outcome("not_home", "2026-11-16T03:20:00Z"),
    });
    expect(row.done).toBe(false);
    expect(row.notHomeLine).toBe("Tadi tidak bertemu, 10.20");
  });

  it("shows a done row with its chip and time, no reasons", () => {
    const row = fieldVisitRow({
      ...HENDRA,
      status: "done",
      reasons: [],
      lastOutcome: outcome("paid", "2026-11-16T02:15:00Z"),
    });
    expect(row.done).toBe(true);
    expect(row.reasons).toEqual([]);
    expect(row.doneChip?.label).toBe("Sudah bayar");
    expect(row.doneLine).toBe("Tunai dicatat 09.15");
  });
});

describe("fieldVisitSections", () => {
  it("splits open and done rows, each in the API's order", () => {
    const done = { ...HENDRA, tenantId: "t-9", status: "done" as const };
    const second = { ...HENDRA, tenantId: "t-2" };
    const sections = fieldVisitSections([HENDRA, second, done]);
    expect(sections.open.map((row) => row.tenantId)).toEqual(["t-1", "t-2"]);
    expect(sections.done.map((row) => row.tenantId)).toEqual(["t-9"]);
  });
});

describe("receiveCashTenant", () => {
  it("passes the overdue days and amounts of the row", () => {
    expect(receiveCashTenant(HENDRA)).toEqual({
      tenantId: "t-1",
      tenantName: "Pak Hendra Gunawan",
      reason: "overdue",
      daysOverdue: 9,
      daysUntilDue: undefined,
      oldestUnpaidDueDate: "2026-11-07",
      outstanding: 2_900_000,
    });
  });

  it("names only the due date once nothing is overdue", () => {
    const tenant = receiveCashTenant({
      ...HENDRA,
      overdue: 0,
      reasons: [{ code: "broken_promises", brokenPromises: 2 }],
    });
    expect(tenant.reason).toBe("paid_today");
    expect(tenant.daysOverdue).toBeUndefined();
  });
});

describe("cash choices", () => {
  const older = cash("pay-1", 300_000, "2026-11-16T01:05:00Z", "pending");
  const newer = cash("pay-2", 1_000_000, "2026-11-16T03:42:00Z", "clean");

  it("lists the cash newest first, with its time and photo", () => {
    expect(cashChoices([older, newer])).toEqual([
      {
        paymentId: "pay-2",
        label: "Rp1.000.000 · 10.42 · foto kwitansi tersimpan",
      },
      {
        paymentId: "pay-1",
        label: "Rp300.000 · 08.05 · foto kwitansi tersimpan",
      },
    ]);
    expect(
      cashChoices([
        cash("pay-3", 50_000, "2026-11-16T03:42:00Z", "rejected"),
      ])[0]?.label,
    ).toBe("Rp50.000 · 10.42 · foto kwitansi ditolak sistem");
  });

  it("selects the newest, or the one picked while it is offered", () => {
    expect(selectedPaymentId([older, newer], null)).toBe("pay-2");
    expect(selectedPaymentId([older, newer], "pay-1")).toBe("pay-1");
    expect(selectedPaymentId([newer], "pay-1")).toBe("pay-2");
    expect(selectedPaymentId([], null)).toBeNull();
  });
});

describe("promiseSummary", () => {
  it("names the day and everything owed", () => {
    expect(promiseSummary(PROMISE)).toEqual({
      promiseVersionId: "v-2",
      terms: "Janji Jum, 20 Nov · semua sisa",
      recorded: "Dicatat 10.31",
    });
  });

  it("names the amount promised", () => {
    expect(promiseSummary({ ...PROMISE, promisedAmount: 500_000 })?.terms).toBe(
      "Janji Jum, 20 Nov · Rp500.000",
    );
  });

  it("is null without a promise today", () => {
    expect(promiseSummary(null)).toBeNull();
  });
});

describe("defaultOutcome", () => {
  it.each([
    { name: "nothing recorded", visit: HENDRA, want: "" },
    {
      name: "a promise made today",
      visit: { ...HENDRA, unlinkedPromiseToday: PROMISE },
      want: "promise",
    },
    {
      name: "cash taken today, before a promise",
      visit: {
        ...HENDRA,
        unlinkedPromiseToday: PROMISE,
        unlinkedCashToday: [cash("pay-1", 1, "2026-11-16T01:00:00Z")],
      },
      want: "paid",
    },
  ])("opens on $want with $name", ({ visit, want }) => {
    expect(defaultOutcome(visit)).toBe(want);
  });
});

describe("canSaveOutcome", () => {
  const none = { paymentId: null, promiseVersionId: null };
  it.each([
    { outcome: "", links: none, want: false },
    { outcome: "paid", links: none, want: false },
    { outcome: "paid", links: { ...none, paymentId: "pay-1" }, want: true },
    { outcome: "promise", links: none, want: false },
    {
      outcome: "promise",
      links: { ...none, promiseVersionId: "v-2" },
      want: true,
    },
    { outcome: "not_home", links: none, want: true },
    { outcome: "number_changed", links: none, want: true },
  ])("$outcome gives $want", ({ outcome, links, want }) => {
    expect(canSaveOutcome(outcome, links, false)).toBe(want);
  });

  it("is off while saving", () => {
    expect(canSaveOutcome("not_home", none, true)).toBe(false);
  });

  it("offers the four answers in the board's order", () => {
    expect(OUTCOME_OPTIONS.map((option) => option.label)).toEqual([
      "Sudah bayar hari ini",
      "Janji bayar baru",
      "Tidak bertemu",
      "Nomor WA berubah",
    ]);
  });
});

describe("visitOutcomeSchema", () => {
  it("asks for an answer", () => {
    const parsed = visitOutcomeSchema.safeParse({
      outcome: "",
      newPhone: "",
      note: "",
    });
    expect(parsed.error?.issues[0]?.message).toBe(CHOOSE_OUTCOME_MESSAGE);
  });

  it("asks for the new number", () => {
    const parsed = visitOutcomeSchema.safeParse({
      outcome: "number_changed",
      newPhone: "  ",
      note: "",
    });
    expect(parsed.error?.issues[0]).toMatchObject({
      path: ["newPhone"],
      message: EMPTY_PHONE_MESSAGE,
    });
  });

  it("refuses a note over 500 letters", () => {
    const parsed = visitOutcomeSchema.safeParse({
      outcome: "not_home",
      newPhone: "",
      note: "a".repeat(501),
    });
    expect(parsed.error?.issues[0]).toMatchObject({
      path: ["note"],
      message: NOTE_TOO_LONG_MESSAGE,
    });
  });

  it("trims the number and the note", () => {
    expect(
      visitOutcomeSchema.parse({
        outcome: "number_changed",
        newPhone: " 0812-3456-7890 ",
        note: "  pindah kerja ",
      }),
    ).toEqual({
      outcome: "number_changed",
      newPhone: "0812-3456-7890",
      note: "pindah kerja",
    });
  });
});

describe("visitOutcomeBody", () => {
  const links = { paymentId: "pay-2", promiseVersionId: "v-2" };

  it.each([
    {
      name: "paid names the cash",
      fields: { outcome: "paid", newPhone: "0812", note: "" },
      want: { requestId: REQUEST_ID, outcome: "paid", paymentId: "pay-2" },
    },
    {
      name: "promise names the version",
      fields: { outcome: "promise", newPhone: "", note: "gajian tgl 20" },
      want: {
        requestId: REQUEST_ID,
        outcome: "promise",
        promiseVersionId: "v-2",
        note: "gajian tgl 20",
      },
    },
    {
      name: "not_home carries only the note",
      fields: { outcome: "not_home", newPhone: "0812", note: "" },
      want: { requestId: REQUEST_ID, outcome: "not_home" },
    },
    {
      name: "number_changed carries the number as typed",
      fields: {
        outcome: "number_changed",
        newPhone: "0812-3456-7890",
        note: "",
      },
      want: {
        requestId: REQUEST_ID,
        outcome: "number_changed",
        newPhone: "0812-3456-7890",
      },
    },
  ] as const)("$name", ({ fields, want }) => {
    expect(visitOutcomeBody(fields, links, REQUEST_ID)).toEqual(want);
  });

  it("is null when the record it names is missing", () => {
    const none = { paymentId: null, promiseVersionId: null };
    expect(
      visitOutcomeBody(
        { outcome: "paid", newPhone: "", note: "" },
        none,
        REQUEST_ID,
      ),
    ).toBeNull();
    expect(
      visitOutcomeBody(
        { outcome: "promise", newPhone: "", note: "" },
        none,
        REQUEST_ID,
      ),
    ).toBeNull();
  });
});

describe("visitErrorView", () => {
  it.each([
    apiError(0, NETWORK_ERROR_CODE, "Tidak dapat terhubung"),
    apiError(502, "unknown", GENERIC_ERROR_MESSAGE),
  ])("may have recorded it when no answer came (status $status)", (error) => {
    expect(visitErrorView(error)).toEqual({
      kind: "unknown",
      message: UNKNOWN_SAVE_TEXT,
    });
  });

  it.each([
    apiError(
      409,
      "visit_not_open",
      "Penyewa ini tidak punya kunjungan terbuka.",
    ),
    apiError(
      400,
      "visit_payment_invalid",
      "Tunai ini tidak bisa dipakai.",
      "paymentId",
    ),
    apiError(
      400,
      "visit_promise_invalid",
      "Janji ini tidak bisa dipakai.",
      "promiseVersionId",
    ),
  ])("reloads the list for $code", (error) => {
    expect(visitErrorView(error)).toEqual({
      kind: "stale",
      message: error.message,
    });
  });

  it.each([
    apiError(400, "invalid_phone", "Nomor WA tidak valid.", "newPhone"),
    apiError(400, "invalid_input", "Catatan terlalu panjang.", "note"),
  ])("puts $code under its field", (error) => {
    expect(visitErrorView(error)).toEqual({
      kind: "field",
      field: error.field,
      message: error.message,
    });
  });

  it("is a generic alert for request_id_reused", () => {
    expect(
      visitErrorView(
        apiError(409, "request_id_reused", "requestId dipakai.", "requestId"),
      ),
    ).toEqual({ kind: "alert", message: GENERIC_ERROR_MESSAGE });
  });

  it("shows the API's message for a denial", () => {
    expect(
      visitErrorView(apiError(403, "forbidden", "Anda tidak boleh mencatat.")),
    ).toEqual({ kind: "alert", message: "Anda tidak boleh mencatat." });
  });

  it("is generic for an error that is not the API's", () => {
    expect(visitErrorView(new TypeError("boom"))).toEqual({
      kind: "alert",
      message: GENERIC_ERROR_MESSAGE,
    });
  });
});

describe("openings", () => {
  type Item = { tenantId: string; name: string };
  const opening = (
    tenantId: string,
    requestId: string,
    unsure: boolean,
  ): TenantOpening<Item> => ({
    tenantId,
    requestId,
    unsure,
    item: { tenantId, name: tenantId },
  });

  it("starts a new opening, dropping the sure ones", () => {
    expect(
      nextOpenings(
        [opening("a", "r-1", false)],
        { tenantId: "b", name: "b" },
        "r-2",
      ),
    ).toEqual([opening("b", "r-2", false)]);
  });

  it("starts afresh for the same tenant once the last opening is sure", () => {
    expect(
      nextOpenings(
        [opening("a", "r-1", false)],
        { tenantId: "a", name: "a" },
        "r-2",
      ),
    ).toEqual([opening("a", "r-2", false)]);
  });

  it("brings back a tenant's unsure opening with its requestId", () => {
    expect(
      nextOpenings(
        [opening("a", "r-1", true)],
        { tenantId: "a", name: "a, newer" },
        "r-2",
      ),
    ).toEqual([
      {
        tenantId: "a",
        requestId: "r-1",
        unsure: true,
        item: { tenantId: "a", name: "a, newer" },
      },
    ]);
  });

  it("keeps another tenant's unsure opening", () => {
    expect(
      nextOpenings(
        [opening("a", "r-1", true)],
        { tenantId: "b", name: "b" },
        "r-2",
      ),
    ).toEqual([opening("a", "r-1", true), opening("b", "r-2", false)]);
  });

  it("marks an opening, and keeps the array when nothing changes", () => {
    const openings = [opening("a", "r-1", false)];
    expect(markOpening(openings, "a", false)).toBe(openings);
    expect(markOpening(openings, "z", true)).toBe(openings);
    expect(markOpening(openings, "a", true)).toEqual([
      opening("a", "r-1", true),
    ]);
  });
});
