// The `?next=` return path of the login page. It is only ever produced from, and accepted as, a
// same-origin relative path, so a crafted link cannot send a user to another site after login.

export const LOGIN_PATH = "/login";

const MAX_NEXT_PATH_LENGTH = 2048;
// A fixed origin to resolve against: anything that resolves elsewhere is not a local path.
const PLACEHOLDER_ORIGIN = "http://next-path.invalid";
// C0 controls and DEL. Browsers drop tab and newline from URLs, so "/\t/evil.com" would
// become "//evil.com".
const CONTROL_CHARACTERS = /[\u0000-\u001f\u007f]/;

// Returns the path (with query and hash) when it is a safe same-origin relative path, or null.
export function safeNextPath(value: string | null | undefined): string | null {
  if (typeof value !== "string" || value === "") return null;
  if (value.length > MAX_NEXT_PATH_LENGTH) return null;
  // One leading slash: "//host" is protocol-relative, "https:" and "javascript:" have none.
  if (!value.startsWith("/") || value.startsWith("//")) return null;
  // Browsers treat "\" as "/" in URLs, so "/\evil.com" is "//evil.com".
  if (value.includes("\\")) return null;
  if (CONTROL_CHARACTERS.test(value)) return null;

  let url: URL;
  try {
    url = new URL(value, PLACEHOLDER_ORIGIN);
  } catch {
    return null;
  }
  if (url.origin !== PLACEHOLDER_ORIGIN) return null;

  // Dot segments can collapse into a protocol-relative path ("/..//evil.com" is "//evil.com"),
  // so check the normalized result again.
  const path = `${url.pathname}${url.search}${url.hash}`;
  if (path.startsWith("//")) return null;
  return path;
}

export function isLoginPath(path: string): boolean {
  return (
    path === LOGIN_PATH ||
    path.startsWith(`${LOGIN_PATH}?`) ||
    path.startsWith(`${LOGIN_PATH}/`) ||
    path.startsWith(`${LOGIN_PATH}#`)
  );
}

// "/login?next=<path>" for a safe path, plain "/login" otherwise.
export function loginPathWithNext(next: string | null | undefined): string {
  const path = safeNextPath(next);
  if (path === null || isLoginPath(path)) return LOGIN_PATH;
  return `${LOGIN_PATH}?${new URLSearchParams({ next: path }).toString()}`;
}
