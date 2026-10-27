// Rupiah display and input parsing. Money is integer rupiah from the API; this module only
// formats it for display and turns typed digits back into an integer. No other money math.

const THOUSANDS = /\B(?=(\d{3})+(?!\d))/g;

/**
 * Formats integer rupiah as `Rp1.250.000`: dot thousands separator, no decimals, no space
 * after `Rp`, and `-Rp1.250.000` for negative amounts.
 *
 * Grouping is done by hand instead of with `Intl` currency style, which inserts a
 * (non-breaking) space after `Rp` and depends on the runtime's ICU data.
 *
 * @throws RangeError when `amount` is not a safe integer.
 */
export function formatRupiah(amount: number): string {
  if (!Number.isSafeInteger(amount)) {
    throw new RangeError(`formatRupiah expects a safe integer, got ${amount}`);
  }
  const grouped = String(Math.abs(amount)).replace(THOUSANDS, ".");
  return amount < 0 ? `-Rp${grouped}` : `Rp${grouped}`;
}

// The same rule as the API's CSV import (internal/tenantimport/parse.go): an explicit "no cents"
// tail is dropped, and what remains is plain digits or digits with a dot between every group of
// three.
const RP_PREFIX = /^rp/i;
const ZERO_CENTS = /[.,]00?$/;
const WHOLE_RUPIAH = /^(\d+|\d{1,3}(\.\d{3})+)$/;

/**
 * Turns what a user typed into an amount field into integer rupiah: `"1.250.000"`,
 * `"Rp 1.250.000"`, `"1250000"` and `"1.250.000,00"` all give `1250000`.
 *
 * A dot is only a thousands separator. Anything else is refused rather than guessed, so the
 * field can show an error: `null` for empty input, letters (`"1.5jt"`), a fraction other than
 * zero cents (`"850.5"`, `"850000.50"`), a misplaced dot, a sign, or a value beyond
 * `Number.MAX_SAFE_INTEGER`.
 */
export function parseRupiahInput(input: string): number | null {
  const compact = input.trim().replace(RP_PREFIX, "").replace(/\s/g, "");
  const whole = compact.replace(ZERO_CENTS, "");
  if (!WHOLE_RUPIAH.test(whole)) {
    return null;
  }
  const amount = Number(whole.replaceAll(".", ""));
  return Number.isSafeInteger(amount) ? amount : null;
}
