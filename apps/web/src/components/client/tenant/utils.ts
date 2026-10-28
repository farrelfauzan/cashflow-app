import { ApiError } from "@/lib/api/errors";
import type { Capability, Lease, Tenant } from "@/lib/api/generated/model";
import { formatDate, formatDayMonth } from "@/lib/format/date";
import { formatPhone } from "@/lib/format/phone";
import { formatRupiah } from "@/lib/format/rupiah";
import { canSee } from "@/lib/organization/org-menu";

import type {
  ChipTone,
  FilterOption,
  LeaseView,
  StatusFilter,
  StatusTone,
  StatusView,
  TenantFilters,
  TenantPermissions,
  TenantRowView,
} from "./type";

export const LOADING_TEXT = "Memuat…";

export const SEARCH_LABEL = "Cari nama, nomor, atau unit";
export const NO_LEASE_UNIT = "Belum punya sewa";
export const NO_COLLECTOR = "Tidak ada";
export const NO_PLANNED_END = "Belum diatur";
export const FLAGGED_CHIP = "Belum punya penagih";

export const ALL_PROPERTIES = "all";
export const ALL_PROPERTIES_LABEL = "Semua properti";
export const ALL_STATUSES_LABEL = "Semua status";

export const NO_FILTERS: TenantFilters = {
  search: "",
  propertyId: ALL_PROPERTIES,
  status: "all",
  flaggedOnly: false,
};

export const NO_PERMISSIONS: TenantPermissions = { canManageLeases: false };

export const STATUS_FILTER_OPTIONS: readonly FilterOption[] = [
  { value: "active", label: "Aktif" },
  { value: "upcoming", label: "Belum mulai" },
  { value: "ended", label: "Sudah keluar" },
  { value: "none", label: "Tanpa sewa" },
];

// A chip's surface by tone, from the theme tokens. The board's blue "Belum mulai" has no token:
// it is outlined in the accent instead.
export const CHIP_TONE_CLASS: Record<ChipTone, string> = {
  active: "bg-accent text-primary",
  upcoming: "border border-primary bg-card text-primary",
  ended: "bg-secondary text-muted-foreground",
  none: "bg-secondary text-muted-foreground",
  warning: "bg-warning/15 text-foreground",
  neutral: "bg-secondary text-foreground",
};

// A lease may be saved before it starts; the API calls it active from then on. A calendar date
// (YYYY-MM-DD) compares correctly as a string, and `today` is the Jakarta day.
export function leaseStatusView(lease: Lease, today: string): StatusView {
  if (lease.status === "ended") {
    return {
      label: "Sudah keluar",
      detail:
        lease.endDate === undefined
          ? null
          : `keluar ${formatDayMonth(lease.endDate)}`,
      tone: "ended",
    };
  }
  if (lease.startDate > today) {
    return {
      label: "Belum mulai",
      detail: `mulai ${formatDayMonth(lease.startDate)}`,
      tone: "upcoming",
    };
  }
  return { label: "Aktif", detail: null, tone: "active" };
}

// The tenant's status is their first lease's (the API sends active leases first).
export function tenantStatusView(tenant: Tenant, today: string): StatusView {
  const first = tenant.leases[0];
  if (first === undefined) {
    return { label: "Tanpa sewa", detail: null, tone: "none" };
  }
  return leaseStatusView(first, today);
}

// Flagged: an active lease whose collector was disabled and not replaced yet. An ended lease
// has nothing left to collect.
export function isFlagged(tenant: Tenant): boolean {
  return tenant.leases.some(
    (lease) =>
      lease.status === "active" &&
      lease.collector !== undefined &&
      !lease.collector.active,
  );
}

export function flaggedCount(tenants: readonly Tenant[]): number {
  return tenants.filter(isFlagged).length;
}

export function flagBannerTitle(count: number): string {
  return `${count} penyewa belum punya penagih`;
}

// Non-breaking spaces, so a narrow column never splits it.
export function extraLeasesText(leaseCount: number): string | null {
  return leaseCount > 1 ? `+${leaseCount - 1}\u00a0sewa\u00a0lain` : null;
}

export function tenantRow(tenant: Tenant, today: string): TenantRowView {
  const first = tenant.leases[0];
  return {
    id: tenant.id,
    name: tenant.name,
    phone: formatPhone(tenant.phone),
    unit: first === undefined ? NO_LEASE_UNIT : first.unitLabel,
    property: first === undefined ? null : first.propertyName,
    extraLeases: extraLeasesText(tenant.leases.length),
    collector: first?.collector?.name ?? null,
    flagged: isFlagged(tenant),
    status: tenantStatusView(tenant, today),
  };
}

// Where the tenant's unit is, as the phone card reads it: "Kamar 1 · Kos Melati · +1 sewa lain".
export function unitText(row: TenantRowView): string {
  return [row.unit, row.property, row.extraLeases]
    .filter((part) => part !== null)
    .join(" · ");
}

// The same without the unit, for the second line under it in the iPad table.
export function propertyText(row: TenantRowView): string {
  return [row.property, row.extraLeases]
    .filter((part) => part !== null)
    .join(" · ");
}

// A tenant the page cannot show: one that does not exist (404), or one that exists but none of
// whose leases the member may see (403, for a member limited to properties or to the tenants they
// collect). Both read "Penyewa tidak ditemukan".
export function isNotFound(error: unknown): boolean {
  return (
    error instanceof ApiError && (error.status === 404 || error.status === 403)
  );
}

export function tenantsSubtitle(organizationName: string): string {
  return organizationName === ""
    ? "Semua penyewa dan sewanya."
    : `Semua penyewa ${organizationName} dan sewanya.`;
}

// Tenant controls: which controls show, from the signed-in member's capabilities (getOrgMe),
// never from role names. Hiding is visibility only; the API checks every request.
export function tenantPermissions(
  capabilities: readonly Capability[],
): TenantPermissions {
  return { canManageLeases: canSee(capabilities, "lease:manage") };
}

// Search: a query made only of digits and phone punctuation is a phone number, compared as
// national digits so "0812 3456", "0812-3456" and "+62 812 3456" all find 0812-3456-xxxx. Fewer
// digits than this are too common to say anything about a number.
const PHONE_QUERY = /^[\d\s()+-]+$/;
export const PHONE_MIN_DIGITS = 3;

function nationalDigits(input: string): string {
  const digits = input.replace(/\D/g, "");
  return digits.startsWith("62") ? `0${digits.slice(2)}` : digits;
}

export function matchesSearch(tenant: Tenant, search: string): boolean {
  const query = search.trim().toLowerCase();
  if (query === "") return true;
  if (tenant.name.toLowerCase().includes(query)) return true;
  if (
    tenant.leases.some((lease) => lease.unitLabel.toLowerCase().includes(query))
  ) {
    return true;
  }
  if (!PHONE_QUERY.test(query)) return false;
  const digits = nationalDigits(query);
  return (
    digits.length >= PHONE_MIN_DIGITS &&
    nationalDigits(tenant.phone).includes(digits)
  );
}

// The property filter keeps a tenant with a lease in that property, whichever lease is first;
// the status filter follows the status the row shows (the first lease's).
export function filterTenants(
  tenants: readonly Tenant[],
  filters: TenantFilters,
  today: string,
): Tenant[] {
  return tenants.filter((tenant) => {
    if (!matchesSearch(tenant, filters.search)) return false;
    if (
      filters.propertyId !== ALL_PROPERTIES &&
      !tenant.leases.some((lease) => lease.propertyId === filters.propertyId)
    ) {
      return false;
    }
    if (
      filters.status !== "all" &&
      tenantStatusView(tenant, today).tone !== filters.status
    ) {
      return false;
    }
    return !filters.flaggedOnly || isFlagged(tenant);
  });
}

export function isFiltering(filters: TenantFilters): boolean {
  return (
    filters.search.trim() !== "" ||
    filters.propertyId !== ALL_PROPERTIES ||
    filters.status !== "all" ||
    filters.flaggedOnly
  );
}

// The properties the tenants' leases are in, by name.
export function propertyOptions(tenants: readonly Tenant[]): FilterOption[] {
  const names = new Map<string, string>();
  for (const tenant of tenants) {
    for (const lease of tenant.leases) {
      names.set(lease.propertyId, lease.propertyName);
    }
  }
  return [...names]
    .map(([value, label]) => ({ value, label }))
    .sort((a, b) => a.label.localeCompare(b.label, "id"));
}

// The status filter's value from the select's string: an unknown value means no filter.
export function toStatusFilter(value: string): StatusFilter {
  const found = STATUS_FILTER_OPTIONS.find((option) => option.value === value);
  return found === undefined ? "all" : (found.value as StatusTone);
}

// The calendar day before a YYYY-MM-DD date. Plain UTC date arithmetic: no timezone can move it.
export function previousDay(date: string): string {
  const [year = 0, month = 1, day = 1] = date.split("-").map(Number);
  return new Date(Date.UTC(year, month - 1, day - 1))
    .toISOString()
    .slice(0, 10);
}

// "Mulai tagih dari" is later than the start date when the admin skipped the periods the owner
// collected by hand; the note says which days those are.
export function billedNote(lease: Lease): string | null {
  if (lease.billedFrom === lease.startDate) return null;
  const from = formatDayMonth(lease.startDate);
  const to = formatDayMonth(previousDay(lease.billedFrom));
  return `${from} sampai ${to} dianggap sudah ditagih sendiri oleh pemilik`;
}

export function leaseView(lease: Lease, today: string): LeaseView {
  const ended = lease.status === "ended";
  return {
    id: lease.id,
    title: `${lease.unitLabel} · ${lease.propertyName}`,
    status: leaseStatusView(lease, today),
    rent: formatRupiah(lease.rentAmount),
    deposit: formatRupiah(lease.depositAmount),
    collector:
      lease.collector === undefined
        ? null
        : { name: lease.collector.name, inactive: !lease.collector.active },
    startDate: formatDate(lease.startDate),
    billedFrom: formatDate(lease.billedFrom),
    billedNote: billedNote(lease),
    plannedEnd: ended
      ? null
      : lease.plannedEndDate === undefined
        ? { text: NO_PLANNED_END, set: false }
        : { text: formatDate(lease.plannedEndDate), set: true },
    endDate:
      ended && lease.endDate !== undefined ? formatDate(lease.endDate) : null,
    charges: lease.charges.map((charge) => ({
      id: charge.chargeTypeId,
      label: `${charge.name} ${formatRupiah(charge.amount)}`,
    })),
    entered: `Dimasukkan ke aplikasi ${formatDate(lease.enteredOn)}.`,
  };
}
