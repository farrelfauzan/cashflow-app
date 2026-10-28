// Phone display. The API stores and answers numbers in E.164 (+6281200090001); people read and
// type them the Indonesian way (0812-0009-0001).

const INDONESIA = "+62";

/**
 * Formats an Indonesian E.164 number as `0812-0009-0001`: a leading 0 instead of +62, then
 * groups of four digits with the remainder last. Any other value is returned unchanged, so a
 * number the API sends in another form is still shown, never hidden.
 */
export function formatPhone(e164: string): string {
  if (!e164.startsWith(INDONESIA) || !/^\+\d+$/.test(e164)) return e164;
  const national = `0${e164.slice(INDONESIA.length)}`;
  const groups = national.match(/.{1,4}/g) ?? [national];
  return groups.join("-");
}
