import { describe, expect, it } from "vitest";
import { z } from "zod";

import type {
  Capability,
  Property,
  Unit,
  UnitPrice,
} from "@/lib/api/generated/model";
import { ApiError, NETWORK_ERROR_CODE } from "@/lib/api/errors";
import { readSort } from "@/components/client/list/utils";
import { createAppAbility } from "@/lib/capabilities";

import type { PropertyPermissions, UnitTermRow } from "./type";
import {
  changePriceLabel,
  checkUnitForm,
  checkUnitPriceForm,
  currentPriceText,
  CURRENT_PRICE_CHIP,
  defaultUnitTerm,
  dropsOfferedTerm,
  EMPTY_PRICE_MESSAGE,
  FUTURE_PRICE_CHIP,
  initialTermRows,
  nextPriceText,
  NO_TERM_MESSAGE,
  PRICE_BEFORE_TODAY_MESSAGE,
  priceHistoryRows,
  priceSavedNotice,
  propertyFormSchema,
  priceTermOptions,
  termLabel,
  termPriceLabel,
  unitTermRowView,
  ZERO_PRICE_MESSAGE,
  TERM_PRICE_HINT,
  lastTermPrices,
  termPriceHint,
  EMPTY_PROPERTY_NAME_MESSAGE,
  EMPTY_UNIT_LABEL_MESSAGE,
  isPropertyMissing,
  NO_KIND_MESSAGE,
  NO_RENT_TEXT,
  occupancyText,
  propertiesReadOnly,
  propertyKindLabel,
  PROPERTY_KINDS,
  PROPERTY_SORT_OPTIONS,
  propertyPermissions,
  propertySummary,
  unitCountText,
  UNIT_SORT_OPTIONS,
  unitOccupancyView,
  unitRentText,
  unitsReadOnly,
} from "./utils";

const NO_PROPERTY_PERMISSIONS: PropertyPermissions = {
  canAddProperty: false,
  canEditProperty: false,
  canManageUnits: false,
  seesOnlyAssigned: false,
};

function capabilities(
  entries: Record<string, Capability["access"]>,
): Capability[] {
  return Object.entries(entries).map(([action, access]) => ({
    action,
    access,
  }));
}

describe("propertyPermissions", () => {
  it.each<{
    name: string;
    capabilities: Capability[];
    want: PropertyPermissions;
  }>([
    {
      name: "an Admin of the whole organization",
      capabilities: capabilities({
        "property:read": "allow",
        "property:manage": "allow",
        "unit:manage": "allow",
      }),
      want: {
        canAddProperty: true,
        canEditProperty: true,
        canManageUnits: true,
        seesOnlyAssigned: false,
      },
    },
    {
      name: "an Admin limited to some properties",
      capabilities: capabilities({
        "property:read": "conditional",
        "property:manage": "conditional",
        "unit:manage": "conditional",
      }),
      want: {
        canAddProperty: false,
        canEditProperty: true,
        canManageUnits: true,
        seesOnlyAssigned: true,
      },
    },
    {
      name: "a Supervisor, who only reads",
      capabilities: capabilities({
        "property:read": "allow",
        "property:manage": "deny",
        "unit:manage": "deny",
      }),
      want: {
        canAddProperty: false,
        canEditProperty: false,
        canManageUnits: false,
        seesOnlyAssigned: false,
      },
    },
    {
      name: "a custom role that manages units only",
      capabilities: capabilities({
        "property:read": "allow",
        "unit:manage": "allow",
      }),
      want: {
        canAddProperty: false,
        canEditProperty: false,
        canManageUnits: true,
        seesOnlyAssigned: false,
      },
    },
    {
      name: "a member who cannot read properties",
      capabilities: capabilities({ "property:read": "deny" }),
      want: NO_PROPERTY_PERMISSIONS,
    },
    {
      name: "actions the API does not list",
      capabilities: [],
      want: NO_PROPERTY_PERMISSIONS,
    },
  ])("reads $name", ({ capabilities: given, want }) => {
    expect(propertyPermissions(createAppAbility(given))).toEqual(want);
  });
});

describe("read-only screens", () => {
  const admin = propertyPermissions(
    createAppAbility(
      capabilities({
        "property:read": "allow",
        "property:manage": "allow",
        "unit:manage": "allow",
      }),
    ),
  );
  const unitsOnly = propertyPermissions(
    createAppAbility(
      capabilities({ "property:read": "allow", "unit:manage": "allow" }),
    ),
  );

  it("is read-only for the list when the member cannot edit a property", () => {
    expect(propertiesReadOnly(admin)).toBe(false);
    expect(propertiesReadOnly(unitsOnly)).toBe(true);
    expect(propertiesReadOnly(NO_PROPERTY_PERMISSIONS)).toBe(true);
  });

  it("is read-only for one property only when nothing there can change", () => {
    expect(unitsReadOnly(admin)).toBe(false);
    expect(unitsReadOnly(unitsOnly)).toBe(false);
    expect(unitsReadOnly(NO_PROPERTY_PERMISSIONS)).toBe(true);
  });
});

const kosMelati: Property = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Kos Melati",
  kind: "kos",
  address: "Jl. Melati No. 12, Sleman",
  unitCount: 10,
  occupiedUnitCount: 8,
  createdAt: "2026-10-01T03:00:00Z",
  updatedAt: "2026-10-01T03:00:00Z",
};

const unit: Unit = {
  id: "22222222-2222-4222-8222-222222222222",
  propertyId: kosMelati.id,
  label: "Kamar 01",
  terms: [{ months: 1, price: 1_200_000 }],
  occupied: true,
  currentTenant: {
    id: "33333333-3333-4333-8333-333333333333",
    name: "Pak Agus Setiawan",
  },
  createdAt: "2026-10-01T03:00:00Z",
  updatedAt: "2026-10-01T03:00:00Z",
};

describe("propertyKindLabel", () => {
  it("names every kind in Indonesian, in the form's order", () => {
    expect(PROPERTY_KINDS.map(propertyKindLabel)).toEqual([
      "Kos",
      "Kontrakan",
      "Ruko",
      "Lainnya",
    ]);
  });
});

describe("unitCountText and occupancyText", () => {
  it.each([
    {
      name: "some free",
      total: 10,
      occupied: 8,
      count: "10 unit",
      text: "8 terisi, 2 kosong",
    },
    {
      name: "all occupied",
      total: 4,
      occupied: 4,
      count: "4 unit",
      text: "4 terisi, 0 kosong",
    },
    {
      name: "all free",
      total: 3,
      occupied: 0,
      count: "3 unit",
      text: "0 terisi, 3 kosong",
    },
    {
      name: "no unit yet",
      total: 0,
      occupied: 0,
      count: "0 unit",
      text: "Belum ada unit",
    },
  ])("$name", ({ total, occupied, count, text }) => {
    const property = { unitCount: total, occupiedUnitCount: occupied };
    expect(unitCountText(property)).toBe(count);
    expect(occupancyText(property)).toBe(text);
  });
});

describe("propertySummary", () => {
  it("joins address, kind and unit count", () => {
    expect(propertySummary(kosMelati)).toBe(
      "Jl. Melati No. 12, Sleman · Kos · 10 unit",
    );
  });

  it("leaves out a missing address", () => {
    expect(
      propertySummary({ ...kosMelati, address: "  ", kind: "other" }),
    ).toBe("Lainnya · 10 unit");
  });
});

describe("unitRentText", () => {
  it.each<{ name: string; terms: Unit["terms"]; want: string }>([
    {
      name: "a monthly price",
      terms: [{ months: 1, price: 1_200_000 }],
      want: "Rp1.200.000",
    },
    {
      name: "a 3-month price",
      terms: [{ months: 3, price: 3_500_000 }],
      want: "Rp3.500.000 per 3 bulan",
    },
    {
      name: "a 6-month price",
      terms: [{ months: 6, price: 6_900_000 }],
      want: "Rp6.900.000 per 6 bulan",
    },
    {
      name: "a yearly price",
      terms: [{ months: 12, price: 15_000_000 }],
      want: "Rp15.000.000 per tahun",
    },
    {
      name: "the shortest of several priced terms",
      terms: [
        { months: 12, price: 13_000_000 },
        { months: 3, price: 3_500_000 },
        { months: 6, price: 6_900_000 },
      ],
      want: "Rp3.500.000 per 3 bulan",
    },
    {
      name: "the shortest term that has a price",
      terms: [{ months: 1 }, { months: 12, price: 15_000_000 }],
      want: "Rp15.000.000 per tahun",
    },
    {
      name: "a term with only a next price",
      terms: [
        {
          months: 1,
          nextPrice: { amount: 1_300_000, effectiveFrom: "2026-12-01" },
        },
      ],
      want: NO_RENT_TEXT,
    },
    { name: "no priced term", terms: [{ months: 1 }], want: NO_RENT_TEXT },
    { name: "no term", terms: [], want: NO_RENT_TEXT },
  ])("shows $name as $want", ({ terms, want }) => {
    expect(unitRentText({ terms })).toBe(want);
  });
});

describe("unitOccupancyView", () => {
  it.each([
    {
      name: "a tenant",
      given: unit,
      want: { kind: "tenant", name: "Pak Agus Setiawan" },
    },
    {
      name: "an occupied unit whose tenant the viewer may not read",
      given: { ...unit, currentTenant: undefined },
      want: { kind: "occupied" },
    },
    {
      name: "a free unit",
      given: { ...unit, occupied: false, currentTenant: undefined },
      want: { kind: "free" },
    },
  ])("reads $name", ({ given, want }) => {
    expect(unitOccupancyView(given)).toEqual(want);
  });
});

// What the property form shows for these values: the input it sends, or the first message under
// each field (as zodResolver puts them).
function checkProperty(values: unknown) {
  const result = propertyFormSchema.safeParse(values);
  if (result.success) return { ok: true, input: result.data };
  const { fieldErrors } = z.flattenError(result.error);
  return {
    ok: false,
    errors: Object.fromEntries(
      Object.entries(fieldErrors).map(([field, messages]) => [
        field,
        messages?.[0],
      ]),
    ),
  };
}

describe("propertyFormSchema", () => {
  it("trims the name and the address and sends the kind", () => {
    expect(
      checkProperty({
        name: "  Kos Melati ",
        kind: "kos",
        address: " Jl. Melati No. 12, Sleman  ",
      }),
    ).toEqual({
      ok: true,
      input: {
        name: "Kos Melati",
        kind: "kos",
        address: "Jl. Melati No. 12, Sleman",
      },
    });
  });

  it("sends an empty address, which clears it when editing", () => {
    expect(checkProperty({ name: "Ruko", kind: "ruko", address: " " })).toEqual(
      {
        ok: true,
        input: { name: "Ruko", kind: "ruko", address: "" },
      },
    );
  });

  it.each([
    {
      name: "nothing filled in",
      values: { name: "  ", kind: undefined, address: "" },
      errors: { name: EMPTY_PROPERTY_NAME_MESSAGE, kind: NO_KIND_MESSAGE },
    },
    {
      name: "no name",
      values: { name: "", kind: "kos" as const, address: "" },
      errors: { name: EMPTY_PROPERTY_NAME_MESSAGE },
    },
    {
      name: "no kind",
      values: { name: "Kos Melati", kind: undefined, address: "" },
      errors: { kind: NO_KIND_MESSAGE },
    },
  ])("refuses $name", ({ values, errors }) => {
    expect(checkProperty(values)).toEqual({
      ok: false,
      errors,
    });
  });
});

// The four rows of "Masa sewa dan harga" with the given ones checked and typed.
function rows(given: Partial<Record<1 | 3 | 6 | 12, string>>): UnitTermRow[] {
  return ([1, 3, 6, 12] as const).map((months) => ({
    months,
    checked: given[months] !== undefined,
    priceText: given[months] ?? "",
  }));
}

describe("checkUnitForm", () => {
  it("sends a new unit's checked terms, with a price only where one is typed", () => {
    expect(
      checkUnitForm(
        { label: " Kamar 11 ", terms: rows({ 1: "1.250.000", 12: "" }) },
        undefined,
      ),
    ).toEqual({
      ok: true,
      input: {
        label: "Kamar 11",
        terms: [{ months: 1, price: 1_250_000 }, { months: 12 }],
      },
    });
  });

  it("never sends a price typed for a term that is then unchecked", () => {
    const values = { label: "Kamar 11", terms: rows({ 1: "" }) };
    values.terms[3] = { months: 12, checked: false, priceText: "15.000.000" };
    expect(checkUnitForm(values, undefined)).toEqual({
      ok: true,
      input: { label: "Kamar 11", terms: [{ months: 1 }] },
    });
  });

  it("keeps a changed unit's priced terms untouched and sends prices typed for the others", () => {
    const saved: Pick<Unit, "terms"> = {
      terms: [{ months: 1, price: 1_200_000 }, { months: 3 }],
    };
    // The monthly term has a price in force: it shows no field, so nothing typed for it goes.
    const values = {
      label: "Kamar 01",
      terms: rows({ 1: "9.999", 3: "3.300.000", 12: "15.000.000" }),
    };
    expect(checkUnitForm(values, saved)).toEqual({
      ok: true,
      input: {
        label: "Kamar 01",
        terms: [
          { months: 1 },
          { months: 3, price: 3_300_000 },
          { months: 12, price: 15_000_000 },
        ],
      },
    });
  });

  it("sends an offered term without a price when its field stays empty", () => {
    expect(
      checkUnitForm(
        { label: "Kamar 01", terms: rows({ 3: "" }) },
        { terms: [{ months: 3 }] },
      ),
    ).toEqual({
      ok: true,
      input: { label: "Kamar 01", terms: [{ months: 3 }] },
    });
  });

  it.each([
    {
      name: "no label",
      values: { label: " ", terms: rows({ 1: "" }) },
      errors: { label: EMPTY_UNIT_LABEL_MESSAGE },
    },
    {
      name: "no term",
      values: { label: "Kamar 01", terms: rows({}) },
      errors: { terms: NO_TERM_MESSAGE },
    },
    {
      name: "neither",
      values: { label: "", terms: rows({}) },
      errors: { label: EMPTY_UNIT_LABEL_MESSAGE, terms: NO_TERM_MESSAGE },
    },
  ])("refuses $name", ({ values, errors }) => {
    expect(checkUnitForm(values, undefined)).toEqual({ ok: false, errors });
  });

  it("keeps a custom term the form has no row for", () => {
    // Ruko 2 in the demo seed: renaming it must not drop its 24-month term.
    const saved: Pick<Unit, "terms"> = {
      terms: [
        { months: 12, price: 50_000_000 },
        { months: 24, price: 95_000_000 },
      ],
    };
    expect(
      checkUnitForm({ label: "Ruko 2A", terms: rows({ 12: "" }) }, saved),
    ).toEqual({
      ok: true,
      input: { label: "Ruko 2A", terms: [{ months: 12 }, { months: 24 }] },
    });
  });

  it("accepts no checked row while a custom term stays", () => {
    expect(
      checkUnitForm(
        { label: "Ruko 2", terms: rows({}) },
        { terms: [{ months: 24, price: 95_000_000 }] },
      ),
    ).toEqual({
      ok: true,
      input: { label: "Ruko 2", terms: [{ months: 24 }] },
    });
  });

  it("refuses a typed price of Rp0 under that term's field", () => {
    expect(
      checkUnitForm(
        { label: "Kamar 01", terms: rows({ 1: "1.250.000", 12: "0" }) },
        undefined,
      ),
    ).toEqual({ ok: false, errors: { prices: { 12: ZERO_PRICE_MESSAGE } } });
  });

  it("ignores what is typed for a term with a price in force, even Rp0", () => {
    expect(
      checkUnitForm(
        { label: "Kamar 01", terms: rows({ 1: "0" }) },
        { terms: [{ months: 1, price: 1_200_000 }] },
      ),
    ).toEqual({
      ok: true,
      input: { label: "Kamar 01", terms: [{ months: 1 }] },
    });
  });
});

describe("a term the unit no longer offers", () => {
  const history: UnitPrice[] = [
    // A later price for the monthly term, still to come.
    {
      id: "p4",
      months: 1,
      amount: 1_400_000,
      effectiveFrom: "2026-12-01",
      recordedAt: "2026-10-30T02:00:00Z",
    },
    {
      id: "p3",
      months: 12,
      amount: 15_000_000,
      effectiveFrom: "2026-09-01",
      recordedAt: "2026-09-01T02:00:00Z",
    },
    {
      id: "p2",
      months: 1,
      amount: 1_300_000,
      effectiveFrom: "2026-09-01",
      recordedAt: "2026-09-01T02:00:00Z",
    },
    {
      id: "p1",
      months: 12,
      amount: 14_000_000,
      effectiveFrom: "2026-01-01",
      recordedAt: "2026-01-01T02:00:00Z",
    },
  ];

  it("takes each term's latest price on or before today, as the API orders the history", () => {
    expect(lastTermPrices(history, "2026-10-30")).toEqual({
      1: 1_300_000,
      12: 15_000_000,
    });
    expect(lastTermPrices([], "2026-10-30")).toEqual({});
  });

  it("re-ticked, it says its last price applies again when the field stays empty", () => {
    // The unit offers only the monthly term now; the yearly one was dropped.
    const saved: Pick<Unit, "terms"> = {
      terms: [{ months: 1, price: 1_300_000 }],
    };
    const view = unitTermRowView(
      saved,
      12,
      lastTermPrices(history, "2026-10-30"),
    );
    expect(view).toEqual({
      kind: "input",
      nextPrice: undefined,
      lastPrice: 15_000_000,
    });
    if (view.kind !== "input") throw new Error("want a price field");
    expect(termPriceHint(view)).toBe(
      "Kalau dikosongkan, harga terakhir Rp15.000.000 berlaku lagi.",
    );
  });

  it("a term never priced keeps the usual hint", () => {
    const view = unitTermRowView(undefined, 3, {});
    if (view.kind !== "input") throw new Error("want a price field");
    expect(termPriceHint(view)).toBe(TERM_PRICE_HINT);
  });
});

describe("unit terms", () => {
  it.each([
    { kind: "kos" as const, want: 1 },
    { kind: "kontrakan" as const, want: 1 },
    { kind: "other" as const, want: 1 },
    { kind: "ruko" as const, want: 12 },
  ])("a new $kind unit starts with $want months", ({ kind, want }) => {
    expect(defaultUnitTerm(kind)).toBe(want);
    expect(
      initialTermRows(undefined, kind)
        .filter((row) => row.checked)
        .map((row) => row.months),
    ).toEqual([want]);
  });

  it("a changed unit starts with its offered terms, in the four rows' order", () => {
    expect(
      initialTermRows(
        { terms: [{ months: 12, price: 15_000_000 }, { months: 3 }] },
        "ruko",
      ),
    ).toEqual([
      { months: 1, checked: false, priceText: "" },
      { months: 3, checked: true, priceText: "" },
      { months: 6, checked: false, priceText: "" },
      { months: 12, checked: true, priceText: "" },
    ]);
  });

  it("shows a priced term read-only and any other with a field", () => {
    const next = { amount: 1_600_000, effectiveFrom: "2027-01-01" };
    const saved: Pick<Unit, "terms"> = {
      terms: [
        { months: 1, price: 1_500_000, nextPrice: next },
        { months: 3, nextPrice: next },
      ],
    };
    expect(unitTermRowView(saved, 1)).toEqual({
      kind: "priced",
      price: 1_500_000,
      nextPrice: next,
    });
    expect(unitTermRowView(saved, 3)).toEqual({
      kind: "input",
      nextPrice: next,
    });
    expect(unitTermRowView(saved, 12)).toEqual({
      kind: "input",
      nextPrice: undefined,
    });
    expect(unitTermRowView(undefined, 1)).toEqual({
      kind: "input",
      nextPrice: undefined,
    });
  });

  it("notices an offered term that is unchecked, only when changing a unit", () => {
    const saved: Pick<Unit, "terms"> = { terms: [{ months: 1 }] };
    expect(dropsOfferedTerm(saved, rows({ 12: "" }))).toBe(true);
    expect(dropsOfferedTerm(saved, rows({ 1: "", 12: "" }))).toBe(false);
    expect(dropsOfferedTerm(undefined, rows({}))).toBe(false);
  });

  it("names the terms and their prices", () => {
    expect([1, 3, 6, 12].map((m) => termLabel(m as 1 | 3 | 6 | 12))).toEqual([
      "Per bulan",
      "Per 3 bulan",
      "Per 6 bulan",
      "Per tahun",
    ]);
    expect(termPriceLabel(3)).toBe("Harga per 3 bulan");
    expect(termPriceLabel(12)).toBe("Harga per tahun");
    expect(changePriceLabel(12)).toBe("Ubah harga per tahun");
    expect(
      nextPriceText({ amount: 1_600_000, effectiveFrom: "2027-01-01" }),
    ).toBe("Mulai 1 Jan 2027: Rp1.600.000");
  });
});

describe("priceHistoryRows", () => {
  const price = (over: Partial<UnitPrice>): UnitPrice => ({
    id: "p",
    months: 12,
    amount: 15_000_000,
    effectiveFrom: "2026-10-30",
    recordedAt: "2026-10-30T03:00:00Z",
    recordedByName: "Pak Hendra Gunawan",
    ...over,
  });

  it("marks what is to come, what is in force per term, and nothing on older rows, in the API's order", () => {
    const history = [
      price({ id: "a", effectiveFrom: "2027-01-01", amount: 16_000_000 }),
      price({ id: "b", effectiveFrom: "2026-10-30" }),
      price({
        id: "c",
        months: 1,
        amount: 1_200_000,
        effectiveFrom: "2026-10-01",
        recordedAt: "2026-09-30T18:00:00Z",
        recordedByName: undefined,
      }),
      price({ id: "d", effectiveFrom: "2026-01-01", amount: 14_000_000 }),
    ];
    expect(priceHistoryRows(history, "2026-10-30")).toEqual([
      {
        id: "a",
        title: "Per tahun · Rp16.000.000",
        detail: "Mulai 1 Jan 2027 · dicatat Pak Hendra Gunawan, 30 Okt 2026",
        chip: FUTURE_PRICE_CHIP,
      },
      {
        id: "b",
        title: "Per tahun · Rp15.000.000",
        detail: "Mulai 30 Okt 2026 · dicatat Pak Hendra Gunawan, 30 Okt 2026",
        chip: CURRENT_PRICE_CHIP,
      },
      {
        id: "c",
        title: "Per bulan · Rp1.200.000",
        detail: "Mulai 1 Okt 2026 · dari data sebelumnya",
        chip: CURRENT_PRICE_CHIP,
      },
      {
        id: "d",
        title: "Per tahun · Rp14.000.000",
        detail: "Mulai 1 Jan 2026 · dicatat Pak Hendra Gunawan, 30 Okt 2026",
        chip: null,
      },
    ]);
  });

  it("shows the recorded day in Jakarta", () => {
    const [row] = priceHistoryRows(
      [price({ recordedAt: "2026-10-29T18:30:00Z" })],
      "2026-10-30",
    );
    expect(row?.detail).toBe(
      "Mulai 30 Okt 2026 · dicatat Pak Hendra Gunawan, 30 Okt 2026",
    );
  });
});

describe("Ubah harga", () => {
  const today = "2026-10-30";

  it("sends the term, the amount as an integer and the date", () => {
    expect(
      checkUnitPriceForm(
        { months: 12, amountText: "16.000.000", effectiveFrom: "2027-01-01" },
        today,
      ),
    ).toEqual({
      ok: true,
      input: { months: 12, amount: 16_000_000, effectiveFrom: "2027-01-01" },
    });
    expect(
      checkUnitPriceForm(
        { months: 1, amountText: "1.300.000", effectiveFrom: today },
        today,
      ).ok,
    ).toBe(true);
  });

  it.each([
    {
      name: "no amount",
      values: { months: 12 as const, amountText: "", effectiveFrom: today },
      errors: { amount: EMPTY_PRICE_MESSAGE },
    },
    {
      name: "a zero amount",
      values: { months: 12 as const, amountText: "0", effectiveFrom: today },
      errors: { amount: ZERO_PRICE_MESSAGE },
    },
    {
      name: "a date before today",
      values: {
        months: 12 as const,
        amountText: "16.000.000",
        effectiveFrom: "2026-10-29",
      },
      errors: { effectiveFrom: PRICE_BEFORE_TODAY_MESSAGE },
    },
    {
      name: "no date and no amount",
      values: { months: 12 as const, amountText: "", effectiveFrom: "" },
      errors: { amount: EMPTY_PRICE_MESSAGE, effectiveFrom: "Isi tanggalnya." },
    },
  ])("refuses $name", ({ values, errors }) => {
    expect(checkUnitPriceForm(values, today)).toEqual({ ok: false, errors });
  });

  it("offers the unit's terms, shortest first, with today's price under them", () => {
    const saved: Pick<Unit, "terms"> = {
      terms: [{ months: 12, price: 15_000_000 }, { months: 1 }],
    };
    expect(priceTermOptions(saved)).toEqual([
      { value: "1", label: "Per bulan" },
      { value: "12", label: "Per tahun" },
    ]);
    expect(currentPriceText(saved, 12)).toBe("Harga sekarang: Rp15.000.000");
    expect(currentPriceText(saved, 1)).toBe("Harga sekarang: belum ada");
  });

  it("says what was saved", () => {
    expect(priceSavedNotice({ months: 12, effectiveFrom: "2027-01-01" })).toBe(
      "Harga per tahun dicatat, berlaku mulai 1 Jan 2027.",
    );
    expect(priceSavedNotice({ months: 1, effectiveFrom: "2026-10-30" })).toBe(
      "Harga per bulan dicatat, berlaku mulai 30 Okt 2026.",
    );
  });
});

describe("isPropertyMissing", () => {
  const apiError = (status: number, code: string) =>
    new ApiError({ status, code, message: "x" });

  it.each<{ name: string; error: unknown; want: boolean }>([
    {
      name: "an unknown property",
      error: apiError(404, "not_found"),
      want: true,
    },
    {
      name: "a property outside the member's",
      error: apiError(403, "forbidden"),
      want: true,
    },
    { name: "no error", error: null, want: false },
    {
      name: "a server error",
      error: apiError(500, "internal"),
      want: false,
    },
    {
      name: "no connection",
      error: apiError(0, NETWORK_ERROR_CODE),
      want: false,
    },
    { name: "a plain error", error: new Error("boom"), want: false },
  ])("$name: $want", ({ error, want }) => {
    expect(isPropertyMissing(error)).toBe(want);
  });
});

describe("sort options", () => {
  it("offers the properties sorts, the API default first", () => {
    expect(PROPERTY_SORT_OPTIONS.map((o) => [o.value, o.label])).toEqual([
      ["name", "Nama A–Z"],
      ["-name", "Nama Z–A"],
    ]);
    expect(
      readSort(new URLSearchParams("sort=-name"), PROPERTY_SORT_OPTIONS),
    ).toBe("-name");
    expect(
      readSort(new URLSearchParams("sort=label"), PROPERTY_SORT_OPTIONS),
    ).toBe("name");
  });

  it("offers the units sorts, the API default first", () => {
    expect(UNIT_SORT_OPTIONS.map((o) => [o.value, o.label])).toEqual([
      ["label", "Nama unit A–Z"],
      ["-label", "Nama unit Z–A"],
    ]);
    expect(
      readSort(new URLSearchParams("sort=-label"), UNIT_SORT_OPTIONS),
    ).toBe("-label");
    expect(readSort(new URLSearchParams(""), UNIT_SORT_OPTIONS)).toBe("label");
  });
});
