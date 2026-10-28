// The 6-digit login code the API sends over WhatsApp (CLAUDE.md "Staff login").
export const LOGIN_CODE_LENGTH = 6;

// What the code field keeps from typed or pasted text: digits only, at most 6. Pasting
// "123 456" or "123-456" from a message still gives "123456".
export function loginCodeDigits(value: string): string {
  return value.replace(/\D/g, "").slice(0, LOGIN_CODE_LENGTH);
}

// The friendly check before a verify request; the API decides whether the code is right.
export function isCompleteLoginCode(value: string): boolean {
  return /^\d{6}$/.test(value);
}
