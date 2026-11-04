import { describe, expect, it } from "vitest";

import type {
  Capability,
  Lease,
  MeteredCharge,
} from "@/lib/api/generated/model";
import { getListInvoicesQueryKey } from "@/lib/api/generated/money/money";
import { createAppAbility } from "@/lib/capabilities";

import type { MeterFormValues } from "./type";
import {
  AMOUNT_TOO_LARGE_MESSAGE,
  AMOUNT_ZERO_MESSAGE,
  canRecordMeter,
  DIGITS_ONLY_MESSAGE,
  EMPTY_AMOUNT_MESSAGE,
  existingAmountText,
  findCharge,
  isMissingMetersQueryKey,
  lockedSelectionText,
  lockedText,
  lockedView,
  meterAvailable,
  meterDigits,
  meterFieldValues,
  meterFormDefaults,
  meterFormSchema,
  meterRequest,
  METER_END_BELOW_START_MESSAGE,
  monthLabel,
  nextInvoiceText,
  NO_NEXT_INVOICE_TEXT,
  previousMonth,
  recordedRows,
  savedText,
  usageMonthOptions,
} from "./utils";

const ORG = "0b9f7c1e-2a34-4c56-8d78-9e0f1a2b3c4d";
const LISTRIK = "11111111-1111-4111-8111-111111111111";
const AIR = "22222222-2222-4222-8222-222222222222";

function charge(overrides: Partial<MeteredCharge>): MeteredCharge {
  return {
    id: "mc-1",
    leaseId: "lease-1",
    chargeTypeId: LISTRIK,
    chargeTypeName: "Listrik",
    usageMonth: "2026-10",
    amount: 123_000,
    recordedBy: { memberId: "m-1", name: "Pak Budi" },
    createdAt: "2026-11-02T03:00:00Z",
    updatedAt: "2026-11-02T03:00:00Z",
    ...overrides,
  };
}

function values(overrides: Partial<MeterFormValues>): MeterFormValues {
  return {
    usageMonth: "2026-10",
    chargeTypeId: LISTRIK,
    meterStart: "",
    meterEnd: "",
    amount: "123.000",
    ...overrides,
  };
}

function messages(overrides: Partial<MeterFormValues>): string[] {
  const result = meterFormSchema.safeParse(values(overrides));
  return result.success
    ? []
    : result.error.issues.map((issue) => issue.message);
}

describe("month math", () => {
  it("steps back over the year boundary", () => {
    expect(previousMonth("2026-01")).toBe("2025-12");
    expect(previousMonth("2026-11")).toBe("2026-10");
    expect(monthLabel("2026-10")).toBe("Oktober 2026");
  });

  it("offers only this month to a lease that started this month", () => {
    expect(usageMonthOptions("2026-11-01", "2026-11-04")).toEqual([
      { value: "2026-11", label: "November 2026" },
    ]);
  });

  it("offers last month back to the start month, newest first", () => {
    expect(
      usageMonthOptions("2026-08-15", "2026-11-04").map((m) => m.value),
    ).toEqual(["2026-10", "2026-09", "2026-08"]);
  });

  it("offers at most 12 months to a lease that started 2 years ago", () => {
    const options = usageMonthOptions("2024-11-01", "2026-11-04");
    expect(options).toHaveLength(12);
    expect(options[0]?.value).toBe("2026-10");
    expect(options[11]?.value).toBe("2025-11");
  });

  it("crosses the year boundary in January", () => {
    expect(
      usageMonthOptions("2025-11-20", "2026-01-03").map((m) => m.label),
    ).toEqual(["Desember 2025", "November 2025"]);
  });

  it("offers nothing to a booking that starts next month", () => {
    expect(usageMonthOptions("2026-12-01", "2026-11-04")).toEqual([]);
  });
});

describe("meterAvailable and canRecordMeter", () => {
  const types = [{ chargeTypeId: LISTRIK, name: "Listrik" }];
  it.each<{
    name: string;
    lease: Pick<Lease, "status" | "startDate" | "meteredChargeTypes">;
    want: boolean;
  }>([
    {
      name: "an active lease with a metered type",
      lease: {
        status: "active",
        startDate: "2026-10-01",
        meteredChargeTypes: types,
      },
      want: true,
    },
    {
      name: "no metered type",
      lease: {
        status: "active",
        startDate: "2026-10-01",
        meteredChargeTypes: [],
      },
      want: false,
    },
    {
      name: "a booking",
      lease: {
        status: "active",
        startDate: "2026-11-20",
        meteredChargeTypes: types,
      },
      want: false,
    },
    {
      name: "an ended lease",
      lease: {
        status: "ended",
        startDate: "2026-10-01",
        meteredChargeTypes: types,
      },
      want: false,
    },
  ])("$name", ({ lease, want }) => {
    expect(meterAvailable(lease, "2026-11-04")).toBe(want);
  });

  it.each<{ access: Capability["access"] | null; want: boolean }>([
    { access: "allow", want: true },
    { access: "conditional", want: true },
    { access: "deny", want: false },
    { access: null, want: false },
  ])("charge:record $access", ({ access, want }) => {
    const capabilities: Capability[] =
      access === null ? [] : [{ action: "charge:record", access }];
    expect(canRecordMeter(createAppAbility(capabilities))).toBe(want);
  });
});

describe("meterFormSchema", () => {
  it.each<{ name: string; input: Partial<MeterFormValues>; want: string }>([
    {
      name: "an empty amount",
      input: { amount: "" },
      want: EMPTY_AMOUNT_MESSAGE,
    },
    { name: "zero", input: { amount: "0" }, want: AMOUNT_ZERO_MESSAGE },
    { name: "letters", input: { amount: "12rb" }, want: DIGITS_ONLY_MESSAGE },
    {
      name: "more than the maximum",
      input: { amount: "1.000.000.000.001" },
      want: AMOUNT_TOO_LARGE_MESSAGE,
    },
    {
      name: "a meter number with letters",
      input: { meterStart: "12a" },
      want: DIGITS_ONLY_MESSAGE,
    },
    {
      name: "an end below the start",
      input: { meterStart: "1250", meterEnd: "1200" },
      want: METER_END_BELOW_START_MESSAGE,
    },
  ])("refuses $name", ({ input, want }) => {
    expect(messages(input)).toEqual([want]);
  });

  it("puts the end below the start under Akhir", () => {
    const result = meterFormSchema.safeParse(
      values({ meterStart: "10", meterEnd: "9" }),
    );
    expect(result.error?.issues[0]?.path).toEqual(["meterEnd"]);
  });

  it("accepts integers, equal meter numbers and the maximum", () => {
    expect(
      meterFormSchema.parse(
        values({
          meterStart: "1250",
          meterEnd: "1250",
          amount: "1.000.000.000.000",
        }),
      ),
    ).toEqual({
      usageMonth: "2026-10",
      chargeTypeId: LISTRIK,
      meterStart: 1250,
      meterEnd: 1250,
      amount: 1_000_000_000_000,
    });
  });

  it("leaves out meter numbers that were not typed", () => {
    expect(meterFormSchema.parse(values({}))).toMatchObject({
      meterStart: undefined,
      meterEnd: undefined,
      amount: 123_000,
    });
  });
});

describe("meterRequest", () => {
  it("puts usageMonth (YYYY-MM) and chargeTypeId in the path, integers in the body", () => {
    const request = meterRequest({
      usageMonth: "2026-10",
      chargeTypeId: AIR,
      meterStart: 0,
      meterEnd: 14,
      amount: 45_000,
    });
    expect(request).toEqual({
      usageMonth: "2026-10",
      chargeTypeId: AIR,
      data: { amount: 45_000, meterStart: 0, meterEnd: 14 },
    });
    expect(request.usageMonth).toMatch(/^\d{4}-\d{2}$/);
  });

  it("sends no meter numbers when none were typed", () => {
    expect(
      meterRequest({
        usageMonth: "2026-10",
        chargeTypeId: LISTRIK,
        meterStart: undefined,
        meterEnd: undefined,
        amount: 1,
      }).data,
    ).toEqual({ amount: 1 });
  });
});

describe("form values", () => {
  it("starts on the newest month and the first metered type", () => {
    expect(
      meterFormDefaults(usageMonthOptions("2026-09-01", "2026-11-04"), {
        meteredChargeTypes: [
          { chargeTypeId: LISTRIK, name: "Listrik" },
          { chargeTypeId: AIR, name: "Air" },
        ],
      }),
    ).toEqual({
      usageMonth: "2026-10",
      chargeTypeId: LISTRIK,
      meterStart: "",
      meterEnd: "",
      amount: "",
    });
  });

  it("prefills a recorded amount, or empties the fields", () => {
    expect(meterFieldValues(charge({ meterStart: 0, meterEnd: 125 }))).toEqual({
      amount: "123.000",
      meterStart: "0",
      meterEnd: "125",
    });
    expect(meterFieldValues(undefined)).toEqual({
      amount: "",
      meterStart: "",
      meterEnd: "",
    });
  });

  it("keeps digits only in the meter numbers", () => {
    expect(meterDigits("1.250 kWh")).toBe("1250");
    expect(meterDigits("9".repeat(20))).toHaveLength(15);
  });

  it("finds the amount of the month and type", () => {
    const list = [charge({}), charge({ id: "mc-2", chargeTypeId: AIR })];
    expect(
      findCharge(list, { usageMonth: "2026-10", chargeTypeId: AIR })?.id,
    ).toBe("mc-2");
    expect(
      findCharge(list, { usageMonth: "2026-09", chargeTypeId: AIR }),
    ).toBeUndefined();
    expect(
      findCharge(undefined, { usageMonth: "2026-10", chargeTypeId: AIR }),
    ).toBeUndefined();
  });
});

describe("texts", () => {
  it("warns that saving replaces an amount already recorded", () => {
    expect(existingAmountText(charge({}))).toBe(
      "Listrik Oktober 2026 sudah dicatat Rp123.000 oleh Pak Budi. Menyimpan akan mengganti jumlahnya.",
    );
  });

  it("says what was saved and where it goes", () => {
    expect(
      savedText(charge({}), { period: "2026-11-01", issuedOn: "2026-11-08" }),
    ).toBe(
      "Listrik Oktober 2026 Rp123.000 tersimpan. Masuk Tagihan November 2026.",
    );
    expect(savedText(charge({}), undefined)).toBe(
      "Listrik Oktober 2026 Rp123.000 tersimpan.",
    );
  });

  it("names the next invoice, or says there is none", () => {
    expect(
      nextInvoiceText({ period: "2026-11-01", issuedOn: "2026-11-08" }),
    ).toBe(
      "Masuk ke tagihan berikutnya yang terbit setelah ini dicatat: Tagihan November 2026, terbit 8 Nov.",
    );
    expect(nextInvoiceText(undefined)).toBe(NO_NEXT_INVOICE_TEXT);
    expect(NO_NEXT_INVOICE_TEXT).toBe(
      "Belum ada tagihan berikutnya untuk sewa ini.",
    );
  });

  it("explains a locked amount", () => {
    expect(lockedText("2026-11-01")).toBe(
      "Jumlah ini sudah masuk Tagihan November 2026 dan tidak bisa diubah lagi. Kalau salah, buat koreksi pada tagihan itu dengan alasannya.",
    );
    expect(lockedSelectionText(charge({}))).toBe(
      "Listrik Oktober 2026 sudah ditagihkan dan terkunci.",
    );
  });
});

describe("recordedRows", () => {
  it("keeps the API's order and marks billed rows locked", () => {
    expect(
      recordedRows([
        charge({
          id: "a",
          chargeTypeId: AIR,
          chargeTypeName: "Air",
          amount: 45_000,
        }),
        charge({
          id: "b",
          usageMonth: "2026-09",
          billed: { invoiceId: "inv-1", kind: "regular", period: "2026-10-01" },
        }),
      ]),
    ).toEqual([
      {
        id: "a",
        selection: { usageMonth: "2026-10", chargeTypeId: AIR },
        text: "Air · Oktober 2026 · Rp45.000",
        name: "Air Oktober 2026",
        locked: false,
      },
      {
        id: "b",
        selection: { usageMonth: "2026-09", chargeTypeId: LISTRIK },
        text: "Listrik · September 2026 · Rp123.000",
        name: "Listrik September 2026",
        locked: true,
      },
    ]);
  });
});

describe("lockedView", () => {
  it("is null for an amount no invoice bills", () => {
    expect(lockedView(charge({}))).toBeNull();
  });

  it("shows the amount, the meter numbers, who recorded it and the invoice", () => {
    expect(
      lockedView(
        charge({
          meterStart: 1200,
          meterEnd: 1350,
          updatedAt: "2026-11-02T18:30:00Z",
          billed: { invoiceId: "inv-1", kind: "regular", period: "2026-11-01" },
        }),
      ),
    ).toEqual({
      heading: "Listrik · Oktober 2026",
      rows: [
        { label: "Jumlah", value: "Rp123.000" },
        { label: "Angka awal", value: "1200" },
        { label: "Angka akhir", value: "1350" },
        // 18.30 UTC is the next day in Jakarta.
        { label: "Dicatat", value: "Pak Budi · 3 Nov 2026" },
      ],
      text: lockedText("2026-11-01"),
      invoiceId: "inv-1",
      correctLabel: "Koreksi tagihan November 2026",
    });
  });

  it("names the final invoice when the move-out bills the amount", () => {
    expect(
      lockedView(
        charge({
          billed: { invoiceId: "inv-9", kind: "final", period: "2026-11-01" },
        }),
      )?.correctLabel,
    ).toBe("Koreksi tagihan akhir November 2026");
  });

  it("leaves out meter numbers that were not recorded", () => {
    expect(
      lockedView(
        charge({
          billed: { invoiceId: "inv-1", kind: "regular", period: "2026-11-01" },
        }),
      )?.rows.map((row) => row.label),
    ).toEqual(["Jumlah", "Dicatat"]);
  });
});

describe("isMissingMetersQueryKey", () => {
  it("matches the organization's missing readings, any filter", () => {
    expect(
      isMissingMetersQueryKey(
        ["org", ORG, `/orgs/${ORG}/metered-charges/missing`, {}],
        ORG,
      ),
    ).toBe(true);
  });

  it("leaves other queries and organizations alone", () => {
    expect(isMissingMetersQueryKey(getListInvoicesQueryKey(ORG), ORG)).toBe(
      false,
    );
    expect(
      isMissingMetersQueryKey(
        ["org", "other", `/orgs/other/metered-charges/missing`, {}],
        ORG,
      ),
    ).toBe(false);
    expect(
      isMissingMetersQueryKey(
        ["org", ORG, `/orgs/${ORG}/leases/x/metered-charges`, {}],
        ORG,
      ),
    ).toBe(false);
  });
});
