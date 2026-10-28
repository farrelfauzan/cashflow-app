// The API client: Orval's mutator for every generated function and hook, and the only place in
// the app that calls fetch. Never log request bodies, cookies or tokens here.

import { apiEnvironment } from "#api-environment";

import {
  ApiError,
  apiErrorFromResponse,
  networkError,
  unexpectedResponseError,
} from "./errors";

// The contract's server URL (servers[0].url in api/openapi.yaml). Generated paths are relative
// to it.
export const API_BASE_PATH = "/api";

function isAbortError(error: unknown): boolean {
  return (
    typeof error === "object" &&
    error !== null &&
    "name" in error &&
    (error as { name: unknown }).name === "AbortError"
  );
}

export async function apiFetch<T>(
  url: string,
  options: RequestInit = {},
): Promise<T> {
  const environment = await apiEnvironment.getRequestEnvironment();

  const headers = new Headers(options.headers);
  headers.set("Accept", "application/json");
  for (const [name, value] of Object.entries(environment.headers)) {
    headers.set(name, value);
  }

  let response: Response;
  let body: string;
  try {
    response = await fetch(`${environment.origin}${API_BASE_PATH}${url}`, {
      ...options,
      headers,
      credentials: environment.credentials,
      // API answers are per user and change with every write; TanStack Query does the caching.
      cache: "no-store",
    });
    body = await response.text();
  } catch (error) {
    // A cancelled query must stay a cancellation for TanStack Query.
    if (isAbortError(error)) throw error;
    throw networkError();
  }

  if (!response.ok) {
    const error = apiErrorFromResponse(response.status, body);
    if (response.status === 401) {
      await apiEnvironment.handleUnauthorized();
    }
    throw error;
  }

  // 204 and other empty bodies.
  if (body === "") return undefined as T;
  try {
    return JSON.parse(body) as T;
  } catch {
    // Non-JSON downloads (the CSV template) are links, not client calls.
    throw unexpectedResponseError(response.status);
  }
}

// Orval's error type for every generated hook: always ApiError, whatever error bodies the spec
// lists. Orval passes that body type in; it is deliberately unused.
// eslint-disable-next-line @typescript-eslint/no-unused-vars
export type ErrorType<ResponseBody> = ApiError;

// Request bodies are sent as declared in the contract.
export type BodyType<RequestBody> = RequestBody;
