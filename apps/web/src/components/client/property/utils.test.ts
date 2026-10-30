import { describe, expect, it } from "vitest";

import type { Capability, Property, Unit } from "@/lib/api/generated/model";
import { ApiError, NETWORK_ERROR_CODE } from "@/lib/api/errors";
import { readSort } from "@/components/client/list/utils";
import { createAppAbility } from "@/lib/capabilities";

import type { PropertyPermissions } from "./type";
import {
  checkPropertyForm,
  checkUnitForm,
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

describe("checkPropertyForm", () => {
  it("trims the name and the address and sends the kind", () => {
    expect(
      checkPropertyForm({
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
    expect(
      checkPropertyForm({ name: "Ruko", kind: "ruko", address: " " }),
    ).toEqual({
      ok: true,
      input: { name: "Ruko", kind: "ruko", address: "" },
    });
  });

  it.each([
    {
      name: "nothing filled in",
      values: { name: "  ", kind: null, address: "" },
      errors: { name: EMPTY_PROPERTY_NAME_MESSAGE, kind: NO_KIND_MESSAGE },
    },
    {
      name: "no name",
      values: { name: "", kind: "kos" as const, address: "" },
      errors: { name: EMPTY_PROPERTY_NAME_MESSAGE },
    },
    {
      name: "no kind",
      values: { name: "Kos Melati", kind: null, address: "" },
      errors: { kind: NO_KIND_MESSAGE },
    },
  ])("refuses $name", ({ values, errors }) => {
    expect(checkPropertyForm(values)).toEqual({ ok: false, errors });
  });
});

describe("checkUnitForm", () => {
  it.each([
    {
      name: "a trimmed label, sent alone",
      values: { label: " Kamar 11 " },
      input: { label: "Kamar 11" },
    },
  ])("accepts $name", ({ values, input }) => {
    expect(checkUnitForm(values)).toEqual({ ok: true, input });
  });

  it.each([
    {
      name: "no label",
      values: { label: " " },
      errors: { label: EMPTY_UNIT_LABEL_MESSAGE },
    },
  ])("refuses $name", ({ values, errors }) => {
    expect(checkUnitForm(values)).toEqual({ ok: false, errors });
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
