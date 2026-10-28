// The API's session cookie (HttpOnly, set by POST /api/auth/otp/verify). The web app never reads
// its value in the browser; on the server it only forwards it to the API.
export const SESSION_COOKIE = "cf_session";

// Set by src/proxy.ts on staff routes: the requested path and query, so the server-side 401
// handler can send the user back here after login. Server components have no other way to know
// the current URL.
export const REQUEST_PATH_HEADER = "x-request-path";
