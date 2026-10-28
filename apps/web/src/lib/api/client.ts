// The API client: Orval's mutator for every generated function and hook, and the only place in
// the app that creates or calls axios. Never log request bodies, cookies or tokens here.

import axios, {
  AxiosError,
  type AxiosRequestConfig,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from "axios";

import { apiEnvironment } from "#api-environment";

import {
  ApiError,
  apiErrorFromResponse,
  invalidPathError,
  networkError,
  unexpectedResponseError,
} from "./errors";

// The contract's server URL (servers[0].url in api/openapi.yaml). Generated paths are relative
// to it.
export const API_BASE_PATH = "/api";

// The one axios instance. axios keeps no cache of its own, so API answers are never cached
// outside TanStack Query, on the server or in the browser.
export const apiAxios = axios.create({
  headers: { Accept: "application/json" },
  // The body arrives as raw text and is parsed in the response interceptor, so an error body
  // that is not the API's JSON (an HTML page from a proxy) can never reach a screen.
  responseType: "text",
  transformResponse: [],
  // Repeated query parameters as `a=1&a=2`, the way the Go API reads them.
  paramsSerializer: { indexes: null },
  // Server side (Node's http adapter): never send the session cookie through an HTTP_PROXY
  // from the environment, and never follow a redirect with it. The browser ignores both.
  proxy: false,
  maxRedirects: 0,
  // An absolute url from a caller never replaces the base URL, so the cookie only goes to the API.
  allowAbsoluteUrls: false,
});

// Request: where the call goes and the one credential it carries. In the browser that is the
// same-origin /api, with the HttpOnly cf_session cookie going along by itself (no token in
// JavaScript, no Authorization header); on the server, API_INTERNAL_URL with the forwarded
// cf_session cookie and nothing else from the incoming request.
async function prepareRequest(
  config: InternalAxiosRequestConfig,
): Promise<InternalAxiosRequestConfig> {
  const environment = await apiEnvironment.getRequestEnvironment();
  config.baseURL = `${environment.origin}${API_BASE_PATH}`;
  for (const [name, value] of Object.entries(environment.headers)) {
    config.headers.set(name, value);
  }
  return config;
}

// Response, 2xx: the parsed JSON body, or undefined for 204 and other empty bodies.
function parseResponse(response: AxiosResponse<string>): AxiosResponse {
  const body = response.data;
  if (body === "" || body === undefined || body === null) {
    return { ...response, data: undefined };
  }
  try {
    return { ...response, data: JSON.parse(body) as unknown };
  } catch {
    // Non-JSON downloads (the CSV template) are links, not client calls.
    throw unexpectedResponseError(response.status);
  }
}

// Response, anything else: every non-2xx becomes the one ApiError, and a 401 goes through the
// single 401 handler first. A cancelled query stays a cancellation for TanStack Query, and an
// error raised before the request was sent (the browser client during server rendering) passes
// through unchanged.
async function rejectResponse(error: unknown): Promise<never> {
  if (axios.isCancel(error) || !(error instanceof AxiosError)) throw error;

  const { response } = error;
  if (response === undefined) throw networkError();

  const body = typeof response.data === "string" ? response.data : "";
  const apiError = apiErrorFromResponse(response.status, body);
  if (response.status === 401) {
    await apiEnvironment.handleUnauthorized();
  }
  throw apiError;
}

// Generated paths interpolate path values unencoded, and those values often come from the page's
// URL (`/o/[orgId]`, `/t/[token]`), already decoded. A value like `../auth/me?x=` would move the
// call, with the session cookie, to another API path. URL parsing treats `%2e` as a dot and `\`
// as a slash, so those count too. Query parameters travel in `params`, never in `url`.
const PATH_ESCAPE = /(^|\/)(\.|%2e){1,2}(\/|$)|[?#\\]/i;

function isSafePath(url: string | undefined): boolean {
  return url === undefined || !PATH_ESCAPE.test(url);
}

apiAxios.interceptors.request.use(prepareRequest);
apiAxios.interceptors.response.use(parseResponse, rejectResponse);

// Orval's mutator: `config` is the generated request (url, method, params, data, headers,
// signal), `options` what a caller adds through the generated `request` option.
export async function apiRequest<T>(
  config: AxiosRequestConfig,
  options?: AxiosRequestConfig,
): Promise<T> {
  const request: AxiosRequestConfig = {
    ...config,
    ...options,
    // A caller's extra header never drops a generated one (Content-Type).
    headers: { ...config.headers, ...options?.headers },
  };
  if (!isSafePath(request.url)) throw invalidPathError();
  const response = await apiAxios.request<T>(request);
  return response.data;
}

// Orval's error type for every generated hook: always ApiError, whatever error bodies the spec
// lists. Orval passes that body type in; it is deliberately unused.
// eslint-disable-next-line @typescript-eslint/no-unused-vars
export type ErrorType<ResponseBody> = ApiError;

// Request bodies are sent as declared in the contract.
export type BodyType<RequestBody> = RequestBody;
