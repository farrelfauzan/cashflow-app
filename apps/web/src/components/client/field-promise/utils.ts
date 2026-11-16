import { promiseDialogView } from "@/components/client/billing/utils";
import type { PromiseDialogView } from "@/components/client/billing/type";
import type { ChoiceOptionView } from "@/components/client/payment-page/type";
import type { FieldPromiseOffer } from "@/lib/api/generated/model";
import { PromiseKind } from "@/lib/api/generated/model";
import { formatDate } from "@/lib/format/date";
import { formatRupiah } from "@/lib/format/rupiah";

import type { FieldPromiseTenant } from "./type";

// "Catat janji bayar" (board HP-Tagihan, column 4): the karyawan records the tenant's payment
// promise from Tagihan hari ini. The fields, their checks and the request are those of the admin's
// "Catat janji" (billing's PromiseForm); only the copy, the order of the choices, and where the
// values come from differ: getFieldPromiseOffer, read when the sheet opens, plus what the row
// says the tenant owes (FieldTodayItem.outstanding, the tenant's whole debt, as on the account).

export const RECORD_FIELD_PROMISE_TEXT = "Catat janji bayar";
export const FIELD_PROMISE_INTRO =
  "Tanpa denda. Admin dan penyewa diberi tahu.";
export const FIELD_OWN_DATE_LABEL = "Pilih tanggal";
export const FIELD_OWN_DATE_SUB = "Tanggal yang disepakati dengan penyewa";
export const FIELD_NEXT_DUE_LABEL = "Bareng jatuh tempo berikutnya";
export const FIELD_PROMISE_DATE_LABEL = "Tanggal janji";
export const FIELD_PROMISE_AMOUNT_LABEL = "Jumlah janji (boleh kosong)";
export const FIELD_PROMISE_LOADING_LABEL = "Memuat janji…";
// The tenant owes nothing any more, or the active promise reached its last day (the offer has no
// dates): nothing can be recorded now.
export const NO_FIELD_PROMISE_TEXT = "Janji tidak bisa dicatat sekarang.";

// "Kosong = semua sisa, Rp1.050.000."
export function fieldPromiseAmountHint(outstanding: number): string {
  return `Kosong = semua sisa, ${formatRupiah(outstanding)}.`;
}

// "Pilih tanggal" first, then "Bareng jatuh tempo berikutnya" with its due date ("15 Des 2026")
// while the API offers it.
export function fieldPromiseKindOptions(
  nextInvoiceDueDate: string | null,
): ChoiceOptionView[] {
  const options: ChoiceOptionView[] = [
    {
      value: PromiseKind.specific_date,
      label: FIELD_OWN_DATE_LABEL,
      sub: FIELD_OWN_DATE_SUB,
    },
  ];
  if (nextInvoiceDueDate !== null) {
    options.push({
      value: PromiseKind.with_next_invoice,
      label: FIELD_NEXT_DUE_LABEL,
      sub: formatDate(nextInvoiceDueDate),
    });
  }
  return options;
}

// The sheet for the offer as it is when it arrives: empty on "Pilih tanggal" (the board's
// default), or filled from the active promise, which saving replaces (the same notice as the
// admin's). Null when no promise can be made now.
export function fieldPromiseDialogView(
  offer: FieldPromiseOffer,
  tenant: FieldPromiseTenant,
  subtitle: string,
): PromiseDialogView | null {
  const view = promiseDialogView(
    { ...offer, outstanding: tenant.outstanding },
    subtitle,
  );
  if (view === null) return null;
  return {
    ...view,
    title: RECORD_FIELD_PROMISE_TEXT,
    intro: FIELD_PROMISE_INTRO,
    kindOptions: fieldPromiseKindOptions(offer.nextInvoiceDueDate),
    dateLabel: FIELD_PROMISE_DATE_LABEL,
    amountLabel: FIELD_PROMISE_AMOUNT_LABEL,
    amountHint: fieldPromiseAmountHint(tenant.outstanding),
    showNote: false,
    defaults:
      offer.activePromise === null
        ? { ...view.defaults, kind: PromiseKind.specific_date }
        : view.defaults,
  };
}
