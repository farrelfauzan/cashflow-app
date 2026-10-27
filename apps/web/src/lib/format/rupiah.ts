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

/**
 * Turns what a user typed into an amount field into integer rupiah, keeping digits only:
 * `"1.250.000"` and `"Rp 1.250.000"` give `1250000`.
 *
 * Returns `null` when there are no digits, when the input contains a comma (the Indonesian
 * decimal separator, so `"1.250.000,00"` is not read as a hundred times the amount), or when
 * the value is beyond `Number.MAX_SAFE_INTEGER`.
 */
export function parseRupiahInput(input: string): number | null {
  if (input.includes(",")) {
    return null;
  }
  const digits = input.replace(/\D/g, "");
  if (digits === "") {
    return null;
  }
  const amount = Number(digits);
  return Number.isSafeInteger(amount) ? amount : null;
}
