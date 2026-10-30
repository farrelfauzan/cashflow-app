import type {
  Lease,
  ListTenantsStatus,
  Tenant,
  TenantInput,
} from "@/lib/api/generated/model";

// Tenant status: what a lease (or a tenant without one) is, from the lease's status and dates.
export type StatusTone = "active" | "upcoming" | "ended" | "none";

export type StatusView = {
  label: string;
  // A short note next to the label in the list: "mulai 5 Des", "keluar 30 Sep".
  detail: string | null;
  tone: StatusTone;
};

// What a chip looks like: one of the statuses, the warning chip ("Belum punya penagih"), or the
// plain one (extra charges).
export type ChipTone = StatusTone | "warning" | "neutral";

// Tenant controls: which controls show, from the signed-in member's capabilities.
export type TenantPermissions = {
  // "Tambah penyewa": the API accepts it only with tenant:manage for the whole organization, so a
  // member limited to some properties ("conditional") does not see it.
  canAddTenant: boolean;
  // "Ubah penyewa"; "conditional" shows it and lets the API decide per tenant.
  canEditTenant: boolean;
  // "Sewakan unit", and the to-do banner about tenants whose collector was disabled.
  canManageLeases: boolean;
  // "Impor dari spreadsheet": the API takes an import only with tenant:manage and lease:manage
  // for the whole organization, so a member limited to some properties does not see it.
  canImport: boolean;
  // "Tambah jenis biaya baru" in "Biaya tambahan": the API adds a charge type only with
  // lease:manage for the whole organization, since a type is used by every property.
  canAddChargeType: boolean;
};

// The changes to an active lease, each opened from its card: "Ubah sewa", "Biaya tambahan",
// "Rencana keluar", "Catat pindah keluar", and "Batalkan sewa" for a booking.
export type LeasePanelKind =
  "editLease" | "charges" | "plannedEnd" | "moveOut" | "cancelBooking";

// The side panel of the tenant screens; one at a time. The tenant is the one the API returned
// (after adding) or the one on the page (editing, renting a unit). A lease panel carries the
// lease from the page and its tenant's name.
export type TenantPanelState =
  | { kind: "closed" }
  | { kind: "add" }
  | { kind: "added"; tenant: Tenant }
  | { kind: "edit"; tenant: Tenant }
  | { kind: "lease"; tenant: Tenant }
  | { kind: LeasePanelKind; lease: Lease; tenantName: string };

export type OpenTenantPanel = Exclude<TenantPanelState, { kind: "closed" }>;

export type TenantFormValues = {
  name: string;
  phone: string;
  note: string;
};

// "note" is never found by the form (any text is fine); it is where an API error on it is shown.
export type TenantFormErrors = Partial<
  Record<"name" | "phone" | "note", string>
>;

export type TenantFormResult =
  { ok: true; input: TenantInput } | { ok: false; errors: TenantFormErrors };

// Tenant filters: what the API narrows the list to, read from the URL. Each is undefined when it
// is not set, so the request and its query key leave it out.
export type TenantListFilters = {
  // The trimmed search.
  q: string | undefined;
  propertyId: string | undefined;
  status: ListTenantsStatus | undefined;
  // Only tenants flagged "Belum punya penagih" (set by the banner's "Tampilkan").
  collectorDisabled: true | undefined;
};

export type FilterOption = {
  value: string;
  label: string;
};

// One tenant in the list, from their first lease.
export type TenantRowView = {
  id: string;
  name: string;
  // Formatted for display, 0812-3456-7801.
  phone: string;
  unit: string;
  // null for a tenant without a lease.
  property: string | null;
  // "+1 sewa lain" when the tenant has more visible leases than the first.
  extraLeases: string | null;
  // The first lease's collector, null when it has none.
  collector: string | null;
  // Any active lease of the tenant has a collector who is disabled.
  flagged: boolean;
  status: StatusView;
};

// The buttons on an active lease's card for a member with lease:manage. "Ubah sewa", "Biaya
// tambahan" and "Rencana keluar" always show; the last one is "Catat pindah keluar", or
// "Batalkan sewa" for a booking.
export type LeaseActions = {
  end: "moveOut" | "cancelBooking";
};

// One lease card on the tenant's page, every value ready to show.
export type LeaseView = {
  id: string;
  title: string;
  status: StatusView;
  // "Sewa per bulan" or the lease's longer term, and its rent or "Belum ada harga".
  rentLabel: string;
  rent: string;
  deposit: string;
  collector: { name: string; inactive: boolean } | null;
  startDate: string;
  billedFrom: string;
  // Why "Mulai tagih dari" differs from the start date, null when it does not.
  billedNote: string | null;
  // "Rencana keluar"; null on an ended lease, where the move-out cleared it.
  plannedEnd: { text: string; set: boolean } | null;
  // The last day, only on an ended lease.
  endDate: string | null;
  charges: readonly { id: string; label: string }[];
  entered: string;
};
