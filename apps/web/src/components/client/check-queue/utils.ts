import {
  Banknote,
  QrCode,
  Receipt,
  ShieldAlert,
  Smartphone,
  type LucideIcon,
} from "lucide-react";
import { z } from "zod";

import { invoiceName, invoiceTitle } from "@/components/client/billing/utils";
import {
  isForbidden,
  receiptCountText,
} from "@/components/client/deposit/utils";
import type { DepositChip } from "@/components/client/deposit/type";
import { readChoice } from "@/components/client/list/utils";
import {
  canRecordPayment,
  PAYMENT_AMOUNT_MAX,
  paymentFormSchema,
} from "@/components/client/payment/utils";
import type { ToastMessage } from "@/components/client/toast/type";
import { ApiError } from "@/lib/api/errors";
import {
  CheckQueueKind,
  type AccountInvoice,
  type CheckDepositGap,
  type CheckPhoneChanged,
  type CheckQrisAmountMismatch,
  type CheckQueueCounts,
  type CheckQueueItem,
  type CheckTenant,
  type CheckTransferProof,
  type CheckUploadInfected,
  type DismissCheckItemRequest,
  type MatchTransferProofRequest,
  type MatchTransferProofResult,
  type TenantPaymentPreview,
  type Upload,
  type UploadPurpose,
} from "@/lib/api/generated/model";
import { everywhere, type AppAbility } from "@/lib/capabilities";
import { formatTimestampShortWeekdayDayMonthTime } from "@/lib/format/date";
import { formatPhone } from "@/lib/format/phone";
import { formatRupiah, parseRupiahInput } from "@/lib/format/rupiah";

import type {
  CheckActionErrorKind,
  CheckFilterOption,
  CheckIconTone,
  CheckRowView,
  DepositGapSheetView,
  NoteFormOutput,
  NoteFormValues,
  PhoneChangeSheetView,
  ProofMatchOutput,
  ProofMatchValues,
  ProofPayerView,
  ProofPhotoCaption,
  ProofPhotoState,
  ProofPreviewView,
  QrisMismatchSheetView,
  UploadInfectedSheetView,
} from "./type";

// "Antrean cek": what the system could not settle on its own, oldest first. Every amount shown is
// a field the API sent, only formatted; the differences shown ("Kurang Rp50.000", "Beda
// Rp50.000") are display only. Closing an item records no money; matching a transfer proof
// records the amount the admin typed, and what it pays is the API's (its preview and answer).

export { isForbidden };

export const TITLE = "Antrean cek";
export const SUBTITLE =
  "Yang tidak bisa dicocokkan otomatis oleh sistem. Periksa satu per satu, yang terlama di atas.";
export const BILLING_LABEL = "Penagihan";
export const BREADCRUMB_LABEL = "Lokasi halaman";

export const LOADING_TEXT = "Memuat…";
export const FORBIDDEN_TEXT = "Anda tidak punya akses ke antrean cek.";
export const EMPTY_TEXT = "Tidak ada yang perlu dicek.";
export const LIST_LABEL = "Item yang perlu dicek";
export const FILTERS_LABEL = "Saring jenis";
export const ALL_LABEL = "Semua";

export const KIND_PARAM = "kind";
export const KINDS: readonly CheckQueueKind[] = Object.values(CheckQueueKind);

export const KIND_LABEL: Record<CheckQueueKind, string> = {
  transfer_proof: "Bukti transfer",
  deposit_gap: "Selisih setoran",
  upload_infected: "File ditolak",
  qris_amount_mismatch: "QRIS tidak cocok",
  phone_changed: "Nomor baru",
};

// An icon component per kind (rendered by the row), and its tile's tint.
export const KIND_ICON: Record<CheckQueueKind, LucideIcon> = {
  transfer_proof: Receipt,
  deposit_gap: Banknote,
  upload_infected: ShieldAlert,
  qris_amount_mismatch: QrCode,
  phone_changed: Smartphone,
};

export const KIND_ICON_TONE: Record<CheckQueueKind, CheckIconTone> = {
  transfer_proof: "accent",
  deposit_gap: "warning",
  upload_infected: "warning",
  qris_amount_mismatch: "warning",
  phone_changed: "accent",
};

export const PURPOSE_LABEL: Record<UploadPurpose, string> = {
  cash_receipt: "Foto kwitansi",
  transfer_proof: "Bukti transfer",
  cash_deposit: "Foto setoran",
};

// The drawers.
export const CLOSE_LABEL = "Tutup";
export const CANCEL_LABEL = "Batal";
export const SAVING_LABEL = "Menyimpan…";
export const TODO_TITLE = "Yang perlu Anda lakukan";

export const DEPOSIT_GAP_TITLE = "Selisih setoran";
export const DEPOSIT_GAP_CALLOUT =
  "Tagihan dan kwitansi penyewa tidak berubah: penyewa sudah membayar ke karyawan.";
export const DEPOSIT_LINK_TEXT = "Lihat setoran ini";
export const DEPOSIT_GAP_NOTE_LABEL = "Catatan penutupan (wajib)";
export const DEPOSIT_GAP_NOTE_PLACEHOLDER =
  "Misalnya: Mas Dimas menyerahkan Rp50.000 sisanya, Senin 16 Nov";
export const DEPOSIT_GAP_SUBMIT = "Tutup selisih";

export const UPLOAD_TITLE = "File ditolak";
export const BLOCKED_FILE_TEXT = "File tidak bisa dibuka";
export const UPLOAD_CALLOUT =
  "Pemindai menemukan isi berbahaya di file ini. Tidak ada uang atau tagihan yang berubah.";
export const NOTE_LABEL = "Catatan (wajib)";
export const UPLOAD_NOTE_PLACEHOLDER =
  "Misalnya: sudah minta Mas Dimas foto ulang";
export const UPLOAD_SUBMIT = "Tandai sudah ditangani";

export const QRIS_TITLE = "QRIS tidak cocok";
export const TRANSACTION_ID_LABEL = "ID transaksi AutoGoPay";
export const COPY_LABEL = "Salin";
export const QRIS_TODO =
  "Cek di dasbor AutoGoPay berapa yang benar-benar masuk. Jika kurang, buat koreksi di halaman penyewa dengan alasannya, lalu tandai di sini.";
export const QRIS_NOTE_PLACEHOLDER =
  "Misalnya: dicek di AutoGoPay, masuk Rp1.000.000, tidak perlu koreksi";
export const QRIS_SUBMIT = "Tandai sudah dicek";

export const PHONE_TITLE = "Nomor baru";
export const PHONE_REJECT_TITLE = "Jangan pakai nomor baru";
export const PHONE_TODO =
  "Pastikan nomor ini benar, misalnya kirim WA atau telepon dulu ke nomor baru.";
export const PHONE_CALLOUT =
  "Setelah dipakai, pengingat dan tautan bayar berikutnya dikirim ke nomor baru. Nomor lama tidak dipakai lagi.";
export const PHONE_APPLY = "Pakai nomor baru";
export const PHONE_REJECT = "Jangan pakai";
export const BACK_LABEL = "Kembali";
export const PHONE_BACK = BACK_LABEL;
export const PHONE_REASON_LABEL = "Kenapa tidak dipakai? (wajib)";
export const PHONE_REASON_PLACEHOLDER =
  "Misalnya: nomor tidak aktif saat ditelepon";
export const PHONE_REJECT_SUBMIT = "Simpan, jangan pakai";

export const PROOF_TITLE = "Bukti transfer";
export const PROOF_SENT_LABEL = "Dikirim penyewa";
export const PROOF_LINK_LABEL = "Dari tautan tagihan";
export const ZOOM_LABEL = "Perbesar foto";
export const ZOOM_TITLE = "Foto bukti transfer";
export const PROOF_PENDING_TEXT = "Foto sedang diperiksa, tunggu sebentar.";
export const PHOTO_LOADING_TEXT = "Memuat foto…";
export const PHOTO_ERROR_TEXT = "Foto tidak bisa dimuat.";
export const PROOF_CALLOUT =
  "Cek mutasi rekening usaha dulu. Simpan hanya jika uangnya sudah masuk.";
export const PAYER_LABEL = "Pembayaran dari";
export const PROOF_AMOUNT_LABEL = "Jumlah yang masuk";
export const PROOF_DATE_LABEL = "Tanggal masuk";
export const PROOF_FIELDS_HINT =
  "Isi sesuai mutasi rekening, bukan hanya dari foto. Kolom ini sengaja kosong saat dibuka.";
export const PREVIEW_LEAD = "Setelah disimpan:";
export const PREVIEW_LOADING_TEXT = "Menghitung…";
export const NO_RECEIPT_TEXT = "Tidak ada kwitansi WA untuk transfer.";
export const PROOF_NOTE_LABEL = "Catatan (opsional)";
export const PROOF_NOTE_PLACEHOLDER =
  "Misalnya: masuk ke BCA usaha, atas nama Wati H.";
export const PROOF_SUBMIT = "Simpan pembayaran";
export const PROOF_REJECT = "Bukan pembayaran";
export const PROOF_REJECT_TITLE = "Bukan pembayaran";
export const PROOF_REJECT_PLACEHOLDER =
  "Misalnya: foto struk belanja, bukan transfer sewa";
export const PROOF_REJECT_HINT =
  "Tidak ada pembayaran yang dicatat. Item ini hilang dari antrean.";
export const PROOF_REJECT_SUBMIT = "Simpan, bukan pembayaran";

// While the photo is being scanned the drawer asks again this often, so it turns usable without
// a reload. A clean photo's link is asked for again this long before it expires.
export const PROOF_POLL_MS = 5_000;
export const LINK_REFRESH_MARGIN_MS = 15_000;

// How long typing pauses before the API previews the amount.
export const PREVIEW_DEBOUNCE_MS = 400;

// The fields an API error can name on "Simpan pembayaran"; any other (requestId) goes to the
// alert.
export const PROOF_MATCH_API_FIELDS = ["amount", "receivedOn", "note"] as const;

// The API's limit (DismissCheckItemRequest.note).
export const NOTE_MAX_LENGTH = 500;
export const NOTE_EMPTY_MESSAGE = "Tulis catatannya dulu.";
export const REASON_EMPTY_MESSAGE = "Tulis alasannya dulu.";
export const NOTE_TOO_LONG_MESSAGE = `Catatan paling banyak ${NOTE_MAX_LENGTH} huruf.`;

// The field an API error can name on a note form; any other goes to the alert.
export const NOTE_API_FIELDS = ["note"] as const;

export const NOTE_FORM_DEFAULTS: NoteFormValues = { note: "" };

// The honorifics a short name keeps: "Mas Dimas Saputra" is asked as "Mas Dimas".
const HONORIFICS = new Set(["Pak", "Bu", "Ibu", "Mas", "Mbak", "Kak"]);

// Anyone who may check payments for the whole organization answers the items. Never by role
// name.
export function canCheckItems(ability: AppAbility): boolean {
  return ability.can("check", everywhere("payment"));
}

// A transfer proof becomes a payment only for a member who may also record payments ("conditional"
// shows it and the API decides per tenant). Never by role name.
export function canMatchProofs(ability: AppAbility): boolean {
  return canCheckItems(ability) && canRecordPayment(ability);
}

// The screen is for a member who may answer the items or read cash for the whole organization
// (the Supervisor, read-only); the API answers 403 to anyone else.
export function canReadCheckQueue(ability: AppAbility): boolean {
  return canCheckItems(ability) || ability.can("read", everywhere("cash"));
}

// Penagihan's link to the queue, with the number of items waiting.
export function checkQueueLinkLabel(waiting: number): string {
  return waiting > 0 ? `${TITLE}, ${waiting} menunggu` : TITLE;
}

// The kind in the URL, or undefined (every kind) for none or an unknown one, so a hand-edited
// URL never makes the API answer 400.
export function readCheckKind(
  params: URLSearchParams,
): CheckQueueKind | undefined {
  return readChoice(params, KIND_PARAM, KINDS);
}

// "Semua" (every kind, counted by `waiting`), then one choice per kind with its count.
export function filterOptions(
  counts: CheckQueueCounts | undefined,
  waiting: number | undefined,
): CheckFilterOption[] {
  return [
    { kind: null, label: ALL_LABEL, count: waiting },
    ...KINDS.map((kind) => ({
      kind,
      label: KIND_LABEL[kind],
      count: counts?.[kind],
    })),
  ];
}

// "Mas Dimas" from "Mas Dimas Saputra"; the full name when it has no honorific or is short.
export function shortName(name: string): string {
  const words = name.trim().split(/\s+/);
  if (words.length < 3 || !HONORIFICS.has(words[0] ?? "")) return name;
  return `${words[0]} ${words[1]}`;
}

// The tenant's first unit (the API lists active ones first): "Kamar 11, Kos Melati".
export function unitPlace(tenant: CheckTenant): string | null {
  const unit = tenant.units[0];
  return unit === undefined ? null : `${unit.unitLabel}, ${unit.propertyName}`;
}

// "Bu Wati Handayani · Kamar 11, Kos Melati", or the name alone without a unit.
export function tenantLine(tenant: CheckTenant): string {
  const place = unitPlace(tenant);
  return place === null ? tenant.name : `${tenant.name} · ${place}`;
}

// "Kurang Rp50.000" or "Lebih Rp50.000"; the gap is received minus expected.
export function gapText(gap: number): string {
  const size = formatRupiah(Math.abs(gap));
  return gap < 0 ? `Kurang ${size}` : `Lebih ${size}`;
}

// "Beda Rp50.000" between the QR and what AutoGoPay reported; null when no report is known.
export function qrisDifferenceText(
  mismatch: Pick<CheckQrisAmountMismatch, "qrAmount" | "reportedAmount">,
): string | null {
  if (mismatch.reportedAmount === undefined) return null;
  return `Beda ${formatRupiah(Math.abs(mismatch.reportedAmount - mismatch.qrAmount))}`;
}

export function arrivedText(createdAt: string): string {
  return `Masuk ${formatTimestampShortWeekdayDayMonthTime(createdAt)}`;
}

// "Foto kwitansi Bu Siti Aminah", or the purpose alone without a tenant.
export function uploadTitle(upload: CheckUploadInfected): string {
  const label = PURPOSE_LABEL[upload.purpose];
  return upload.tenant === undefined ? label : `${label} ${upload.tenant.name}`;
}

function uploadedByText(upload: CheckUploadInfected): string {
  return upload.uploadedBy === undefined
    ? "Dikirim penyewa"
    : `Diunggah ${upload.uploadedBy.name}`;
}

export function transferProofChip(proof: CheckTransferProof): DepositChip {
  return proof.uploadStatus === "clean"
    ? { label: "Siap dicek", tone: "active" }
    : { label: "Foto sedang diperiksa", tone: "neutral" };
}

export function qrisChip(mismatch: CheckQrisAmountMismatch): DepositChip {
  return {
    label: qrisDifferenceText(mismatch) ?? "Perlu dicek",
    tone: "warning",
  };
}

type RowContent = Pick<CheckRowView, "title" | "detail" | "chip">;

function rowContent(item: CheckQueueItem): RowContent | null {
  switch (item.kind) {
    case "transfer_proof": {
      const proof = item.transferProof;
      if (proof === undefined) return null;
      return {
        title: tenantLine(proof.tenant),
        detail: "Foto bukti dari halaman pembayaran",
        chip: transferProofChip(proof),
      };
    }
    case "deposit_gap": {
      const gap = item.depositGap;
      if (gap === undefined) return null;
      return {
        title: `${gap.depositor.name} · ${receiptCountText(gap.paymentCount)}`,
        detail: `Total kwitansi ${formatRupiah(gap.expectedAmount)} · diterima ${formatRupiah(gap.receivedAmount)}`,
        chip: { label: gapText(gap.gap), tone: "warning" },
      };
    }
    case "upload_infected": {
      const upload = item.uploadInfected;
      if (upload === undefined) return null;
      return {
        title: uploadTitle(upload),
        detail: uploadedByText(upload),
        chip: { label: "Berbahaya", tone: "warning" },
      };
    }
    case "qris_amount_mismatch": {
      const mismatch = item.qrisAmountMismatch;
      if (mismatch === undefined) return null;
      const reported =
        mismatch.reportedAmount === undefined
          ? "laporan AutoGoPay tidak diketahui"
          : `dilaporkan ${formatRupiah(mismatch.reportedAmount)}`;
      return {
        title: tenantLine(mismatch.tenant),
        detail: `QR ${formatRupiah(mismatch.qrAmount)} · ${reported}`,
        chip: qrisChip(mismatch),
      };
    }
    case "phone_changed": {
      const change = item.phoneChanged;
      if (change === undefined) return null;
      return {
        title: tenantLine(change.tenant),
        detail: `Dicatat ${change.recordedBy.name} saat kunjungan`,
        chip: { label: "Perlu diterapkan", tone: "neutral" },
      };
    }
  }
}

// One item as a row; null when the API sent it without the object its kind names. Every row
// opens its kind's drawer.
export function checkRowView(item: CheckQueueItem): CheckRowView | null {
  const content = rowContent(item);
  if (content === null) return null;
  return {
    id: item.id,
    kind: item.kind,
    kindLabel: KIND_LABEL[item.kind],
    icon: KIND_ICON[item.kind],
    iconTone: KIND_ICON_TONE[item.kind],
    ...content,
    arrived: arrivedText(item.createdAt),
    opens: true,
  };
}

export function checkRowViews(
  items: readonly CheckQueueItem[],
): CheckRowView[] {
  return items.flatMap((item) => {
    const row = checkRowView(item);
    return row === null ? [] : [row];
  });
}

export function depositGapSheetView(gap: CheckDepositGap): DepositGapSheetView {
  return {
    subtitle: `${gap.depositor.name} · dihitung ${gap.confirmedBy.name}, ${formatTimestampShortWeekdayDayMonthTime(gap.confirmedAt)}`,
    tiles: [
      {
        label: "Total kwitansi",
        value: formatRupiah(gap.expectedAmount),
        sub: receiptCountText(gap.paymentCount),
        tone: "plain",
      },
      {
        label: "Diterima admin",
        value: formatRupiah(gap.receivedAmount),
        sub: null,
        tone: "plain",
      },
      { label: "Selisih", value: gapText(gap.gap), sub: null, tone: "warning" },
    ],
    todo: `Tanyakan ke ${shortName(gap.depositor.name)}. Setelah selisihnya selesai, misalnya sisa uangnya diserahkan atau ternyata salah hitung, tutup dengan catatan.`,
  };
}

function uploadTodo(upload: CheckUploadInfected): string {
  const uploader =
    upload.uploadedBy === undefined
      ? "karyawan yang mengunggahnya"
      : shortName(upload.uploadedBy.name);
  switch (upload.purpose) {
    case "cash_receipt": {
      const receipt =
        upload.tenant === undefined
          ? "kwitansi ini"
          : `kwitansi ${upload.tenant.name}`;
      return `Minta ${uploader} memotret ${receipt} lagi. Di HP-nya kwitansi ini sudah tampil “Foto ditolak, ambil ulang”.`;
    }
    case "transfer_proof":
      return "Minta penyewa mengirim foto lagi dari halaman pembayaran.";
    case "cash_deposit":
      return `Minta ${uploader} memotret uang setoran lagi.`;
  }
}

export function uploadInfectedSheetView(
  upload: CheckUploadInfected,
): UploadInfectedSheetView {
  const by =
    upload.uploadedBy === undefined
      ? "dikirim penyewa"
      : `diunggah ${upload.uploadedBy.name}`;
  return {
    subtitle: `${uploadTitle(upload)} · ${by}`,
    scannedLine:
      upload.scannedAt === undefined
        ? "Sudah diamankan"
        : `Diperiksa ${formatTimestampShortWeekdayDayMonthTime(upload.scannedAt)} · sudah diamankan`,
    todo: uploadTodo(upload),
  };
}

export function qrisMismatchSheetView(
  mismatch: CheckQrisAmountMismatch,
): QrisMismatchSheetView {
  const tenant = mismatch.tenant.name;
  const reported =
    mismatch.reportedAmount === undefined
      ? {
          label: "Dilaporkan AutoGoPay",
          value: "Tidak diketahui",
          sub: "tercatat oleh pengecekan otomatis",
          tone: "plain" as const,
        }
      : {
          label: "Dilaporkan AutoGoPay",
          value: formatRupiah(mismatch.reportedAmount),
          sub: null,
          tone: "warning" as const,
        };
  const recorded =
    mismatch.recordedAmount === undefined
      ? []
      : [
          {
            label: "Tercatat",
            value: formatRupiah(mismatch.recordedAmount),
            sub: "di tagihan",
            tone: "plain" as const,
          },
        ];
  const recordedText =
    mismatch.recordedAmount === undefined
      ? ""
      : ` ${formatRupiah(mismatch.recordedAmount)}`;
  return {
    subtitle: tenantLine(mismatch.tenant),
    tiles: [
      {
        label: "Jumlah QR",
        value: formatRupiah(mismatch.qrAmount),
        sub: null,
        tone: "plain",
      },
      reported,
      ...recorded,
    ],
    callout: `Pembayaran sudah tercatat${recordedText} sesuai QR, dan tagihan ${tenant} sudah dihitung dari jumlah itu.`,
    transactionId: mismatch.transactionId,
    tenantId: mismatch.tenant.id,
    tenantLinkText: `Buka ${tenant}`,
  };
}

// `createdAt` is the item's: when the karyawan recorded the new number.
export function phoneChangeSheetView(
  change: CheckPhoneChanged,
  createdAt: string,
): PhoneChangeSheetView {
  const recordedBy = change.recordedBy.name;
  const current = formatPhone(change.currentPhone);
  const note = change.note?.trim() ?? "";
  return {
    subtitle: tenantLine(change.tenant),
    tiles: [
      {
        label: "Nomor sekarang",
        value: current,
        sub: "di data penyewa",
        tone: "plain",
      },
      {
        label: "Nomor baru",
        value: formatPhone(change.newPhone),
        sub: `dari kunjungan ${recordedBy}`,
        tone: "accent",
      },
    ],
    noteCaption:
      note === ""
        ? null
        : `Catatan ${recordedBy} · ${formatTimestampShortWeekdayDayMonthTime(createdAt)}`,
    note: note === "" ? null : `“${note}”`,
    keepLine: `Nomor ${change.tenant.name} tetap ${current}. Item ini hilang dari antrean.`,
    applyToast: `Nomor ${change.tenant.name} diganti`,
  };
}

// The friendly early checks of a note: required, trimmed, at most the API's limit. The Go API
// checks it again and its message wins.
function noteSchema(emptyMessage: string) {
  return z.object({
    note: z
      .string()
      .transform((text) => text.trim())
      .refine((text) => text !== "", { message: emptyMessage })
      .refine((text) => Array.from(text).length <= NOTE_MAX_LENGTH, {
        message: NOTE_TOO_LONG_MESSAGE,
      }),
  });
}

// Closing a deposit gap, an infected file or a QRIS mismatch.
export const dismissFormSchema = noteSchema(NOTE_EMPTY_MESSAGE);

// Why a new number is not used.
export const rejectFormSchema = noteSchema(REASON_EMPTY_MESSAGE);

export function dismissRequestBody(
  fields: NoteFormOutput,
): DismissCheckItemRequest {
  return { note: fields.note };
}

export function checkActionErrorKind(error: unknown): CheckActionErrorKind {
  if (!(error instanceof ApiError) || error.status !== 409) return "other";
  switch (error.code) {
    case "check_item_resolved":
      return "resolved";
    case "phone_taken":
      return "phone_taken";
    case "upload_not_ready":
      return "upload_not_ready";
    case "upload_refused":
      return "upload_refused";
    default:
      return "other";
  }
}

function done(title: string): ToastMessage {
  return { tone: "create", title, description: null };
}

export const DISMISS_TOAST: Record<
  Exclude<CheckQueueKind, "transfer_proof" | "phone_changed">,
  ToastMessage
> = {
  deposit_gap: done("Selisih ditutup"),
  upload_infected: done("Ditandai sudah ditangani"),
  qris_amount_mismatch: done("Ditandai sudah dicek"),
};

export const PHONE_REJECTED_TOAST: ToastMessage = {
  tone: "neutral",
  title: "Nomor baru tidak dipakai",
  description: null,
};

export function phoneAppliedToast(view: PhoneChangeSheetView): ToastMessage {
  return done(view.applyToast);
}

export const RESOLVED_TOAST: ToastMessage = {
  tone: "neutral",
  title: "Sudah ditangani admin lain",
  description: null,
};

export const COPIED_TOAST: ToastMessage = {
  tone: "neutral",
  title: "Disalin",
  description: TRANSACTION_ID_LABEL,
  icon: "copy",
};

export const COPY_FAILED_TOAST: ToastMessage = {
  tone: "caution",
  title: "Tidak bisa menyalin",
  description: "Salin ID transaksi secara manual.",
};

// "Bukti transfer": the friendly early checks of "Simpan pembayaran", the ones of "Catat
// pembayaran" without the method (a proof is always a transfer). The Go API checks everything
// again and its message wins. `today` is the Jakarta day.
export function proofMatchSchema(today: string) {
  return paymentFormSchema(today).omit({ method: true });
}

// The amount is empty when the drawer opens: the admin types what the bank statement shows, not
// what the photo says. The day is today in Jakarta.
export function proofMatchDefaults(today: string): ProofMatchValues {
  return { amount: "", receivedOn: today, note: "" };
}

// The request body: the proof's own tenant (no tenantId), the integer amount, the day, and the
// trimmed note only when there is one. One requestId per opening of the drawer, sent again
// unchanged on a retry.
export function proofMatchRequestBody(
  fields: ProofMatchOutput,
  requestId: string,
): MatchTransferProofRequest {
  const body: MatchTransferProofRequest = {
    requestId,
    amount: fields.amount,
    receivedOn: fields.receivedOn,
  };
  if (fields.note !== "") body.note = fields.note;
  return body;
}

// The amount to preview: the field's whole rupiah, or null while it holds none the API would take.
export function previewAmount(text: string): number | null {
  const amount = parseRupiahInput(text);
  if (amount === null || amount <= 0 || amount > PAYMENT_AMOUNT_MAX) {
    return null;
  }
  return amount;
}

// The photo's scan status: getUpload's once it has answered, else the queue's.
function proofStatus(
  upload: Upload | undefined,
  queued: CheckTransferProof["uploadStatus"],
): Upload["status"] {
  return upload?.status ?? queued;
}

// Whether "Simpan pembayaran" may go: the scan found the photo clean.
export function proofReady(
  upload: Upload | undefined,
  queued: CheckTransferProof["uploadStatus"],
): boolean {
  return proofStatus(upload, queued) === "clean";
}

// What the photo frame shows. `errorMessage` is getUpload's failure, already in Bahasa Indonesia.
export function proofPhotoState(
  upload: Upload | undefined,
  queued: CheckTransferProof["uploadStatus"],
  errorMessage: string | null,
): ProofPhotoState {
  if (upload === undefined && errorMessage !== null) {
    return { kind: "error", message: errorMessage };
  }
  switch (proofStatus(upload, queued)) {
    case "pending":
      return { kind: "pending" };
    case "rejected":
    case "infected":
      return { kind: "refused" };
    case "clean":
      if (upload === undefined) return { kind: "loading" };
      return upload.downloadUrl === undefined
        ? { kind: "error", message: PHOTO_ERROR_TEXT }
        : { kind: "clean", url: upload.downloadUrl };
  }
}

// When to ask getUpload again: every PROOF_POLL_MS while the photo is being scanned, shortly
// before a clean photo's link expires, never once the scan refused it.
export function uploadRefetchMs(
  upload: Upload | undefined,
  now: number,
): number | false {
  if (upload === undefined) return false;
  if (upload.status === "pending") return PROOF_POLL_MS;
  if (upload.status !== "clean" || upload.downloadExpiresAt === undefined) {
    return false;
  }
  const expires = Date.parse(upload.downloadExpiresAt);
  if (Number.isNaN(expires)) return false;
  return Math.max(expires - now - LINK_REFRESH_MARGIN_MS, PROOF_POLL_MS);
}

// The photo's text alternative: "Foto bukti transfer dari Bu Wati Handayani".
export function proofPhotoAlt(tenant: CheckTenant): string {
  return `${ZOOM_TITLE} dari ${tenant.name}`;
}

// When the tenant sent the proof (the item's createdAt), and the month of the link's invoice when
// the tenant's account lists it.
export function proofPhotoCaption(
  createdAt: string,
  invoiceId: string,
  invoices: readonly AccountInvoice[] | undefined,
): ProofPhotoCaption {
  const invoice = invoices?.find((each) => each.id === invoiceId);
  return {
    sentAt: formatTimestampShortWeekdayDayMonthTime(createdAt),
    link: invoice === undefined ? null : invoiceTitle(invoice),
  };
}

// "Pembayaran dari": the tenant, their unit and what they still owe over all their invoices
// (getTenantAccount's outstanding), each left out while unknown.
export function proofPayerView(
  tenant: CheckTenant,
  outstanding: number | undefined,
): ProofPayerView {
  const parts = [
    unitPlace(tenant),
    outstanding === undefined ? null : `sisa ${formatRupiah(outstanding)}`,
  ].filter((part) => part !== null);
  return {
    name: tenant.name,
    detail: parts.length === 0 ? null : parts.join(" · "),
  };
}

// The API's preview as lines: which invoice becomes "Lunas", which is paid in part and what it
// would still owe, and the credit the amount would add. The unit is named only when the amount
// reaches invoices of more than one unit.
export function proofPreviewView(
  preview: TenantPaymentPreview,
): ProofPreviewView {
  const units = new Set(
    preview.items.map((item) => `${item.propertyName}|${item.unitLabel}`),
  );
  return {
    amount: preview.amount,
    lines: preview.items.map((item) => {
      const name =
        units.size > 1
          ? `${invoiceName(item)} · ${item.unitLabel}`
          : invoiceName(item);
      return item.paidInFull
        ? { id: item.invoiceId, lead: `${name} jadi`, result: "Lunas" }
        : {
            id: item.invoiceId,
            lead: `${name}:`,
            result: `Sebagian, sisa ${formatRupiah(item.balanceAfter)}`,
          };
    }),
    credit:
      preview.creditAdded > 0
        ? `Kelebihan ${formatRupiah(preview.creditAdded)} jadi saldo`
        : null,
  };
}

// The toast once the API recorded the payment: the amount it answered and the tenant's name.
export function proofMatchedToast(
  result: MatchTransferProofResult,
  tenantName: string,
): ToastMessage {
  return {
    tone: "create",
    title: "Pembayaran disimpan",
    description: `${formatRupiah(result.payment.payment.amount)} dari ${tenantName}`,
  };
}

export const PROOF_DISMISSED_TOAST: ToastMessage = {
  tone: "neutral",
  title: "Ditandai bukan pembayaran",
  description: null,
};

// The scan refused the photo while the drawer was open: it closes, and the proof is listed again
// as "File ditolak".
export const UPLOAD_REFUSED_TOAST: ToastMessage = {
  tone: "caution",
  title: "Foto bukti ditolak",
  description: "Pemindai menolak foto ini. Lihat di File ditolak.",
};
