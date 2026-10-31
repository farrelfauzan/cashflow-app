import { z } from "zod";

import {
  leaseUnitText,
  termPeriodWords,
} from "@/components/client/lease/utils";
import {
  readChoice,
  readSearch,
  SEARCH_PARAM,
  searchQuery,
} from "@/components/client/list/utils";
import { ApiError } from "@/lib/api/errors";
import {
  ListTenantsStatus,
  type Lease,
  type Property,
  type Tenant,
  type TermMonths,
} from "@/lib/api/generated/model";
import { formatDate, formatDayMonth } from "@/lib/format/date";
import { formatPhone } from "@/lib/format/phone";
import { formatRupiah } from "@/lib/format/rupiah";
import { everywhere, type AppAbility } from "@/lib/capabilities";

import type {
  ChipTone,
  FilterOption,
  LeaseActions,
  LeasePanelKind,
  LeaseView,
  StatusView,
  TenantFormValues,
  TenantListFilters,
  TenantPermissions,
  TenantRowView,
} from "./type";

export const LOADING_TEXT = "Memuat…";

export const SEARCH_LABEL = "Cari nama, nomor, atau unit";
export const NO_LEASE_UNIT = "Belum punya sewa";
export const NO_COLLECTOR = "Tidak ada";
export const NO_PLANNED_END = "Belum diatur";
export const FLAGGED_CHIP = "Belum punya penagih";

// The select value that means no filter; it never goes into the URL.
export const ALL_PROPERTIES = "all";
export const ALL_STATUSES = "all";
export const ALL_PROPERTIES_LABEL = "Semua properti";
export const ALL_STATUSES_LABEL = "Semua status";

// The URL search params the tenant filters live in, next to the list's page, size and search.
export const PROPERTY_PARAM = "propertyId";
export const STATUS_PARAM = "status";
export const COLLECTOR_DISABLED_PARAM = "collectorDisabled";

// The statuses the API accepts; any other value in the URL counts as no status filter.
const TENANT_STATUSES = Object.values(ListTenantsStatus);

// Takes the search and every filter off in one change ("Hapus pencarian").
export const CLEAR_FILTERS: Readonly<Record<string, null>> = {
  [SEARCH_PARAM]: null,
  [PROPERTY_PARAM]: null,
  [STATUS_PARAM]: null,
  [COLLECTOR_DISABLED_PARAM]: null,
};

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

// A tenant's leases in the order the screens show them: running ones, then ones booked ahead,
// then ended ones, keeping the API's order (newest start first) within each. The API calls a
// lease booked ahead active too, so its order alone could put it before the one the tenant lives
// in now.
export function orderedLeases(
  leases: readonly Lease[],
  today: string,
): Lease[] {
  const rank = (lease: Lease) =>
    lease.status === "ended" ? 2 : lease.startDate > today ? 1 : 0;
  return [...leases].sort((a, b) => rank(a) - rank(b));
}

// The tenant's status is the status of the lease their row shows.
export function tenantStatusView(tenant: Tenant, today: string): StatusView {
  const first = orderedLeases(tenant.leases, today)[0];
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

export function flagBannerTitle(count: number): string {
  return `${count} penyewa belum punya penagih`;
}

// Non-breaking spaces, so a narrow column never splits it.
export function extraLeasesText(leaseCount: number): string | null {
  return leaseCount > 1 ? `+${leaseCount - 1}\u00a0sewa\u00a0lain` : null;
}

export function tenantRow(tenant: Tenant, today: string): TenantRowView {
  const first = orderedLeases(tenant.leases, today)[0];
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

// Tenant controls: which controls show, from the member's ability (getOrgMe's capabilities),
// never from role names. Hiding is visibility only; the API checks every request.
export function tenantPermissions(ability: AppAbility): TenantPermissions {
  return {
    canAddTenant: ability.can("manage", everywhere("tenant")),
    canEditTenant: ability.can("manage", "tenant"),
    canManageLeases: ability.can("manage", "lease"),
    canImport:
      ability.can("manage", everywhere("tenant")) &&
      ability.can("manage", everywhere("lease")),
    canAddChargeType: ability.can("manage", everywhere("lease")),
  };
}

// A tenant who may rent a unit now: one without an active lease (a booking counts as active).
// The API itself allows a second lease; the screen offers it only here (board Sewa-Baru.dc.html).
export function needsLease(tenant: Pick<Tenant, "leases">): boolean {
  return !tenant.leases.some((lease) => lease.status === "active");
}

// What the tenant form checks before it sends anything. The Go API validates everything again
// (the number's format included) and its message always wins; these only give a friendly
// message early.

// The API's limits (TenantInput).
export const TENANT_NAME_MAX_LENGTH = 100;
export const TENANT_NOTE_MAX_LENGTH = 500;

export const EMPTY_TENANT_NAME_MESSAGE = "Isi nama penyewa.";
export const EMPTY_TENANT_PHONE_MESSAGE = "Isi nomor WhatsApp.";

// The fields the API's errors can name; any other field goes to the alert. The note has no
// check of its own (any text is fine); it is only where an API error on it is shown.
export const TENANT_FORM_API_FIELDS = ["name", "phone", "note"] as const;

export const tenantFormSchema = z.object({
  name: z.string().trim().min(1, EMPTY_TENANT_NAME_MESSAGE),
  phone: z.string().trim().min(1, EMPTY_TENANT_PHONE_MESSAGE),
  note: z.string().trim(),
});

// The form's starting text: empty when adding, the tenant's own when changing them, with the
// number the way people read it.
export function tenantFormValues(
  tenant: Pick<Tenant, "name" | "phone" | "note"> | undefined,
): TenantFormValues {
  if (tenant === undefined) return { name: "", phone: "", note: "" };
  return {
    name: tenant.name,
    phone: formatPhone(tenant.phone),
    note: tenant.note,
  };
}

// The short notices announced after a panel closes.
export function tenantAddedNotice(name: string): string {
  return `${name} sudah ditambahkan.`;
}

export function tenantSavedNotice(name: string): string {
  return `${name} sudah disimpan.`;
}

export function leaseSavedNotice(
  lease: Pick<Lease, "unitLabel" | "propertyName">,
  tenantName: string,
): string {
  return `${lease.unitLabel} · ${lease.propertyName} sudah disewakan kepada ${tenantName}.`;
}

// "Penyewa ditambahkan" (board Penyewa-Tambah.dc.html).
export function tenantAddedText(
  name: string,
  canManageLeases: boolean,
): string {
  return canManageLeases
    ? `${name} belum punya sewa. Sewakan unit sekarang supaya tagihannya bisa dibuat.`
    : `${name} belum punya sewa.`;
}

export const TENANT_ADDED_LATER_TEXT =
  "Kalau belum siap, penyewa tetap tersimpan dan bisa disewakan unit nanti dari daftar penyewa.";

// "Belum ada sewa" on the tenant's page (board Sewa-Baru.dc.html). Without lease:manage the
// sentence about renting a unit is left out.
export function noLeaseText(name: string, canManageLeases: boolean): string {
  return canManageLeases
    ? `${name} belum menyewa unit apa pun. Sewakan satu unit supaya tagihan bisa dibuat.`
    : `${name} belum menyewa unit apa pun.`;
}

// A property id as the API parses it; anything else (a link cut short in a chat) is not one.
const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

// The tenant filters in the URL. A status outside the API's list, a property that is not an id
// or any collectorDisabled other than "true" counts as not set, so a hand-edited URL never makes
// the API answer 400.
export function readTenantFilters(params: URLSearchParams): TenantListFilters {
  const propertyId = params.get(PROPERTY_PARAM) ?? "";
  return {
    q: searchQuery(readSearch(params)),
    propertyId: UUID_PATTERN.test(propertyId) ? propertyId : undefined,
    status: readChoice(params, STATUS_PARAM, TENANT_STATUSES),
    collectorDisabled:
      params.get(COLLECTOR_DISABLED_PARAM) === "true" ? true : undefined,
  };
}

// Whether the search or a filter narrows the list: then an empty answer is "Tidak ada penyewa
// yang cocok", not "Belum ada penyewa".
export function isFiltering(filters: TenantListFilters): boolean {
  return (
    filters.q !== undefined ||
    filters.propertyId !== undefined ||
    filters.status !== undefined ||
    filters.collectorDisabled !== undefined
  );
}

// A select's value as a URL param: "Semua ..." removes the param.
export function filterParam(value: string, allValue: string): string | null {
  return value === allValue ? null : value;
}

// The property filter's choices, in the API's order (by name).
export function propertyFilterOptions(
  properties: readonly Property[],
): FilterOption[] {
  return properties.map((property) => ({
    value: property.id,
    label: property.name,
  }));
}

// The calendar day before a YYYY-MM-DD date. Plain UTC date arithmetic: no timezone can move it.
export function previousDay(date: string): string {
  const [year = 0, month = 1, day = 1] = date.split("-").map(Number);
  return new Date(Date.UTC(year, month - 1, day - 1))
    .toISOString()
    .slice(0, 10);
}

// "Mulai tagih dari" is later than the start date when the admin skipped the periods the owner
// collected by hand; the note says which days those are. A longer term's skipped periods span a
// year or more, so they carry the year ("15 Sep 2025 sampai 14 Sep 2026").
export function billedNote(lease: Lease): string | null {
  if (lease.billedFrom === lease.startDate) return null;
  const format = lease.termMonths === 1 ? formatDayMonth : formatDate;
  const from = format(lease.startDate);
  const to = format(previousDay(lease.billedFrom));
  return `${from} sampai ${to} dianggap sudah ditagih sendiri oleh pemilik`;
}

// The rent's label on a lease card, by the lease's term: "Sewa per bulan", "Sewa per 3 bulan",
// "Sewa per 6 bulan" or "Sewa per tahun".
export function leaseRentLabel(termMonths: TermMonths): string {
  return `Sewa per ${termPeriodWords(termMonths)}`;
}

export const NO_PERIOD_RENT_TEXT = "Belum ada harga";

// The rent for one period of the lease's term in force today, from the API, or "Belum ada harga"
// when the unit has no price for that term.
export function leaseRentText(lease: Pick<Lease, "periodRent">): string {
  return lease.periodRent === undefined
    ? NO_PERIOD_RENT_TEXT
    : formatRupiah(lease.periodRent);
}

export function leaseView(lease: Lease, today: string): LeaseView {
  const ended = lease.status === "ended";
  return {
    id: lease.id,
    title: leaseUnitText(lease),
    status: leaseStatusView(lease, today),
    rentLabel: leaseRentLabel(lease.termMonths),
    rent: leaseRentText(lease),
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

// The buttons on a lease's card: only on an active lease, and only for a member with lease:manage
// (canManageLeases, from the capabilities). A lease that starts after today is a booking, which is
// cancelled instead of moved out of.
export function leaseActions(
  lease: Pick<Lease, "status" | "startDate">,
  today: string,
  canManageLeases: boolean,
): LeaseActions | null {
  if (!canManageLeases || lease.status !== "active") return null;
  return { end: lease.startDate > today ? "cancelBooking" : "moveOut" };
}

// The buttons in an active lease card's footer, in the board's order: "Biaya tambahan", "Rencana
// keluar", then "Catat pindah keluar" or "Batalkan sewa". "Ubah sewa" sits in the card's header.
export function leaseFooterKinds(actions: LeaseActions): LeasePanelKind[] {
  return ["charges", "plannedEnd", actions.end];
}

// An outline button in a lease card's footer: full width and stacked on the phone, in a row on
// iPad, 48px tall.
export const LEASE_ACTION_CLASS =
  "h-12 w-full rounded-xl border-[1.5px] bg-card px-[18px] text-base font-semibold md:w-auto md:text-[17px]";

// The lease panels' titles, also the labels of the card's buttons that open them.
export const LEASE_PANEL_TITLES: Record<LeasePanelKind, string> = {
  editLease: "Ubah sewa",
  charges: "Biaya tambahan",
  plannedEnd: "Rencana keluar",
  moveOut: "Catat pindah keluar",
  cancelBooking: "Batalkan sewa",
};

// A lease panel's subtitle: "Bu Rina Wulandari · Kamar 07 · Kos Melati".
export function leasePanelSubtitle(
  tenantName: string,
  lease: Pick<Lease, "unitLabel" | "propertyName">,
): string {
  return `${tenantName} · ${leaseUnitText(lease)}`;
}

export function leaseUpdatedNotice(lease: Pick<Lease, "unitLabel">): string {
  return `Sewa ${lease.unitLabel} diperbarui.`;
}

// After "Simpan rencana keluar" or "Hapus rencana keluar", from the saved lease.
export function plannedEndNotice(
  lease: Pick<Lease, "unitLabel" | "plannedEndDate">,
): string {
  return lease.plannedEndDate === undefined
    ? `Rencana keluar ${lease.unitLabel} dihapus.`
    : `Rencana keluar ${lease.unitLabel}: ${formatDate(lease.plannedEndDate)}.`;
}

export function leaseChargesNotice(lease: Pick<Lease, "unitLabel">): string {
  return `Biaya tambahan ${lease.unitLabel} disimpan.`;
}

export function movedOutNotice(lease: Pick<Lease, "unitLabel">): string {
  return `Pindah keluar dicatat. ${lease.unitLabel} sekarang kosong.`;
}

export function bookingCancelledNotice(
  lease: Pick<Lease, "unitLabel">,
): string {
  return `Sewa ${lease.unitLabel} dibatalkan. Unitnya kosong lagi.`;
}
