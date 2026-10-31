import { describe, expect, it } from "vitest";
import { z } from "zod";

import { ApiError } from "@/lib/api/errors";
import type {
  Capability,
  Lease,
  Property,
  Tenant,
} from "@/lib/api/generated/model";
import { createAppAbility } from "@/lib/capabilities";

import type { TenantListFilters } from "./type";
import {
  bookingCancelledNotice,
  leaseActions,
  leaseChargesNotice,
  leaseFooterKinds,
  LEASE_PANEL_TITLES,
  leasePanelSubtitle,
  leaseUpdatedNotice,
  movedOutNotice,
  plannedEndNotice,
  billedNote,
  CLEAR_FILTERS,
  EMPTY_TENANT_NAME_MESSAGE,
  EMPTY_TENANT_PHONE_MESSAGE,
  leaseSavedNotice,
  needsLease,
  noLeaseText,
  tenantAddedNotice,
  tenantAddedText,
  tenantFormSchema,
  tenantFormValues,
  tenantSavedNotice,
  extraLeasesText,
  filterParam,
  isFiltering,
  isFlagged,
  isNotFound,
  leaseStatusView,
  leaseRentLabel,
  leaseRentText,
  leaseView,
  NO_PERIOD_RENT_TEXT,
  orderedLeases,
  previousDay,
  propertyFilterOptions,
  propertyText,
  readTenantFilters,
  tenantPermissions,
  tenantRow,
  tenantsSubtitle,
  tenantStatusView,
  unitText,
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
  termMonths: 1,
  periodRent: 850_000,
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

describe("orderedLeases", () => {
  const running = lease({ id: "running" });
  const older = lease({ id: "older", startDate: "2026-01-05" });
  const ahead = lease({ id: "ahead", startDate: "2026-12-01", ...MAWAR });
  const ended = lease({ id: "ended", status: "ended", endDate: "2026-09-30" });

  it.each([
    {
      name: "a lease booked ahead after the running one, whatever the API order",
      leases: [ahead, running],
      want: ["running", "ahead"],
    },
    {
      name: "an ended lease last",
      leases: [ended, ahead, running],
      want: ["running", "ahead", "ended"],
    },
    {
      name: "the API's order within the running leases",
      leases: [running, older],
      want: ["running", "older"],
    },
    { name: "no leases", leases: [], want: [] },
  ])("$name", ({ leases, want }) => {
    expect(orderedLeases(leases, TODAY).map((l) => l.id)).toEqual(want);
  });

  it("does not change the API's array", () => {
    const leases = [ahead, running];
    orderedLeases(leases, TODAY);
    expect(leases.map((l) => l.id)).toEqual(["ahead", "running"]);
  });
});

describe("tenantStatusView", () => {
  it("follows the running lease before an ended one", () => {
    const t = tenant({
      leases: [
        lease({ status: "ended", endDate: "2026-09-30" }),
        lease({ id: "x" }),
      ],
    });
    expect(tenantStatusView(t, TODAY).tone).toBe("active");
  });

  it("follows the running lease before one booked ahead", () => {
    const t = tenant({
      leases: [lease({ id: "x", startDate: "2026-12-01" }), lease()],
    });
    expect(tenantStatusView(t, TODAY).tone).toBe("active");
  });

  it("is 'Belum mulai' when the only lease left is booked ahead", () => {
    const t = tenant({
      leases: [
        lease({ status: "ended", endDate: "2026-09-30" }),
        lease({ id: "x", startDate: "2026-12-01" }),
      ],
    });
    expect(tenantStatusView(t, TODAY).tone).toBe("upcoming");
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
  it("shows the lease the tenant lives in, not one booked ahead that the API sends first", () => {
    const row = tenantRow(
      tenant({
        leases: [
          lease({
            id: "y",
            unitLabel: "Rumah B",
            startDate: "2026-12-01",
            ...MAWAR,
          }),
          lease({ collector: ACTIVE_COLLECTOR }),
        ],
      }),
      TODAY,
    );
    expect(row).toMatchObject({
      unit: "Kamar 1",
      property: "Kos Melati",
      collector: "Mas Dimas Saputra",
      extraLeases: "+1\u00a0sewa\u00a0lain",
      status: { tone: "active" },
    });
  });

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
    expect(
      tenantPermissions(createAppAbility(capabilities)).canManageLeases,
    ).toBe(want);
  });
});

describe("tenantPermissions for the tenant and lease controls", () => {
  const permissions = (capabilities: Capability[]) =>
    tenantPermissions(createAppAbility(capabilities));

  it.each<{ name: string; capabilities: Capability[]; want: object }>([
    {
      name: "an admin of the whole organization sees every control",
      capabilities: [
        { action: "tenant:manage", access: "allow" },
        { action: "lease:manage", access: "allow" },
      ],
      want: {
        canAddTenant: true,
        canEditTenant: true,
        canManageLeases: true,
        canImport: true,
        canAddChargeType: true,
      },
    },
    {
      name: "an admin limited to a property cannot add a tenant, but edits and rents",
      capabilities: [
        { action: "tenant:manage", access: "conditional" },
        { action: "lease:manage", access: "conditional" },
      ],
      want: {
        canAddTenant: false,
        canEditTenant: true,
        canManageLeases: true,
        canImport: false,
        canAddChargeType: false,
      },
    },
    {
      name: "a read-only member sees none",
      capabilities: [
        { action: "tenant:read", access: "allow" },
        { action: "tenant:manage", access: "deny" },
        { action: "lease:manage", access: "deny" },
      ],
      want: {
        canAddTenant: false,
        canEditTenant: false,
        canManageLeases: false,
        canImport: false,
        canAddChargeType: false,
      },
    },
    {
      name: "tenant:manage without lease:manage adds but does not rent",
      capabilities: [{ action: "tenant:manage", access: "allow" }],
      want: {
        canAddTenant: true,
        canEditTenant: true,
        canManageLeases: false,
        canImport: false,
        canAddChargeType: false,
      },
    },
  ])("$name", ({ capabilities, want }) => {
    expect(permissions(capabilities)).toEqual(want);
  });
});

describe("tenantPermissions for the import", () => {
  it.each<{ name: string; capabilities: Capability[]; want: boolean }>([
    {
      name: "tenant:manage and lease:manage for the whole organization",
      capabilities: [
        { action: "tenant:manage", access: "allow" },
        { action: "lease:manage", access: "allow" },
      ],
      want: true,
    },
    {
      name: "tenant:manage limited to some properties",
      capabilities: [
        { action: "tenant:manage", access: "conditional" },
        { action: "lease:manage", access: "allow" },
      ],
      want: false,
    },
    {
      name: "lease:manage limited to some properties",
      capabilities: [
        { action: "tenant:manage", access: "allow" },
        { action: "lease:manage", access: "conditional" },
      ],
      want: false,
    },
    {
      name: "lease:manage denied",
      capabilities: [
        { action: "tenant:manage", access: "allow" },
        { action: "lease:manage", access: "deny" },
      ],
      want: false,
    },
    {
      name: "read only",
      capabilities: [
        { action: "tenant:read", access: "allow" },
        { action: "lease:read", access: "allow" },
      ],
      want: false,
    },
  ])("$name", ({ capabilities, want }) => {
    expect(tenantPermissions(createAppAbility(capabilities)).canImport).toBe(
      want,
    );
  });
});

describe("needsLease", () => {
  it.each<{ name: string; leases: Lease[]; want: boolean }>([
    { name: "no lease", leases: [], want: true },
    {
      name: "only an ended lease",
      leases: [lease({ status: "ended" })],
      want: true,
    },
    { name: "a running lease", leases: [lease()], want: false },
    {
      name: "a booking (the API calls it active)",
      leases: [lease({ startDate: "2026-12-01" })],
      want: false,
    },
    {
      name: "an ended lease and a running one",
      leases: [lease({ status: "ended" }), lease({ id: "x" })],
      want: false,
    },
  ])("$name: $want", ({ leases, want }) => {
    expect(needsLease({ leases })).toBe(want);
  });
});

// What the tenant form shows for these values: the input it sends, or the first message under
// each field (as zodResolver puts them).
function checkTenant(values: unknown) {
  const result = tenantFormSchema.safeParse(values);
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

describe("tenantFormSchema", () => {
  it("trims and sends name, number and note", () => {
    expect(
      checkTenant({
        name: "  Bu Dewi Lestari ",
        phone: " 0812-3456-7899 ",
        note: " bayar lewat anaknya ",
      }),
    ).toEqual({
      ok: true,
      input: {
        name: "Bu Dewi Lestari",
        phone: "0812-3456-7899",
        note: "bayar lewat anaknya",
      },
    });
  });

  it("sends an empty note when there is none", () => {
    expect(checkTenant({ name: "A", phone: "0812", note: "  " })).toEqual({
      ok: true,
      input: { name: "A", phone: "0812", note: "" },
    });
  });

  it.each([
    {
      name: "an empty name",
      values: { name: " ", phone: "0812", note: "" },
      want: { name: EMPTY_TENANT_NAME_MESSAGE },
    },
    {
      name: "an empty number",
      values: { name: "A", phone: "", note: "" },
      want: { phone: EMPTY_TENANT_PHONE_MESSAGE },
    },
    {
      name: "both",
      values: { name: "", phone: " ", note: "" },
      want: {
        name: EMPTY_TENANT_NAME_MESSAGE,
        phone: EMPTY_TENANT_PHONE_MESSAGE,
      },
    },
  ])("refuses $name", ({ values, want }) => {
    expect(checkTenant(values)).toEqual({ ok: false, errors: want });
  });
});

describe("tenantFormValues", () => {
  it("is empty when adding", () => {
    expect(tenantFormValues(undefined)).toEqual({
      name: "",
      phone: "",
      note: "",
    });
  });

  it("starts from the tenant, with the number as people read it", () => {
    expect(tenantFormValues(tenant({ note: "kerja di toko" }))).toEqual({
      name: "Rina Wulandari",
      phone: "0813-0001-0001",
      note: "kerja di toko",
    });
  });
});

describe("tenant copy", () => {
  it("announces what was saved", () => {
    expect(tenantAddedNotice("Bu Dewi")).toBe("Bu Dewi sudah ditambahkan.");
    expect(tenantSavedNotice("Bu Dewi")).toBe("Bu Dewi sudah disimpan.");
    expect(leaseSavedNotice(lease(), "Bu Dewi")).toBe(
      "Kamar 1 · Kos Melati sudah disewakan kepada Bu Dewi.",
    );
  });

  it.each([
    {
      canManageLeases: true,
      added:
        "Bu Dewi belum punya sewa. Sewakan unit sekarang supaya tagihannya bisa dibuat.",
      none: "Bu Dewi belum menyewa unit apa pun. Sewakan satu unit supaya tagihan bisa dibuat.",
    },
    {
      canManageLeases: false,
      added: "Bu Dewi belum punya sewa.",
      none: "Bu Dewi belum menyewa unit apa pun.",
    },
  ])(
    "speaks of renting a unit only with lease:manage ($canManageLeases)",
    ({ canManageLeases, added, none }) => {
      expect(tenantAddedText("Bu Dewi", canManageLeases)).toBe(added);
      expect(noLeaseText("Bu Dewi", canManageLeases)).toBe(none);
    },
  );
});

describe("readTenantFilters", () => {
  const read = (query: string) => readTenantFilters(new URLSearchParams(query));
  const none: TenantListFilters = {
    q: undefined,
    propertyId: undefined,
    status: undefined,
    collectorDisabled: undefined,
  };

  it("reads nothing from an empty URL", () => {
    expect(read("")).toEqual(none);
  });

  it("reads every filter", () => {
    expect(
      read(
        `q=%20rina%20&propertyId=${MAWAR.propertyId}&status=upcoming&collectorDisabled=true&page=2`,
      ),
    ).toEqual({
      q: "rina",
      propertyId: MAWAR.propertyId,
      status: "upcoming",
      collectorDisabled: true,
    });
  });

  it.each(["active", "upcoming", "ended", "none"])(
    "reads the status %s",
    (status) => {
      expect(read(`status=${status}`).status).toBe(status);
    },
  );

  it.each([
    { name: "a blank search", query: "q=%20%20" },
    { name: "an empty property", query: "propertyId=" },
    { name: "a property that is not an id", query: "propertyId=abc" },
    {
      name: "a property id cut short",
      query: "propertyId=66666666-6666-4666-8666-66666666",
    },
    { name: "an unknown status", query: "status=deleted" },
    { name: "a status in another case", query: "status=Active" },
    { name: "the status 'all'", query: "status=all" },
    { name: "collectorDisabled=false", query: "collectorDisabled=false" },
    { name: "collectorDisabled=1", query: "collectorDisabled=1" },
    { name: "an empty collectorDisabled", query: "collectorDisabled=" },
  ])("counts $name as not set", ({ query }) => {
    expect(read(query)).toEqual(none);
  });
});

describe("isFiltering", () => {
  const none: TenantListFilters = {
    q: undefined,
    propertyId: undefined,
    status: undefined,
    collectorDisabled: undefined,
  };

  it.each<{ name: string; filters: TenantListFilters; want: boolean }>([
    { name: "no filters", filters: none, want: false },
    { name: "a search", filters: { ...none, q: "a" }, want: true },
    { name: "a property", filters: { ...none, propertyId: "p" }, want: true },
    { name: "a status", filters: { ...none, status: "ended" }, want: true },
    {
      name: "flagged only",
      filters: { ...none, collectorDisabled: true },
      want: true,
    },
  ])("$name: $want", ({ filters, want }) => {
    expect(isFiltering(filters)).toBe(want);
  });
});

describe("CLEAR_FILTERS", () => {
  it("removes the search and every filter, and keeps the page size", () => {
    const params = new URLSearchParams(
      "q=rina&propertyId=p&status=ended&collectorDisabled=true&pageSize=50",
    );
    for (const [name, value] of Object.entries(CLEAR_FILTERS)) {
      expect(value).toBeNull();
      params.delete(name);
    }
    expect(params.toString()).toBe("pageSize=50");
  });
});

describe("filterParam", () => {
  it.each([
    { value: "all", want: null },
    { value: "active", want: "active" },
    { value: MAWAR.propertyId, want: MAWAR.propertyId },
  ])("turns '$value' into $want", ({ value, want }) => {
    expect(filterParam(value, "all")).toBe(want);
  });
});

describe("propertyFilterOptions", () => {
  const property = (id: string, name: string): Property => ({
    id,
    name,
    kind: "kos",
    address: "",
    unitCount: 0,
    occupiedUnitCount: 0,
    createdAt: "2026-10-28T03:00:00Z",
    updatedAt: "2026-10-28T03:00:00Z",
  });

  it("offers each property by id and name, in the API's order", () => {
    expect(
      propertyFilterOptions([
        property(MAWAR.propertyId, "Kontrakan Mawar"),
        property("44444444-4444-4444-8444-444444444444", "Kos Melati"),
      ]),
    ).toEqual([
      { value: MAWAR.propertyId, label: "Kontrakan Mawar" },
      { value: "44444444-4444-4444-8444-444444444444", label: "Kos Melati" },
    ]);
  });

  it("is empty without properties", () => {
    expect(propertyFilterOptions([])).toEqual([]);
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

  it("carries the year for a longer term", () => {
    expect(
      billedNote(
        lease({
          startDate: "2025-09-15",
          billedFrom: "2026-09-15",
          termMonths: 12,
        }),
      ),
    ).toBe(
      "15 Sep 2025 sampai 14 Sep 2026 dianggap sudah ditagih sendiri oleh pemilik",
    );
  });
});

describe("leaseRentLabel", () => {
  it.each([
    { months: 1, want: "Sewa per bulan" },
    { months: 3, want: "Sewa per 3 bulan" },
    { months: 6, want: "Sewa per 6 bulan" },
    { months: 12, want: "Sewa per tahun" },
  ] as const)("names a $months-month term $want", ({ months, want }) => {
    expect(leaseRentLabel(months)).toBe(want);
  });
});

describe("leaseRentText", () => {
  it("formats the period's rent", () => {
    expect(leaseRentText({ periodRent: 15_000_000 })).toBe("Rp15.000.000");
  });

  it("says so when the unit has no price for the term", () => {
    expect(leaseRentText({})).toBe(NO_PERIOD_RENT_TEXT);
  });
});

describe("leaseView", () => {
  it("formats a running lease", () => {
    const view = leaseView(
      lease({
        periodRent: 1_500_000,
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
      rentLabel: "Sewa per bulan",
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

describe("leaseActions", () => {
  it.each<{
    name: string;
    over: Partial<Lease>;
    canManage: boolean;
    want: object | null;
  }>([
    {
      name: "a running lease: move-out",
      over: {},
      canManage: true,
      want: { end: "moveOut" },
    },
    {
      name: "a lease that starts today: move-out",
      over: { startDate: TODAY },
      canManage: true,
      want: { end: "moveOut" },
    },
    {
      name: "a booking: cancel",
      over: { startDate: "2026-12-05" },
      canManage: true,
      want: { end: "cancelBooking" },
    },
    {
      name: "an ended lease: none",
      over: { status: "ended", endDate: "2026-10-01" },
      canManage: true,
      want: null,
    },
    {
      name: "without lease:manage: none",
      over: {},
      canManage: false,
      want: null,
    },
    {
      name: "a booking without lease:manage: none",
      over: { startDate: "2026-12-05" },
      canManage: false,
      want: null,
    },
  ])("$name", ({ over, canManage, want }) => {
    expect(leaseActions(lease(over), TODAY, canManage)).toEqual(want);
  });

  it("follows lease:manage from the capabilities", () => {
    const canManage = (access: Capability["access"]) =>
      tenantPermissions(createAppAbility([{ action: "lease:manage", access }]))
        .canManageLeases;
    expect(leaseActions(lease(), TODAY, canManage("conditional"))).toEqual({
      end: "moveOut",
    });
    expect(leaseActions(lease(), TODAY, canManage("deny"))).toBeNull();
  });
});

describe("canAddChargeType", () => {
  it.each<{ name: string; access: Capability["access"] | null; want: boolean }>(
    [
      {
        name: "lease:manage for the whole organization",
        access: "allow",
        want: true,
      },
      {
        name: "lease:manage on some properties",
        access: "conditional",
        want: false,
      },
      { name: "lease:manage denied", access: "deny", want: false },
      { name: "lease:manage not listed", access: null, want: false },
    ],
  )("$name", ({ access, want }) => {
    const capabilities: Capability[] =
      access === null ? [] : [{ action: "lease:manage", access }];
    expect(
      tenantPermissions(createAppAbility(capabilities)).canAddChargeType,
    ).toBe(want);
  });
});

describe("leaseFooterKinds", () => {
  it("puts Biaya tambahan first, then Rencana keluar and the end", () => {
    const labels = (end: "moveOut" | "cancelBooking") =>
      leaseFooterKinds({ end }).map((kind) => LEASE_PANEL_TITLES[kind]);
    expect(labels("moveOut")).toEqual([
      "Biaya tambahan",
      "Rencana keluar",
      "Catat pindah keluar",
    ]);
    expect(labels("cancelBooking")).toEqual([
      "Biaya tambahan",
      "Rencana keluar",
      "Batalkan sewa",
    ]);
  });
});

describe("lease notices", () => {
  const kamar = lease({ unitLabel: "Kamar 07", propertyName: "Kos Melati" });

  it("names the tenant, unit and property in a lease panel's subtitle", () => {
    expect(leasePanelSubtitle("Bu Rina Wulandari", kamar)).toBe(
      "Bu Rina Wulandari · Kamar 07 · Kos Melati",
    );
  });

  it.each([
    {
      name: "updated",
      got: leaseUpdatedNotice(kamar),
      want: "Sewa Kamar 07 diperbarui.",
    },
    {
      name: "planned end set",
      got: plannedEndNotice({ ...kamar, plannedEndDate: "2026-11-14" }),
      want: "Rencana keluar Kamar 07: 14 Nov 2026.",
    },
    {
      name: "planned end cleared",
      got: plannedEndNotice(kamar),
      want: "Rencana keluar Kamar 07 dihapus.",
    },
    {
      name: "charges saved",
      got: leaseChargesNotice(kamar),
      want: "Biaya tambahan Kamar 07 disimpan.",
    },
    {
      name: "moved out",
      got: movedOutNotice(kamar),
      want: "Pindah keluar dicatat. Kamar 07 sekarang kosong.",
    },
    {
      name: "booking cancelled",
      got: bookingCancelledNotice(kamar),
      want: "Sewa Kamar 07 dibatalkan. Unitnya kosong lagi.",
    },
  ])("$name", ({ got, want }) => {
    expect(got).toBe(want);
  });
});
