import type { ChipTone } from "@/components/client/tenant/type";
import type { ListInvoicesStatus } from "@/lib/api/generated/model";

// What the API narrows the month's invoices to, read from the URL. Each is undefined when it is
// not set, so the request and its query key leave it out.
export type InvoiceListFilters = {
  // The month in the URL, YYYY-MM; undefined is this month in Jakarta, which the API answers by
  // default. Not sent while the status is "overdue" (debt is not tied to a month).
  period: string | undefined;
  // The trimmed search.
  q: string | undefined;
  propertyId: string | undefined;
  status: ListInvoicesStatus | undefined;
};

// One chip of an invoice's status.
export type InvoiceChip = {
  label: string;
  tone: ChipTone;
};

// One invoice in the list, every value ready to show.
export type InvoiceRowView = {
  id: string;
  tenantId: string;
  tenantName: string;
  unit: string;
  property: string;
  // "15 Nov 2026".
  dueDate: string;
  billed: string;
  paid: string;
  outstanding: string;
  // The status chip first, then "Lewat rencana keluar" when the lease is past its planned end.
  chips: readonly InvoiceChip[];
};

// One lease in "Meteran belum dicatat", every value ready to show.
export type MissingMeterRowView = {
  // The key of the row.
  leaseId: string;
  tenantId: string;
  tenantName: string;
  unit: string;
  property: string;
  // "Listrik, Air": the types with no amount yet.
  missing: string;
  // "terbit 8 Nov" for the lease's next invoice, "—" when the API names none.
  nextInvoice: string;
};
