// API_INTERNAL_URL: the Go API's origin as the Next.js server reaches it (http://localhost:8080
// locally, http://api:8080 inside Compose). Server components call `${origin}/api/...` there.
// Checked once at startup by src/instrumentation.ts, so a bad value stops the server.

export const API_INTERNAL_URL_ENV = "API_INTERNAL_URL";

// Returns the origin (no trailing slash) or throws an Error that names the variable.
export function parseApiInternalUrl(value: string | undefined): string {
  if (value === undefined || value.trim() === "") {
    throw new Error(
      `${API_INTERNAL_URL_ENV} is not set. Set it to the Go API's origin, for example http://localhost:8080.`,
    );
  }

  let url: URL;
  try {
    url = new URL(value.trim());
  } catch {
    throw new Error(
      `${API_INTERNAL_URL_ENV} is not a valid URL. Use the Go API's origin, for example http://localhost:8080.`,
    );
  }

  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error(`${API_INTERNAL_URL_ENV} must use http or https.`);
  }
  if (url.username !== "" || url.password !== "") {
    throw new Error(`${API_INTERNAL_URL_ENV} must not contain credentials.`);
  }
  // Only an origin: the /api prefix comes from the contract and is added per request.
  if (url.pathname !== "/" || url.search !== "" || url.hash !== "") {
    throw new Error(
      `${API_INTERNAL_URL_ENV} must be an origin only (scheme, host and port), without a path, query or fragment.`,
    );
  }
  return url.origin;
}

export function apiInternalUrl(): string {
  return parseApiInternalUrl(process.env[API_INTERNAL_URL_ENV]);
}
