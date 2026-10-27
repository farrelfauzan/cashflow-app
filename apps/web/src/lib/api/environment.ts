// What differs between an API call from a server component and one from the browser. The mutator
// (client.ts) imports the right implementation through the `#api-environment` subpath import in
// package.json:
//
// - "react-server" (the server component graph): environment-server.ts. Calls the API at
//   API_INTERNAL_URL, forwards the cf_session cookie, redirects to /login on 401.
// - everything else (the browser, and client components rendered on the server):
//   environment-browser.ts. Calls the same-origin /api, clears the query cache and navigates to
//   /login on 401.
//
// The condition keeps next/headers, next/navigation's server redirect and `server-only` out of
// the client bundle entirely; a runtime `typeof window` check could not.

export type RequestEnvironment = {
  // Prefix for `/api/...`: "" in the browser (same origin), API_INTERNAL_URL on the server.
  origin: string;
  credentials: RequestCredentials;
  headers: Record<string, string>;
};

export type ApiEnvironment = {
  getRequestEnvironment: () => Promise<RequestEnvironment>;
  // Called once for a 401 before the ApiError is thrown. It may not return (a redirect).
  handleUnauthorized: () => Promise<void>;
};
