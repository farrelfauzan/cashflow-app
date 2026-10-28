// The two letters on an organization's tile: the first letters of its first two words
// ("Griya Sejahtera" → "GS"), or the first two letters of a one-word name.
export function orgInitials(name: string): string {
  const words = name
    .trim()
    .split(/\s+/)
    .filter((word) => word !== "");
  if (words.length === 0) return "";
  const letters =
    words.length === 1
      ? words[0]!.slice(0, 2)
      : `${words[0]![0]}${words[1]![0]}`;
  return letters.toLocaleUpperCase("id-ID");
}
