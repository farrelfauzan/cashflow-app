import { describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api/errors";
import type { Capability, Lease, Tenant } from "@/lib/api/generated/model";

import type { TenantFilters } from "./type";
import {
  billedNote,
  extraLeasesText,
  filterTenants,
  flaggedCount,
  isFiltering,
  isFlagged,
  isNotFound,
  leaseStatusView,
  leaseView,
  matchesSearch,
  NO_FILTERS,
  previousDay,
  propertyText,
  propertyOptions,
  tenantPermissions,
  tenantRow,
  tenantsSubtitle,
  tenantStatusView,
  unitText,
  toStatusFilter,
} from "./utils";

const TODAY = "2026-10-28";

const lease = (over: Partial<Lease> = {}): Lease => ({
  id: "11111111-1111-4111-8111-111111111111",
  tenantId: "22222222-2222-4222-8222-222222222222",
  unitId: "33333333-3333-4333-8333-333333333333",
  unitLabel: "Kamar 1",
  propertyId: "44444444-4444-4444-8444-444444444444",
  propertyName: "Kos Melati",
  startDate: "2026-07-01",
  enteredOn: "2026-10-28",
  billedFrom: "2026-07-01",
  rentAmount: 850_000,
  depositAmount: 850_000,
  status: "active",
  charges: [],
  createdAt: "2026-10-28T03:00:00Z",
  updatedAt: "2026-10-28T03:00:00Z",
  ...over,
});

const tenant = (over: Partial<Tenant> = {}): Tenant => ({
  id: "55555555-5555-4555-8555-555555555555",
  name: "Rina Wulandari",
  phone: "+6281300010001",
  note: "",
  leases: [lease()],
  createdAt: "2026-10-28T03:00:00Z",
  updatedAt: "2026-10-28T03:00:00Z",
  ...over,
});

const MAWAR = {
  propertyId: "66666666-6666-4666-8666-666666666666",
  propertyName: "Kontrakan Mawar",
};
const ACTIVE_COLLECTOR = {
  memberId: "77777777-7777-4777-8777-777777777777",
  name: "Mas Dimas Saputra",
  active: true,
};
const DISABLED_COLLECTOR = {
  memberId: "88888888-8888-4888-8888-888888888888",
  name: "Mas Agus Setiawan",
  active: false,
};

describe("leaseStatusView", () => {
  it.each<{ name: string; lease: Lease; today: string; want: object }>([
    {
      name: "a running lease",
      lease: lease(),
      today: TODAY,
      want: { label: "Aktif", detail: null, tone: "active" },
    },
    {
      name: "a lease that starts today",
      lease: lease({ startDate: TODAY }),
      today: TODAY,
      want: { label: "Aktif", detail: null, tone: "active" },
    },
    {
      name: "a lease that starts tomorrow",
      lease: lease({ startDate: "2026-10-29" }),
      today: TODAY,
      want: { label: "Belum mulai", detail: "mulai 29 Okt", tone: "upcoming" },
    },
    {
      name: "a lease booked for next month",
      lease: lease({ startDate: "2026-12-05" }),
      today: TODAY,
      want: { label: "Belum mulai", detail: "mulai 5 Des", tone: "upcoming" },
    },
    {
      name: "an ended lease",
      lease: lease({ status: "ended", endDate: "2026-09-30" }),
      today: TODAY,
      want: { label: "Sudah keluar", detail: "keluar 30 Sep", tone: "ended" },
    },
    {
      name: "an ended lease without an end date",
      lease: lease({ status: "ended" }),
      today: TODAY,
      want: { label: "Sudah keluar", detail: null, tone: "ended" },
    },
  ])("gives $name its status", ({ lease: l, today, want }) => {
    expect(leaseStatusView(l, today)).toEqual(want);
  });
});

describe("tenantStatusView", () => {
  it("follows the first lease", () => {
    const t = tenant({
      leases: [
        lease({ status: "ended", endDate: "2026-09-30" }),
        lease({ id: "x" }),
      ],
    });
    expect(tenantStatusView(t, TODAY).tone).toBe("ended");
  });

  it("is 'Tanpa sewa' without leases", () => {
    expect(tenantStatusView(tenant({ leases: [] }), TODAY)).toEqual({
      label: "Tanpa sewa",
      detail: null,
      tone: "none",
    });
  });
});

describe("flagged tenants", () => {
  it.each<{ name: string; tenant: Tenant; want: boolean }>([
    {
      name: "an active lease with a disabled collector",
      tenant: tenant({ leases: [lease({ collector: DISABLED_COLLECTOR })] }),
      want: true,
    },
    {
      name: "an active lease with an active collector",
      tenant: tenant({ leases: [lease({ collector: ACTIVE_COLLECTOR })] }),
      want: false,
    },
    {
      name: "an active lease without a collector",
      tenant: tenant(),
      want: false,
    },
    {
      name: "an ended lease whose collector was disabled",
      tenant: tenant({
        leases: [lease({ status: "ended", collector: DISABLED_COLLECTOR })],
      }),
      want: false,
    },
    {
      name: "a second lease with a disabled collector",
      tenant: tenant({
        leases: [
          lease({ collector: ACTIVE_COLLECTOR }),
          lease({ id: "y", ...MAWAR, collector: DISABLED_COLLECTOR }),
        ],
      }),
      want: true,
    },
    { name: "no leases", tenant: tenant({ leases: [] }), want: false },
  ])("$name: $want", ({ tenant: t, want }) => {
    expect(isFlagged(t)).toBe(want);
  });

  it("counts them", () => {
    const flagged = tenant({
      leases: [lease({ collector: DISABLED_COLLECTOR })],
    });
    expect(flaggedCount([flagged, tenant(), flagged])).toBe(2);
    expect(flaggedCount([])).toBe(0);
  });
});

describe("extraLeasesText", () => {
  it.each([
    { count: 0, want: null },
    { count: 1, want: null },
    { count: 2, want: "+1\u00a0sewa\u00a0lain" },
    { count: 4, want: "+3\u00a0sewa\u00a0lain" },
  ])("gives $want for $count leases", ({ count, want }) => {
    expect(extraLeasesText(count)).toBe(want);
  });
});

describe("tenantRow", () => {
  it("shows the first lease, its collector and the other leases", () => {
    const row = tenantRow(
      tenant({
        leases: [
          lease({ collector: ACTIVE_COLLECTOR }),
          lease({ id: "y", unitLabel: "Rumah B", ...MAWAR }),
        ],
      }),
      TODAY,
    );
    expect(row).toMatchObject({
      name: "Rina Wulandari",
      phone: "0813-0001-0001",
      unit: "Kamar 1",
      property: "Kos Melati",
      extraLeases: "+1\u00a0sewa\u00a0lain",
      collector: "Mas Dimas Saputra",
      flagged: false,
    });
    expect(row.status.label).toBe("Aktif");
  });

  it("says a tenant without leases has none", () => {
    expect(tenantRow(tenant({ leases: [] }), TODAY)).toMatchObject({
      unit: "Belum punya sewa",
      property: null,
      extraLeases: null,
      collector: null,
      flagged: false,
    });
  });

  it("keeps the disabled collector's name and flags the row", () => {
    const row = tenantRow(
      tenant({ leases: [lease({ collector: DISABLED_COLLECTOR })] }),
      TODAY,
    );
    expect(row.collector).toBe("Mas Agus Setiawan");
    expect(row.flagged).toBe(true);
  });
});

describe("tenantsSubtitle", () => {
  it.each([
    {
      name: "Griya Sejahtera",
      want: "Semua penyewa Griya Sejahtera dan sewanya.",
    },
    { name: "", want: "Semua penyewa dan sewanya." },
  ])("gives '$want' for '$name'", ({ name, want }) => {
    expect(tenantsSubtitle(name)).toBe(want);
  });
});

describe("tenantPermissions", () => {
  it.each<{ name: string; capabilities: Capability[]; want: boolean }>([
    {
      name: "lease:manage allowed",
      capabilities: [{ action: "lease:manage", access: "allow" }],
      want: true,
    },
    {
      name: "lease:manage conditional",
      capabilities: [{ action: "lease:manage", access: "conditional" }],
      want: true,
    },
    {
      name: "lease:manage denied",
      capabilities: [{ action: "lease:manage", access: "deny" }],
      want: false,
    },
    {
      name: "lease:manage not listed",
      capabilities: [{ action: "tenant:read", access: "allow" }],
      want: false,
    },
  ])("$name", ({ capabilities, want }) => {
    expect(tenantPermissions(capabilities).canManageLeases).toBe(want);
  });
});

describe("matchesSearch", () => {
  const t = tenant({
    name: "Eko Prasetyo",
    phone: "+6281200010003",
    leases: [
      lease({ unitLabel: "Rumah A" }),
      lease({ id: "y", unitLabel: "Kamar 12" }),
    ],
  });

  it.each([
    { name: "an empty query", search: "", want: true },
    { name: "only spaces", search: "   ", want: true },
    { name: "part of the name", search: "prase", want: true },
    { name: "the name in other letters", search: "EKO", want: true },
    { name: "a unit label", search: "rumah a", want: true },
    { name: "the label of another lease", search: "kamar 12", want: true },
    { name: "a number with the leading 0", search: "0812 0001", want: true },
    { name: "a number with dashes", search: "0812-0001-0003", want: true },
    { name: "a number with +62", search: "+62 812 0001", want: true },
    {
      name: "a number with 62 and no plus",
      search: "6281200010003",
      want: true,
    },
    { name: "the middle of a number", search: "0001 0003", want: true },
    { name: "a number that is not theirs", search: "0812 9999", want: false },
    { name: "too few digits", search: "08", want: false },
    { name: "a word that is not there", search: "tegalrejo", want: false },
    { name: "digits inside a longer text", search: "kamar 1203", want: false },
    { name: "a property name", search: "melati", want: false },
  ])("$name: $want", ({ search, want }) => {
    expect(matchesSearch(t, search)).toBe(want);
  });
});

describe("filterTenants", () => {
  const rina = tenant({
    id: "a",
    name: "Rina Wulandari",
    phone: "+6281300010001",
    leases: [
      lease({ collector: ACTIVE_COLLECTOR }),
      lease({ id: "r2", unitLabel: "Rumah B", ...MAWAR }),
    ],
  });
  const eko = tenant({
    id: "b",
    name: "Eko Prasetyo",
    phone: "+6281300010005",
    leases: [
      lease({
        id: "e1",
        unitLabel: "Rumah A",
        ...MAWAR,
        collector: DISABLED_COLLECTOR,
      }),
    ],
  });
  const dewi = tenant({
    id: "c",
    name: "Dewi Anggraini",
    phone: "+6281300010004",
    leases: [lease({ id: "d1", status: "ended", endDate: "2026-09-30" })],
  });
  const fajar = tenant({
    id: "d",
    name: "Fajar Nugroho",
    phone: "+6281300010007",
    leases: [lease({ id: "f1", startDate: "2026-11-01" })],
  });
  const anton = tenant({ id: "e", name: "Anton Wijaya", leases: [] });
  const all = [rina, eko, dewi, fajar, anton];

  const names = (filters: Partial<TenantFilters>) =>
    filterTenants(all, { ...NO_FILTERS, ...filters }, TODAY).map((t) => t.name);

  it("keeps everyone without filters", () => {
    expect(names({})).toHaveLength(5);
  });

  it.each<{ name: string; filters: Partial<TenantFilters>; want: string[] }>([
    {
      name: "a search",
      filters: { search: "eko" },
      want: ["Eko Prasetyo"],
    },
    {
      name: "a phone search",
      filters: { search: "0813 0001 0004" },
      want: ["Dewi Anggraini"],
    },
    {
      name: "a property: any lease there counts",
      filters: { propertyId: MAWAR.propertyId },
      want: ["Rina Wulandari", "Eko Prasetyo"],
    },
    {
      name: "the status Aktif",
      filters: { status: "active" },
      want: ["Rina Wulandari", "Eko Prasetyo"],
    },
    {
      name: "the status Belum mulai",
      filters: { status: "upcoming" },
      want: ["Fajar Nugroho"],
    },
    {
      name: "the status Sudah keluar",
      filters: { status: "ended" },
      want: ["Dewi Anggraini"],
    },
    {
      name: "the status Tanpa sewa",
      filters: { status: "none" },
      want: ["Anton Wijaya"],
    },
    {
      name: "flagged only",
      filters: { flaggedOnly: true },
      want: ["Eko Prasetyo"],
    },
    {
      name: "a search and a property together",
      filters: { search: "rina", propertyId: MAWAR.propertyId },
      want: ["Rina Wulandari"],
    },
    {
      name: "filters that exclude each other",
      filters: { search: "dewi", status: "active" },
      want: [],
    },
  ])("narrows by $name", ({ filters, want }) => {
    expect(names(filters)).toEqual(want);
  });
});

describe("isFiltering", () => {
  it.each<{ name: string; filters: TenantFilters; want: boolean }>([
    { name: "no filters", filters: NO_FILTERS, want: false },
    {
      name: "spaces in the search",
      filters: { ...NO_FILTERS, search: "  " },
      want: false,
    },
    { name: "a search", filters: { ...NO_FILTERS, search: "a" }, want: true },
    {
      name: "a property",
      filters: { ...NO_FILTERS, propertyId: "p" },
      want: true,
    },
    {
      name: "a status",
      filters: { ...NO_FILTERS, status: "ended" },
      want: true,
    },
    {
      name: "flagged only",
      filters: { ...NO_FILTERS, flaggedOnly: true },
      want: true,
    },
  ])("$name: $want", ({ filters, want }) => {
    expect(isFiltering(filters)).toBe(want);
  });
});

describe("propertyOptions", () => {
  it("lists each property of the leases once, by name", () => {
    const options = propertyOptions([
      tenant({ leases: [lease(), lease({ id: "y", ...MAWAR })] }),
      tenant({ leases: [lease({ id: "z" })] }),
      tenant({ leases: [] }),
    ]);
    expect(options).toEqual([
      { value: MAWAR.propertyId, label: "Kontrakan Mawar" },
      { value: "44444444-4444-4444-8444-444444444444", label: "Kos Melati" },
    ]);
  });

  it("is empty without leases", () => {
    expect(propertyOptions([tenant({ leases: [] })])).toEqual([]);
  });
});

describe("toStatusFilter", () => {
  it.each([
    { value: "active", want: "active" },
    { value: "upcoming", want: "upcoming" },
    { value: "ended", want: "ended" },
    { value: "none", want: "none" },
    { value: "all", want: "all" },
    { value: "bogus", want: "all" },
  ])("turns '$value' into $want", ({ value, want }) => {
    expect(toStatusFilter(value)).toBe(want);
  });
});

describe("previousDay", () => {
  it.each([
    { date: "2026-10-15", want: "2026-10-14" },
    { date: "2026-11-01", want: "2026-10-31" },
    { date: "2027-01-01", want: "2026-12-31" },
    { date: "2028-03-01", want: "2028-02-29" },
    { date: "2026-03-01", want: "2026-02-28" },
  ])("is $want before $date", ({ date, want }) => {
    expect(previousDay(date)).toBe(want);
  });
});

describe("billedNote", () => {
  it("is empty when billing starts with the lease", () => {
    expect(billedNote(lease())).toBeNull();
  });

  it("names the days the owner collected by hand", () => {
    expect(
      billedNote(lease({ startDate: "2026-09-15", billedFrom: "2026-10-15" })),
    ).toBe("15 Sep sampai 14 Okt dianggap sudah ditagih sendiri oleh pemilik");
  });

  it("crosses a month end", () => {
    expect(
      billedNote(lease({ startDate: "2026-09-30", billedFrom: "2026-11-01" })),
    ).toBe("30 Sep sampai 31 Okt dianggap sudah ditagih sendiri oleh pemilik");
  });
});

describe("leaseView", () => {
  it("formats a running lease", () => {
    const view = leaseView(
      lease({
        rentAmount: 1_500_000,
        depositAmount: 3_000_000,
        startDate: "2026-09-15",
        billedFrom: "2026-10-15",
        enteredOn: "2026-10-28",
        collector: DISABLED_COLLECTOR,
        charges: [
          { chargeTypeId: "c1", name: "Wi-Fi", amount: 50_000 },
          { chargeTypeId: "c2", name: "Kebersihan", amount: 25_000 },
        ],
      }),
      TODAY,
    );
    expect(view).toMatchObject({
      title: "Kamar 1 · Kos Melati",
      rent: "Rp1.500.000",
      deposit: "Rp3.000.000",
      collector: { name: "Mas Agus Setiawan", inactive: true },
      startDate: "15 Sep 2026",
      billedFrom: "15 Okt 2026",
      plannedEnd: { text: "Belum diatur", set: false },
      endDate: null,
      entered: "Dimasukkan ke aplikasi 28 Okt 2026.",
    });
    expect(view.status.label).toBe("Aktif");
    expect(view.charges).toEqual([
      { id: "c1", label: "Wi-Fi Rp50.000" },
      { id: "c2", label: "Kebersihan Rp25.000" },
    ]);
  });

  it("shows the planned end", () => {
    expect(
      leaseView(lease({ plannedEndDate: "2026-12-31" }), TODAY).plannedEnd,
    ).toEqual({ text: "31 Des 2026", set: true });
  });

  it("shows the end date of an ended lease instead of a planned one", () => {
    const view = leaseView(
      lease({ status: "ended", endDate: "2026-09-30" }),
      TODAY,
    );
    expect(view.endDate).toBe("30 Sep 2026");
    expect(view.plannedEnd).toBeNull();
    expect(view.status.label).toBe("Sudah keluar");
  });

  it("has no collector when the lease has none", () => {
    expect(leaseView(lease(), TODAY).collector).toBeNull();
  });
});

describe("unit and property text", () => {
  const withLeases = tenantRow(
    tenant({
      leases: [lease(), lease({ id: "y", unitLabel: "Rumah B", ...MAWAR })],
    }),
    TODAY,
  );
  const single = tenantRow(tenant(), TODAY);
  const none = tenantRow(tenant({ leases: [] }), TODAY);

  it.each([
    { name: "one lease", row: single, want: "Kamar 1 · Kos Melati" },
    {
      name: "more leases",
      row: withLeases,
      want: "Kamar 1 · Kos Melati · +1\u00a0sewa\u00a0lain",
    },
    { name: "no lease", row: none, want: "Belum punya sewa" },
  ])("reads the unit for $name", ({ row, want }) => {
    expect(unitText(row)).toBe(want);
  });

  it.each([
    { name: "one lease", row: single, want: "Kos Melati" },
    {
      name: "more leases",
      row: withLeases,
      want: "Kos Melati · +1\u00a0sewa\u00a0lain",
    },
    { name: "no lease", row: none, want: "" },
  ])("reads the property for $name", ({ row, want }) => {
    expect(propertyText(row)).toBe(want);
  });
});

describe("isNotFound", () => {
  const apiError = (status: number) =>
    new ApiError({ status, code: "not_found", message: "Tidak ditemukan." });

  it.each([
    { name: "a 404", error: apiError(404), want: true },
    {
      name: "a 403, a tenant the member may not see",
      error: apiError(403),
      want: true,
    },
    { name: "a 401", error: apiError(401), want: false },
    { name: "a 500", error: apiError(500), want: false },
    { name: "a plain error", error: new Error("x"), want: false },
    { name: "nothing", error: null, want: false },
  ])("$name: $want", ({ error, want }) => {
    expect(isNotFound(error)).toBe(want);
  });
});
