// What an amount field shows while it is typed. The field takes digits only; the value it sends
// comes from parseRupiahInput (./rupiah). No money math: only digits and dots.

// Fifteen digits always fit in a safe integer (Number.MAX_SAFE_INTEGER has sixteen).
export const MAX_RUPIAH_DIGITS = 15;

/**
 * Keeps only the digits of what was typed or pasted (`"Rp 1.250.000"` gives `"1250000"`), drops
 * leading zeros, and cuts the rest at MAX_RUPIAH_DIGITS. Typing only zeros leaves the field empty.
 */
export function rupiahDigits(input: string): string {
  return input
    .replace(/\D/g, "")
    .replace(/^0+/, "")
    .slice(0, MAX_RUPIAH_DIGITS);
}

/** Puts a dot between every group of three digits: `"1250000"` gives `"1.250.000"`. */
export function groupRupiahDigits(digits: string): string {
  return digits.replace(/\B(?=(\d{3})+(?!\d))/g, ".");
}

/** The text of an amount field for a typed or pasted value: digits only, grouped. */
export function rupiahFieldText(input: string): string {
  return groupRupiahDigits(rupiahDigits(input));
}
