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
  // The to-do banner about tenants whose collector was disabled ("Tampilkan").
  canManageLeases: boolean;
};

// Tenant filters: what the list is narrowed to, in the browser.
export type StatusFilter = "all" | StatusTone;

export type TenantFilters = {
  search: string;
  // A property id, or "all".
  propertyId: string;
  status: StatusFilter;
  // Only tenants flagged "Belum punya penagih" (set by the banner's "Tampilkan").
  flaggedOnly: boolean;
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

// One lease card on the tenant's page, every value ready to show.
export type LeaseView = {
  id: string;
  title: string;
  status: StatusView;
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
