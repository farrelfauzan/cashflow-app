// What an amount field shows while it is typed. The field takes digits only; the value it sends
// comes from parseRupiahInput (./rupiah). No money math: only digits and dots.

// Fifteen digits always fit in a safe integer (Number.MAX_SAFE_INTEGER has sixteen).
export const MAX_RUPIAH_DIGITS = 15;

/**
 * Keeps only the digits of what was typed or pasted (`"Rp 1.250.000"` gives `"1250000"`), drops
 * leading zeros, and cuts the rest at MAX_RUPIAH_DIGITS. Typing only zeros leaves one "0", so a
 * field that asks for 0 ("Isi 0 kalau tidak ada") can show it.
 */
export function rupiahDigits(input: string): string {
  return input
    .replace(/\D/g, "")
    .replace(/^0+(?=\d)/, "")
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

/**
 * How many digits of a typed value sit before the caret, counted as the field keeps them, so the
 * caret can go back after the same digits once the text is grouped again.
 */
export function rupiahDigitsBefore(input: string, caret: number): number {
  const digits = input.replace(/\D/g, "");
  // The leading zeros rupiahDigits drops from the whole value never count.
  const dropped = digits.length - digits.replace(/^0+(?=\d)/, "").length;
  const before = input.slice(0, caret).replace(/\D/g, "").length;
  return Math.min(Math.max(before - dropped, 0), MAX_RUPIAH_DIGITS);
}

/** The caret position right after the first `count` digits of a grouped text. */
export function caretAfterDigits(text: string, count: number): number {
  if (count <= 0) return 0;
  let seen = 0;
  for (let i = 0; i < text.length; i++) {
    const char = text.charAt(i);
    if (char >= "0" && char <= "9") {
      seen++;
      if (seen === count) return i + 1;
    }
  }
  return text.length;
}
