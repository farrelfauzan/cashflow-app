import type { LeaseInput, Unit } from "@/lib/api/generated/model";
import { formatDate, formatDayMonth } from "@/lib/format/date";
import { formatPhone } from "@/lib/format/phone";
import { formatRupiah, parseRupiahInput } from "@/lib/format/rupiah";
import { groupRupiahDigits } from "@/lib/format/rupiah-input";

import type {
  DateOption,
  LeaseFormErrors,
  LeaseFormField,
  LeaseFormResult,
  LeaseFormValues,
  StartTiming,
} from "./type";

// Calendar dates here are YYYY-MM-DD strings. They compare correctly as strings, and the math is
// plain UTC date arithmetic, so no timezone can move a day. "Today" is always the Jakarta day,
// passed in by the caller.

const CALENDAR_DATE = /^(\d{4})-(\d{2})-(\d{2})$/;

function parts(date: string): [year: number, month: number, day: number] {
  const match = CALENDAR_DATE.exec(date);
  if (match === null) throw new RangeError(`expects YYYY-MM-DD, got ${date}`);
  return [Number(match[1]), Number(match[2]), Number(match[3])];
}

function isoDate(utc: Date): string {
  return utc.toISOString().slice(0, 10);
}

// Whether a date input's value is a real calendar day.
export function isCalendarDate(date: string): boolean {
  if (!CALENDAR_DATE.test(date)) return false;
  const [year, month, day] = parts(date);
  const utc = new Date(Date.UTC(year, month - 1, day));
  return (
    utc.getUTCFullYear() === year &&
    utc.getUTCMonth() === month - 1 &&
    utc.getUTCDate() === day
  );
}

export function addDays(date: string, days: number): string {
  const [year, month, day] = parts(date);
  return isoDate(new Date(Date.UTC(year, month - 1, day + days)));
}

// The anniversary rule mirrors the API's (services/api/internal/tenant/anniversary.go:
// Anniversary, IsAnniversary, FirstAnniversaryAfter), which stays the judge: its 400 on
// billedFrom is shown under the field.

// The start date plus n months (n >= 0). When that month is shorter than the start day it is the
// month's last day, always counted from the start date: 31 Jan gives 28 Feb, then 31 Mar.
export function anniversary(start: string, n: number): string {
  const [year, month, day] = parts(start);
  // Day 0 of the month after is the last day of the month n months on.
  const lastDay = new Date(Date.UTC(year, month + n, 0)).getUTCDate();
  return isoDate(
    new Date(Date.UTC(year, month - 1 + n, Math.min(day, lastDay))),
  );
}

// The whole calendar months from the start's month to the date's month.
function monthsBetween(start: string, date: string): number {
  const [startYear, startMonth] = parts(start);
  const [year, month] = parts(date);
  return (year - startYear) * 12 + (month - startMonth);
}

// Whether date is the start date or one of its anniversaries.
export function isAnniversary(start: string, date: string): boolean {
  if (date < start) return false;
  return anniversary(start, monthsBetween(start, date)) === date;
}

// The first anniversary strictly after date; the start date for a date before it.
export function firstAnniversaryAfter(start: string, date: string): string {
  if (date < start) return start;
  let n = monthsBetween(start, date);
  while (anniversary(start, n) <= date) n++;
  return anniversary(start, n);
}

// The anniversary that begins the period date falls in (date on or after the start).
export function periodStartOn(start: string, date: string): string {
  let n = monthsBetween(start, date);
  while (n > 0 && anniversary(start, n) > date) n--;
  return anniversary(start, n);
}

export function startTiming(start: string, today: string): StartTiming {
  if (start < today) return "past";
  if (start > today) return "future";
  return "today";
}

// The later anniversaries "Mulai tagih dari" may pick for a new lease that started before today:
// from the second period up to the first anniversary after today, as the API allows (no later
// than the first anniversary after the later of the start date and the day it is entered, which
// is today for a new lease). The start date itself is the first radio choice.
export function billedFromOptions(start: string, today: string): DateOption[] {
  const latest = firstAnniversaryAfter(start, today > start ? today : start);
  const options: DateOption[] = [];
  for (let n = 1; ; n++) {
    const date = anniversary(start, n);
    if (date > latest) break;
    options.push({ value: date, label: formatDate(date) });
  }
  return options;
}

// The choice the select starts on: the period running today (as on the board), or the first one
// when the lease is still in its first period.
export function defaultBilledFrom(
  options: readonly DateOption[],
  today: string,
): string {
  let chosen = options[0]?.value ?? "";
  for (const option of options) {
    if (option.value <= today) chosen = option.value;
  }
  return chosen;
}

// The box under "Mulai tagih dari" for a start before today, with the real dates. Past periods are
// due today and the current period 7 days after (CLAUDE.md "A lease entered late"); billing from
// the next anniversary leaves them all to the owner, and its invoice comes as usual.
export function billingExplanation(
  start: string,
  billedFrom: string,
  today: string,
): string {
  const current = periodStartOn(start, today);
  const currentDue = formatDayMonth(addDays(today, 7));
  if (billedFrom > today) {
    return `Bulan sebelum ${formatDate(billedFrom)} tidak ditagih. Tagihan pertama untuk bulan mulai ${formatDate(billedFrom)}, jatuh tempo ${formatDayMonth(billedFrom)}.`;
  }
  if (billedFrom === current) {
    return `Tanggal mulai ini sudah lewat, jadi bulan berjalan sejak ${formatDate(billedFrom)} ikut ditagih, jatuh tempo ${currentDue}.`;
  }
  return `Tanggal mulai ini sudah lewat, jadi setiap bulan sejak ${formatDate(billedFrom)} ikut ditagih. Bulan yang sudah lewat jatuh tempo hari ini (${formatDayMonth(today)}); bulan berjalan jatuh tempo ${currentDue}. Kalau pemilik sudah menerima uang sewa bulan-bulan itu langsung, pilih bulan mulai tagih yang lebih baru.`;
}

// The first radio's line: "Semua bulan sejak 15 Sep 2026 dibuatkan tagihan."
export function billAllHint(start: string): string {
  return `Semua bulan sejak ${formatDate(start)} dibuatkan tagihan.`;
}

export const BILL_PICK_HINT =
  "Bulan sebelumnya dianggap sudah dibayar langsung kepada pemilik, jadi tidak dibuatkan tagihan.";

export const BILL_PICK_SELECT_HINT =
  "Pilihan bulan hanya berlaku kalau opsi kedua dipilih.";

export const BOOKING_NOTE =
  "Tanggal mulai ada di masa depan, jadi sewa ini dihitung sebagai pesanan: unit langsung terisi, dan tagihan pertama dibuat menjelang tanggal mulai. Pilihan Mulai tagih dari tidak diperlukan.";

const MONTH_END_NOTE =
  "Tanggal 29 sampai 31 jatuh di hari terakhir bulan yang lebih pendek.";

// The hint under "Mulai sewa", with the day of the chosen date.
export function startDateHint(start: string): string {
  if (!isCalendarDate(start)) {
    return `Juga jadi tanggal tagihan tiap bulan. ${MONTH_END_NOTE}`;
  }
  const day = parts(start)[2];
  return `Juga jadi tanggal tagihan tiap bulan. Mulai tanggal ${day}, jatuh tempo tiap tanggal ${day}. ${MONTH_END_NOTE}`;
}

// The panel's subtitle: "Bu Dewi Lestari · 0812-3456-7899".
export function leaseFormSubtitle(name: string, phone: string): string {
  return `${name} · ${formatPhone(phone)}`;
}

// The units a new lease may take. A display filter only: the API refuses an occupied unit.
export function freeUnits(units: readonly Unit[]): Unit[] {
  return units.filter((unit) => !unit.occupied);
}

// The rent field's starting text for a unit: its usual rent, or nothing.
export function rentTextFor(
  unit: Pick<Unit, "defaultRent"> | undefined,
): string {
  return unit?.defaultRent === undefined
    ? ""
    : groupRupiahDigits(String(unit.defaultRent));
}

// "Usulan dari sewa biasa unit: Rp1.250.000.", none when the unit has no usual rent.
export function rentSuggestion(
  unit: Pick<Unit, "defaultRent"> | undefined,
): string | null {
  return unit?.defaultRent === undefined
    ? null
    : `Usulan dari sewa biasa unit: ${formatRupiah(unit.defaultRent)}.`;
}

// The collector select's value for none ("Tidak ada"); a select item cannot hold "".
export const NO_COLLECTOR = "none";
export const NO_COLLECTOR_LABEL = "Tidak ada";
export const COLLECTOR_HINT =
  "Karyawan yang menagih penyewa ini. Boleh dikosongkan.";
export const FREE_UNITS_HINT = "Hanya unit yang masih kosong.";
export const NO_FREE_UNIT_TEXT = "Semua unit di properti ini sudah terisi.";
export const NO_PROPERTY_TEXT =
  "Belum ada properti. Tambahkan properti dan unitnya dulu.";
export const DEPOSIT_HINT = "Isi 0 kalau tidak ada.";

export const NO_PROPERTY_MESSAGE = "Pilih properti.";
export const NO_UNIT_MESSAGE = "Pilih unit.";
export const NO_START_MESSAGE = "Isi tanggal mulai sewa.";
export const NO_BILLED_FROM_MESSAGE = "Pilih bulan mulai tagih.";
export const BAD_RENT_MESSAGE = "Isi sewa per bulan dengan angka.";
export const BAD_DEPOSIT_MESSAGE =
  "Isi uang jaminan dengan angka, atau kosongkan.";

// The fields the API's errors can name; any other field goes to the alert.
export const LEASE_API_FIELDS: readonly LeaseFormField[] = [
  "unitId",
  "startDate",
  "billedFrom",
  "rentAmount",
  "depositAmount",
  "collectorMemberId",
];

// The fields in the form's order, for moving focus to the first bad one.
export const LEASE_FIELD_ORDER: readonly LeaseFormField[] = [
  "propertyId",
  "unitId",
  "startDate",
  "billedFrom",
  "rentAmount",
  "depositAmount",
  "collectorMemberId",
];

export function firstLeaseError(
  errors: LeaseFormErrors,
): LeaseFormField | null {
  return LEASE_FIELD_ORDER.find((field) => errors[field] !== undefined) ?? null;
}

export function isLeaseFormField(
  field: string | null,
): field is LeaseFormField {
  return LEASE_FIELD_ORDER.some((known) => known === field);
}

// What the lease form checks before it sends anything, and the request it sends: amounts as
// integers, billedFrom only for a start before today with the second choice, the deposit and the
// collector only when given. The Go API validates everything again and its message wins.
export function checkLeaseForm(
  values: LeaseFormValues,
  propertyId: string,
  today: string,
): LeaseFormResult {
  const errors: LeaseFormErrors = {};
  if (propertyId === "") errors.propertyId = NO_PROPERTY_MESSAGE;
  if (values.unitId === "") errors.unitId = NO_UNIT_MESSAGE;
  const startOk = isCalendarDate(values.startDate);
  if (!startOk) errors.startDate = NO_START_MESSAGE;

  const billedFrom =
    startOk &&
    startTiming(values.startDate, today) === "past" &&
    values.billing === "pick"
      ? values.billedFrom
      : undefined;
  if (
    billedFrom !== undefined &&
    !billedFromOptions(values.startDate, today).some(
      (option) => option.value === billedFrom,
    )
  ) {
    errors.billedFrom = NO_BILLED_FROM_MESSAGE;
  }

  const rent = parseRupiahInput(values.rent);
  if (rent === null || rent < 1) errors.rentAmount = BAD_RENT_MESSAGE;

  let deposit: number | undefined;
  if (values.deposit.trim() !== "") {
    const parsed = parseRupiahInput(values.deposit);
    if (parsed === null) errors.depositAmount = BAD_DEPOSIT_MESSAGE;
    else deposit = parsed;
  }

  if (Object.keys(errors).length > 0 || rent === null) {
    return { ok: false, errors };
  }
  const input: LeaseInput = {
    tenantId: values.tenantId,
    unitId: values.unitId,
    startDate: values.startDate,
    rentAmount: rent,
  };
  if (deposit !== undefined) input.depositAmount = deposit;
  if (values.collector !== NO_COLLECTOR) {
    input.collectorMemberId = values.collector;
  }
  if (billedFrom !== undefined) input.billedFrom = billedFrom;
  return { ok: true, input };
}
