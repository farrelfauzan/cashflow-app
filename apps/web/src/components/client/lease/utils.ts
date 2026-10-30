import type {
  ChargeType,
  Collector,
  CollectorCandidate,
  Lease,
  LeaseCharge,
  LeaseInput,
  LeaseTerms,
  TermMonths,
  Unit,
  UnitTerm,
} from "@/lib/api/generated/model";
import { formatDate, formatDayMonth } from "@/lib/format/date";
import { formatPhone } from "@/lib/format/phone";
import { formatRupiah, parseRupiahInput } from "@/lib/format/rupiah";
import { groupRupiahDigits } from "@/lib/format/rupiah-input";

import type {
  BillingChoice,
  ChargeEntries,
  ChargeEntry,
  ChargeErrors,
  ChargeRow,
  ChargeTypeNameCheck,
  CollectorOption,
  DateCheck,
  DateOption,
  EditStartBox,
  LeaseChargesResult,
  LeaseFormErrors,
  LeaseFormField,
  LeaseFormResult,
  LeaseFormValues,
  LeaseTermsResult,
  LeaseTermsValues,
  PlannedEndSuggestion,
  StartTiming,
  TermOption,
  TermPeriodNames,
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

// The period rule mirrors the API's (services/api/internal/tenant/anniversary.go: Anniversary, and
// period.go: PeriodStart, PeriodIndex, IsPeriodStart, NextPeriodStart), which stays the judge: its
// 400 on billedFrom is shown under the field. A lease is billed once per term (1, 3, 6 or 12
// months); its periods start on the start date and the same day whole terms later. Go takes the
// term before the date; here it comes last, after the arguments the monthly rule always had.

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

// The day the lease's n-th period (n >= 0) begins: the start date plus n terms, with the month-end
// rule of anniversary, so it is always counted from the start date (a 3-month lease from 30 Nov
// has 28 Feb, then 30 May).
export function periodStart(
  start: string,
  n: number,
  term: TermMonths,
): string {
  return anniversary(start, n * term);
}

// The n of the period date falls in, or -1 for a date before the start.
function periodIndex(start: string, date: string, term: TermMonths): number {
  if (date < start) return -1;
  // A guess from the calendar months between them; the loops correct it by a period or so.
  let n = Math.floor(monthsBetween(start, date) / term);
  while (n > 0 && periodStart(start, n, term) > date) n--;
  while (periodStart(start, n + 1, term) <= date) n++;
  return n;
}

// Whether date is the start date or the start of a later period of the term.
export function isPeriodStart(
  start: string,
  date: string,
  term: TermMonths,
): boolean {
  const n = periodIndex(start, date, term);
  return n >= 0 && periodStart(start, n, term) === date;
}

// The first period start strictly after date; the start date for a date before it.
export function nextPeriodStart(
  start: string,
  date: string,
  term: TermMonths,
): string {
  return periodStart(start, periodIndex(start, date, term) + 1, term);
}

// The start of the period date falls in (date on or after the start).
export function periodStartOn(
  start: string,
  date: string,
  term: TermMonths,
): string {
  return periodStart(start, Math.max(0, periodIndex(start, date, term)), term);
}

export function startTiming(start: string, today: string): StartTiming {
  if (start < today) return "past";
  if (start > today) return "future";
  return "today";
}

// The later period starts "Mulai tagih dari" may pick for a lease that started before the day it
// is entered: from the second period up to the first period start after that day, as the API
// allows (no later than the first period start after the later of the start date and the day it
// is entered). That day is today for a new lease and the lease's enteredOn for one being changed.
// The start date itself is the first radio choice.
export function billedFromOptions(
  start: string,
  enteredOn: string,
  term: TermMonths,
): DateOption[] {
  const latest = nextPeriodStart(
    start,
    enteredOn > start ? enteredOn : start,
    term,
  );
  const options: DateOption[] = [];
  for (let n = 1; ; n++) {
    const date = periodStart(start, n, term);
    if (date > latest) break;
    options.push({ value: date, label: formatDate(date) });
  }
  return options;
}

// The choice the select starts on: the period running on the day the lease is entered (today
// for a new lease, as on the board), or the first one when the lease was still in its first
// period then.
export function defaultBilledFrom(
  options: readonly DateOption[],
  enteredOn: string,
): string {
  let chosen = options[0]?.value ?? "";
  for (const option of options) {
    if (option.value <= enteredOn) chosen = option.value;
  }
  return chosen;
}

// How the lease forms name one period of each term. A monthly lease keeps the words the boards
// use ("bulan"); a longer term says "periode 3 bulan" or "tahun".
export const TERM_PERIOD_NAMES: Record<TermMonths, TermPeriodNames> = {
  1: { noun: "bulan", short: "bulan", plural: "bulan-bulan" },
  3: { noun: "periode 3 bulan", short: "periode", plural: "periode-periode" },
  6: { noun: "periode 6 bulan", short: "periode", plural: "periode-periode" },
  12: { noun: "tahun", short: "tahun", plural: "tahun-tahun" },
};

function capitalized(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1);
}

// The box under "Mulai tagih dari" for a start before the day the lease is entered, with the real
// dates. Past periods are due on that day and the period running then 7 days after (CLAUDE.md "A
// lease entered late"); billing from the next period start leaves them all to the owner, and its
// invoice comes as usual. For a new lease that day is today; for a lease being changed it is its
// enteredOn, named as such.
export function billingExplanation(
  start: string,
  billedFrom: string,
  enteredOn: string,
  today: string,
  term: TermMonths,
): string {
  const { noun, short, plural } = TERM_PERIOD_NAMES[term];
  const current = periodStartOn(start, enteredOn, term);
  const currentDue = formatDayMonth(addDays(enteredOn, 7));
  const pastDue =
    enteredOn === today
      ? `hari ini (${formatDayMonth(enteredOn)})`
      : `${formatDayMonth(enteredOn)}, hari sewa ini dimasukkan`;
  if (billedFrom > enteredOn) {
    return `${capitalized(short)} sebelum ${formatDate(billedFrom)} tidak ditagih. Tagihan pertama untuk ${short} mulai ${formatDate(billedFrom)}, jatuh tempo ${formatDayMonth(billedFrom)}.`;
  }
  if (billedFrom === current) {
    return `Tanggal mulai ini sudah lewat, jadi ${short} berjalan sejak ${formatDate(billedFrom)} ikut ditagih, jatuh tempo ${currentDue}.`;
  }
  return `Tanggal mulai ini sudah lewat, jadi setiap ${noun} sejak ${formatDate(billedFrom)} ikut ditagih. ${capitalized(short)} yang sudah lewat jatuh tempo ${pastDue}; ${short} berjalan jatuh tempo ${currentDue}. Kalau pemilik sudah menerima uang sewa ${plural} itu langsung, pilih ${short} mulai tagih yang lebih baru.`;
}

// The first radio: "Tagih semua bulan sejak tanggal mulai", "Tagih semua tahun sejak tanggal mulai".
export function billAllLabel(term: TermMonths): string {
  return `Tagih semua ${TERM_PERIOD_NAMES[term].noun} sejak tanggal mulai`;
}

// The first radio's line: "Semua bulan sejak 15 Sep 2026 dibuatkan tagihan."
export function billAllHint(start: string, term: TermMonths): string {
  return `Semua ${TERM_PERIOD_NAMES[term].noun} sejak ${formatDate(start)} dibuatkan tagihan.`;
}

// The second radio: "Mulai dari bulan yang saya pilih", "Mulai dari periode yang saya pilih".
export function billPickLabel(term: TermMonths): string {
  return `Mulai dari ${TERM_PERIOD_NAMES[term].short} yang saya pilih`;
}

export function billPickHint(term: TermMonths): string {
  return `${capitalized(TERM_PERIOD_NAMES[term].short)} sebelumnya dianggap sudah dibayar langsung kepada pemilik, jadi tidak dibuatkan tagihan.`;
}

// The period select's accessible name: "Bulan mulai tagih", "Tahun mulai tagih".
export function billedFromSelectLabel(term: TermMonths): string {
  return `${capitalized(TERM_PERIOD_NAMES[term].short)} mulai tagih`;
}

export function billPickSelectHint(term: TermMonths): string {
  return `Pilihan ${TERM_PERIOD_NAMES[term].short} hanya berlaku kalau opsi kedua dipilih.`;
}

export function noBilledFromMessage(term: TermMonths): string {
  return `Pilih ${TERM_PERIOD_NAMES[term].short} mulai tagih.`;
}

export const BOOKING_NOTE =
  "Tanggal mulai ada di masa depan, jadi sewa ini dihitung sebagai pesanan: unit langsung terisi, dan tagihan pertama dibuat menjelang tanggal mulai. Pilihan Mulai tagih dari tidak diperlukan.";

const MONTH_END_NOTE =
  "Tanggal 29 sampai 31 jatuh di hari terakhir bulan yang lebih pendek.";

// The hint under "Mulai sewa", with the day of the chosen date: the day of the month for a term in
// months, the day and month for a yearly lease.
export function startDateHint(start: string, term: TermMonths): string {
  const every = `Juga jadi tanggal tagihan tiap ${TERM_PERIOD_WORDS[term]}.`;
  if (!isCalendarDate(start)) return `${every} ${MONTH_END_NOTE}`;
  if (term === 12) {
    const dayMonth = formatDayMonth(start);
    return `${every} Mulai ${dayMonth}, jatuh tempo tiap ${dayMonth}. ${MONTH_END_NOTE}`;
  }
  const day = parts(start)[2];
  const due =
    term === 1
      ? `jatuh tempo tiap tanggal ${day}`
      : `jatuh tempo tanggal ${day} tiap ${TERM_PERIOD_WORDS[term]}`;
  return `${every} Mulai tanggal ${day}, ${due}. ${MONTH_END_NOTE}`;
}

// The panel's subtitle: "Bu Dewi Lestari · 0812-3456-7899".
export function leaseFormSubtitle(name: string, phone: string): string {
  return `${name} · ${formatPhone(phone)}`;
}

// The units a new lease may take. A display filter only: the API refuses an occupied unit.
export function freeUnits(units: readonly Unit[]): Unit[] {
  return units.filter((unit) => !unit.occupied);
}

// The words for one period of a term, after "per": "Sewa per 3 bulan", "Rp15.000.000 per tahun".
export const TERM_PERIOD_WORDS: Record<TermMonths, string> = {
  1: "bulan",
  3: "3 bulan",
  6: "6 bulan",
  12: "tahun",
};

// The shortest of the given terms, or null when there is none.
export function shortestTerm(
  terms: readonly Pick<UnitTerm, "months">[] | undefined,
): TermMonths | null {
  let shortest: TermMonths | null = null;
  for (const term of terms ?? []) {
    if (shortest === null || term.months < shortest) shortest = term.months;
  }
  return shortest;
}

// The term "Masa sewa" starts on for a unit: its shortest term with a price in force, or null when
// no unit is chosen or none of its terms has a price (a lease on an unpriced term is refused with
// no_price, so the form says so before sending).
export function leaseTermFor(
  unit: Pick<Unit, "terms"> | undefined,
): TermMonths | null {
  return shortestTerm(
    (unit?.terms ?? []).filter((term) => term.price !== undefined),
  );
}

// The term "Masa sewa" shows: the one chosen while the unit still offers it with a price, else
// the unit's default (leaseTermFor). The choice is null until the admin picks one, and goes back
// to null when the unit changes.
export function selectedTerm(
  unit: Pick<Unit, "terms"> | undefined,
  chosen: TermMonths | null,
): TermMonths | null {
  const offered = (unit?.terms ?? []).some(
    (term) => term.months === chosen && term.price !== undefined,
  );
  return offered ? chosen : leaseTermFor(unit);
}

// The term a "Masa sewa" value names, when the unit offers it; null for anything else.
export function termFromValue(
  unit: Pick<Unit, "terms"> | undefined,
  value: string,
): TermMonths | null {
  return (
    (unit?.terms ?? []).find((term) => String(term.months) === value)?.months ??
    null
  );
}

// One "Masa sewa" choice: "Per tahun · Rp15.000.000", or "Per 3 bulan · belum ada harga" for a
// term with no price in force, which cannot be picked.
export function termOptionLabel(
  term: Pick<UnitTerm, "months" | "price">,
): string {
  const price =
    term.price === undefined ? NO_TERM_PRICE_TEXT : formatRupiah(term.price);
  return `Per ${TERM_PERIOD_WORDS[term.months]} · ${price}`;
}

// The unit's terms as "Masa sewa" choices, shortest first; none before a unit is chosen.
export function termOptions(
  unit: Pick<Unit, "terms"> | undefined,
): TermOption[] {
  return [...(unit?.terms ?? [])]
    .sort((a, b) => a.months - b.months)
    .map((term) => ({
      value: String(term.months),
      label: termOptionLabel(term),
      disabled: term.price === undefined,
    }));
}

// The hint under "Masa sewa": how often the chosen term is billed. A period's rent is the unit's
// price in force on the day its invoice is made (CLAUDE.md "Billing and reminders").
export function termHint(term: TermMonths | null): string {
  if (term === null) return TERM_NO_CHOICE_HINT;
  return `Tagihan dibuat sekali tiap ${TERM_PERIOD_WORDS[term]}, dibayar di muka. Harganya ikut harga unit yang berlaku saat tagihan dibuat.`;
}

export const NO_TERM_PRICE_TEXT = "belum ada harga";
export const TERM_NO_UNIT_PLACEHOLDER = "Pilih unit dulu";
export const TERM_PLACEHOLDER = "Pilih masa sewa";
export const TERM_NO_CHOICE_HINT =
  "Pilihan masa sewa dan harganya mengikuti unit.";

// The collector select's choices: "Tidak ada", then the candidates. The lease's own collector is
// added when they are no longer a candidate (a disabled member), so the select can still show
// them: the API lets a lease keep its collector.
export function collectorOptions(
  candidates: readonly CollectorCandidate[],
  current: Pick<Collector, "memberId" | "name"> | undefined,
): CollectorOption[] {
  const options = [
    { value: NO_COLLECTOR, label: NO_COLLECTOR_LABEL },
    ...candidates.map((c) => ({ value: c.memberId, label: c.name })),
  ];
  if (
    current !== undefined &&
    !candidates.some((c) => c.memberId === current.memberId)
  ) {
    options.push({ value: current.memberId, label: current.name });
  }
  return options;
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
export const NO_PRICE_MESSAGE =
  "Unit ini belum punya harga sewa. Atur harganya di halaman properti.";
export const BAD_DEPOSIT_MESSAGE =
  "Isi uang jaminan dengan angka, atau kosongkan.";

// The fields the API's errors can name; any other field goes to the alert.
export const LEASE_API_FIELDS: readonly LeaseFormField[] = [
  "unitId",
  "termMonths",
  "startDate",
  "billedFrom",
  "depositAmount",
  "collectorMemberId",
];

// The fields in the form's order, for moving focus to the first bad one.
export const LEASE_FIELD_ORDER: readonly LeaseFormField[] = [
  "propertyId",
  "unitId",
  "termMonths",
  "startDate",
  "billedFrom",
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

// The deposit of a lease form as an integer, undefined when the field is empty. A message goes
// into errors for a deposit that is typed but not a whole amount.
export function checkDeposit(
  values: Pick<LeaseTermsValues, "deposit">,
  errors: LeaseFormErrors,
): number | undefined {
  if (values.deposit.trim() === "") return undefined;
  const parsed = parseRupiahInput(values.deposit);
  if (parsed === null) errors.depositAmount = BAD_DEPOSIT_MESSAGE;
  return parsed ?? undefined;
}

// What the lease form checks before it sends anything, and the request it sends: amounts as
// integers, the chosen term, billedFrom (a period start of that term) only for a start before
// today with the second choice, the deposit and the collector only when given. A chosen unit with
// no priced term is refused under "Masa sewa", as the API would with no_price. The Go API
// validates everything again and its message wins.
export function checkLeaseForm(
  values: LeaseFormValues,
  propertyId: string,
  today: string,
): LeaseFormResult {
  const errors: LeaseFormErrors = {};
  if (propertyId === "") errors.propertyId = NO_PROPERTY_MESSAGE;
  if (values.unitId === "") errors.unitId = NO_UNIT_MESSAGE;
  else if (values.termMonths === null) errors.termMonths = NO_PRICE_MESSAGE;
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
    values.termMonths !== null &&
    !billedFromOptions(values.startDate, today, values.termMonths).some(
      (option) => option.value === billedFrom,
    )
  ) {
    errors.billedFrom = noBilledFromMessage(values.termMonths);
  }

  const deposit = checkDeposit(values, errors);

  if (Object.keys(errors).length > 0 || values.termMonths === null) {
    return { ok: false, errors };
  }
  const input: LeaseInput = {
    tenantId: values.tenantId,
    unitId: values.unitId,
    startDate: values.startDate,
    termMonths: values.termMonths,
  };
  if (deposit !== undefined) input.depositAmount = deposit;
  if (values.collector !== NO_COLLECTOR) {
    input.collectorMemberId = values.collector;
  }
  if (billedFrom !== undefined) input.billedFrom = billedFrom;
  return { ok: true, input };
}

// "Ubah sewa" (boards Sewa-Ubah.dc.html and Sewa-Ubah-Phone.dc.html).

export const EDIT_LEASE_NOTE =
  "Unit dan penyewa tidak bisa diganti. Untuk pindah unit, catat pindah keluar, lalu buat sewa baru.";
export const DISABLED_COLLECTOR_HINT =
  "Penagih ini sudah dinonaktifkan. Pilih penagih baru.";

// The fields the API's errors on a lease's terms can name; any other field goes to the alert.
export const LEASE_TERMS_API_FIELDS: readonly LeaseFormField[] = [
  "startDate",
  "billedFrom",
  "depositAmount",
  "collectorMemberId",
];

// What shows under "Mulai sewa" while a lease's terms are changed: "Mulai tagih dari" for a start
// before the day the lease was entered (a lease entered late, the only case the API lets skip
// periods), the booking note for a start after today, and nothing in between.
export function editStartBox(
  start: string,
  enteredOn: string,
  today: string,
): EditStartBox {
  if (!isCalendarDate(start)) return "none";
  if (start < enteredOn) return "billing";
  if (start > today) return "booking";
  return "none";
}

// The terms form's starting values, from the lease: amounts as grouped digits, its collector or
// none, and the second billing choice when it skipped periods.
export function leaseTermsValues(
  lease: Pick<
    Lease,
    "startDate" | "billedFrom" | "depositAmount" | "collector"
  >,
): LeaseTermsValues {
  const skipped = lease.billedFrom !== lease.startDate;
  const billing: BillingChoice = skipped ? "pick" : "start";
  return {
    startDate: lease.startDate,
    billing,
    billedFrom: skipped ? lease.billedFrom : "",
    deposit: groupRupiahDigits(String(lease.depositAmount)),
    collector: lease.collector?.memberId ?? NO_COLLECTOR,
  };
}

// Whether the collector select shows the lease's own collector who has been disabled: the select
// gets the warning border and DISABLED_COLLECTOR_HINT.
export function keepsDisabledCollector(
  selected: string,
  current: Pick<Collector, "memberId" | "active"> | undefined,
): boolean {
  return (
    current !== undefined && !current.active && selected === current.memberId
  );
}

// What "Ubah sewa" checks and sends. PUT replaces the terms, so the whole LeaseTerms goes: the
// deposit (0 when empty) and the collector only when one is chosen ("Tidak ada" clears it).
// billedFrom follows the API's rule for a lease being changed: while "Mulai tagih dari" shows, the
// start date for the first choice or the chosen period start; otherwise left out when the start
// date is unchanged (the API keeps the stored value) and the new start date when it changed.
export function checkLeaseTerms(
  values: LeaseTermsValues,
  lease: Pick<Lease, "startDate" | "enteredOn" | "termMonths">,
): LeaseTermsResult {
  const errors: LeaseFormErrors = {};
  const startOk = isCalendarDate(values.startDate);
  if (!startOk) errors.startDate = NO_START_MESSAGE;

  let billedFrom: string | undefined;
  if (startOk && values.startDate < lease.enteredOn) {
    if (values.billing === "start") {
      billedFrom = values.startDate;
    } else {
      billedFrom = values.billedFrom;
      const offered = billedFromOptions(
        values.startDate,
        lease.enteredOn,
        lease.termMonths,
      );
      if (!offered.some((option) => option.value === billedFrom)) {
        errors.billedFrom = noBilledFromMessage(lease.termMonths);
      }
    }
  } else if (startOk && values.startDate !== lease.startDate) {
    billedFrom = values.startDate;
  }

  const deposit = checkDeposit(values, errors);

  if (Object.keys(errors).length > 0) return { ok: false, errors };
  const input: LeaseTerms = {
    startDate: values.startDate,
    depositAmount: deposit ?? 0,
  };
  if (values.collector !== NO_COLLECTOR) {
    input.collectorMemberId = values.collector;
  }
  if (billedFrom !== undefined) input.billedFrom = billedFrom;
  return { ok: true, input };
}

// "Unit: Kamar 07 · Kos Melati".
export function leaseUnitText(
  lease: Pick<Lease, "unitLabel" | "propertyName">,
): string {
  return `${lease.unitLabel} · ${lease.propertyName}`;
}

// "Rencana keluar" (boards Sewa-Rencana.dc.html and Sewa-Rencana-Phone.dc.html) and "Catat pindah
// keluar" (Sewa-Pindah.dc.html). The API checks the same rules and its message wins.

export const NO_DATE_MESSAGE = "Isi tanggalnya.";
export const PLANNED_END_BEFORE_START_MESSAGE =
  "Rencana keluar tidak boleh sebelum tanggal mulai sewa.";
export const PLANNED_END_BEFORE_TODAY_MESSAGE =
  "Rencana keluar tidak boleh sebelum hari ini.";
export const END_BEFORE_START_MESSAGE =
  "Tanggal keluar tidak boleh sebelum tanggal mulai sewa.";
export const END_AFTER_TODAY_MESSAGE =
  "Tanggal keluar tidak boleh setelah hari ini.";

export const EFFECTS_TITLE = "Yang terjadi setelah ini";

// The API fields these forms show errors under; any other goes to the alert.
export const PLANNED_END_API_FIELDS: readonly string[] = ["plannedEndDate"];
export const MOVE_OUT_API_FIELDS: readonly string[] = ["endDate"];

// What a planned end does, per period of the lease's term.
export function plannedEndEffects(term: TermMonths): string[] {
  const { noun, short } = TERM_PERIOD_NAMES[term];
  return [
    `Tidak ada tagihan baru untuk ${noun} yang mulai sesudah tanggal itu.`,
    `${capitalized(noun)} yang sudah mulai tetap ditagih penuh, tanpa potongan.`,
    `Tagihan ${short} sesudahnya yang sudah terlanjur dibuat dibatalkan otomatis, dan tercatat sebagai koreksi dengan alasan "sewa tidak diperpanjang".`,
    "Tunggakan lama tetap ditagih seperti biasa.",
  ];
}

export function moveOutEffects(term: TermMonths): string[] {
  return [
    "Unit langsung kosong dan bisa disewakan lagi.",
    `Tagihan untuk ${TERM_PERIOD_NAMES[term].noun} yang mulai sesudah tanggal itu dibatalkan.`,
    "Rencana keluar yang sudah diisi ikut dihapus.",
    "Kalau masih ada tunggakan, aplikasi mengikuti aturan usaha di Pengaturan: potong uang jaminan dulu, atau tetap ditagih.",
  ];
}

// The suggested last day: the day before the next period of the term starts, so the tenant
// stops before a new one. For a booking that is the day before its second period.
export function plannedEndSuggestion(
  start: string,
  today: string,
  term: TermMonths,
): PlannedEndSuggestion {
  const nextStart =
    start > today
      ? periodStart(start, 1, term)
      : nextPeriodStart(start, today, term);
  return { date: addDays(nextStart, -1), nextStart };
}

export function plannedEndHint(
  suggestion: PlannedEndSuggestion,
  term: TermMonths,
): string {
  return `Usulan: ${formatDate(suggestion.date)}, sehari sebelum ${TERM_PERIOD_NAMES[term].noun} berikutnya (${formatDayMonth(suggestion.nextStart)}) dimulai. Tanggal harus hari ini atau sesudahnya, dan tidak sebelum tanggal mulai sewa.`;
}

export function currentPlannedEndText(date: string): string {
  return `Rencana keluar saat ini: ${formatDate(date)}. Menghapusnya mengembalikan tagihan yang tadi dibatalkan.`;
}

// The planned last day: a real date, not before the start date and not before today.
export function checkPlannedEnd(
  date: string,
  start: string,
  today: string,
): DateCheck {
  if (!isCalendarDate(date)) return { ok: false, error: NO_DATE_MESSAGE };
  if (date < start) {
    return { ok: false, error: PLANNED_END_BEFORE_START_MESSAGE };
  }
  if (date < today) {
    return { ok: false, error: PLANNED_END_BEFORE_TODAY_MESSAGE };
  }
  return { ok: true, date };
}

export function moveOutHint(start: string): string {
  return `Diisi hari ini. Tidak boleh sebelum tanggal mulai sewa (${formatDate(start)}) dan tidak boleh setelah hari ini.`;
}

// The last day of a lease that has started: a real date, not before the start, not after today.
export function checkMoveOut(
  date: string,
  start: string,
  today: string,
): DateCheck {
  if (!isCalendarDate(date)) return { ok: false, error: NO_DATE_MESSAGE };
  if (date < start) return { ok: false, error: END_BEFORE_START_MESSAGE };
  if (date > today) return { ok: false, error: END_AFTER_TODAY_MESSAGE };
  return { ok: true, date };
}

// "Batalkan sewa" for a booking (boards Sewa-Batal.dc.html and Sewa-Batal-Phone.dc.html).
export function cancelBookingText(start: string): string {
  return `Sewa ini belum dimulai (mulai ${formatDate(start)}). Kalau dibatalkan, unit langsung kosong lagi dan sewa ini tidak akan pernah ditagih.`;
}

export const CANCEL_BOOKING_HINT =
  "Diisi otomatis sama dengan tanggal mulai. Sewa yang belum dimulai hanya bisa dibatalkan dengan tanggal itu.";

// "Biaya tambahan" (boards Sewa-Biaya.dc.html and Sewa-Biaya-Phone.dc.html): the lease's fixed
// extras, set as a whole. The API checks the same rules and its message wins.

export const LEASE_CHARGES_INTRO =
  "Biaya yang jumlahnya sama setiap bulan. Ditambahkan ke tagihan di atas sewa, mulai tagihan berikutnya. Hanya angka.";
// For a lease billed every 3, 6 or 12 months: the fixed charges are monthly amounts, so each
// invoice bills them times the term's months (CLAUDE.md "Billing and reminders"). None for a
// monthly lease.
export function chargesTermHint(term: TermMonths): string | null {
  if (term === 1) return null;
  return `Biaya tetap ditagih per bulan × ${term} bulan di setiap tagihan.`;
}

export const METERED_CHARGES_NOTE =
  "Listrik dan air yang dicatat dari meteran tidak diatur di sini. Jumlahnya dicatat tiap bulan.";
export const LOADING_CHARGE_TYPES_TEXT = "Memuat…";
export const NO_FIXED_CHARGE_TYPES_TEXT = "Belum ada jenis biaya tetap.";
export const BAD_CHARGE_AMOUNT_MESSAGE =
  "Isi jumlahnya dengan angka, paling sedikit Rp1.";

export const ADD_CHARGE_TYPE_LABEL = "Tambah jenis biaya baru";
export const NEW_CHARGE_TYPE_TITLE = "Jenis biaya baru";
export const CHARGE_TYPE_NAME_LABEL = "Nama biaya";
export const CHARGE_TYPE_NAME_PLACEHOLDER = "Contoh: Air galon";
export const CHARGE_TYPE_METERED_LABEL =
  "Jumlahnya berbeda tiap bulan (dicatat dari meteran)";
export const CREATE_CHARGE_TYPE_LABEL = "Tambah jenis biaya";
export const NEW_CHARGE_TYPE_HINT =
  "Jenis biaya baru dipakai untuk semua properti.";

// The API's limit on a charge type's name (ChargeTypeInput), counted in characters.
export const CHARGE_TYPE_NAME_MAX_LENGTH = 50;
export const EMPTY_CHARGE_TYPE_NAME_MESSAGE = "Isi nama biaya.";
export const LONG_CHARGE_TYPE_NAME_MESSAGE = `Nama biaya paling panjang ${CHARGE_TYPE_NAME_MAX_LENGTH} huruf.`;

// The API field "Jenis biaya baru" shows its errors under; any other goes to the box's alert.
export const CHARGE_TYPE_API_FIELDS: readonly string[] = ["name"];

const NO_ENTRY: ChargeEntry = { checked: false, amount: "" };

// A row's amount input id, under the form's own id, so the form can focus it.
export function chargeAmountId(formId: string, chargeTypeId: string): string {
  return `${formId}-${chargeTypeId}-amount`;
}

// The amount field's accessible name in a row: "Jumlah Wi-Fi".
export function chargeAmountLabel(name: string): string {
  return `Jumlah ${name}`;
}

// After a metered type is added: it has no row here.
export function meteredChargeTypeAddedText(name: string): string {
  return `Jenis biaya ${name} ditambahkan. Biaya dari meteran dicatat tiap bulan, jadi tidak diatur di sini.`;
}

// The form's starting entries: the lease's charges, checked, with their amounts.
export function chargeEntriesFromLease(
  charges: readonly LeaseCharge[],
): ChargeEntries {
  const entries: Record<string, ChargeEntry> = {};
  for (const charge of charges) {
    entries[charge.chargeTypeId] = {
      checked: true,
      amount: groupRupiahDigits(String(charge.amount)),
    };
  }
  return entries;
}

// One row per non-metered charge type, in the API's order (by name). Metered types are recorded
// every month instead, so they are never fixed charges.
export function chargeRows(
  types: readonly ChargeType[],
  entries: ChargeEntries,
): ChargeRow[] {
  return types
    .filter((type) => !type.metered)
    .map((type) => ({
      chargeTypeId: type.id,
      name: type.name,
      ...(entries[type.id] ?? NO_ENTRY),
    }));
}

// A row's amount typed: typing into an unchecked row checks it. Emptying the field leaves the
// checkbox as it is.
export function withChargeAmount(
  entries: ChargeEntries,
  chargeTypeId: string,
  amount: string,
): ChargeEntries {
  const entry = entries[chargeTypeId] ?? NO_ENTRY;
  return {
    ...entries,
    [chargeTypeId]: { checked: entry.checked || amount !== "", amount },
  };
}

// A row checked or unchecked. Unchecking keeps the typed text; the row is just not sent.
export function withChargeChecked(
  entries: ChargeEntries,
  chargeTypeId: string,
  checked: boolean,
): ChargeEntries {
  const entry = entries[chargeTypeId] ?? NO_ENTRY;
  return { ...entries, [chargeTypeId]: { ...entry, checked } };
}

// What "Simpan biaya tambahan" checks and sends: the checked rows in display order, each with a
// whole amount of at least 1 sent as an integer. No checked row sends [], which removes every
// extra.
export function checkLeaseCharges(
  rows: readonly ChargeRow[],
): LeaseChargesResult {
  const errors: ChargeErrors = {};
  let first: string | null = null;
  const charges: { chargeTypeId: string; amount: number }[] = [];
  for (const row of rows) {
    if (!row.checked) continue;
    const amount = parseRupiahInput(row.amount);
    if (amount === null || amount < 1) {
      errors[row.chargeTypeId] = BAD_CHARGE_AMOUNT_MESSAGE;
      first ??= row.chargeTypeId;
      continue;
    }
    charges.push({ chargeTypeId: row.chargeTypeId, amount });
  }
  if (first !== null) return { ok: false, errors, first };
  return {
    ok: true,
    input: { charges },
    sent: charges.map((charge) => charge.chargeTypeId),
  };
}

const CHARGE_API_FIELD = /^charges\[(\d+)\]\.(amount|chargeTypeId)$/;

// The API fields that can name a row of a request: charges[i].amount and charges[i].chargeTypeId
// for every index sent. Any other field (such as "charges") goes to the alert.
export function chargeApiFields(sent: readonly string[]): string[] {
  return sent.flatMap((_, i) => [
    `charges[${i}].amount`,
    `charges[${i}].chargeTypeId`,
  ]);
}

// The charge type id of the row an API field names, from the rows in the order they were sent;
// null for a field that names no row sent.
export function chargeRowForApiField(
  field: string | null,
  sent: readonly string[],
): string | null {
  if (field === null) return null;
  const match = CHARGE_API_FIELD.exec(field);
  if (match === null) return null;
  return sent[Number(match[1])] ?? null;
}

// "Nama biaya": not empty and at most CHARGE_TYPE_NAME_MAX_LENGTH characters once trimmed.
export function checkChargeTypeName(name: string): ChargeTypeNameCheck {
  const trimmed = name.trim();
  if (trimmed === "")
    return { ok: false, error: EMPTY_CHARGE_TYPE_NAME_MESSAGE };
  if ([...trimmed].length > CHARGE_TYPE_NAME_MAX_LENGTH) {
    return { ok: false, error: LONG_CHARGE_TYPE_NAME_MESSAGE };
  }
  return { ok: true, name: trimmed };
}
