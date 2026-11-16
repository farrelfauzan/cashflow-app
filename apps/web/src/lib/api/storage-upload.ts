// The browser's PUT of a photo to file storage, the one request of the app that does not go to
// our API. createUpload answers a presigned URL on another origin (RustFS in development and
// tests, R2 in production) and the exact headers the PUT must carry; this sends the file there.
//
// Why it does not go through the API client (client.ts) and its axios instance:
// - Nothing of the mutator applies to storage. Its request interceptor points every call at
//   /api and forwards the session cookie; its response interceptor parses the API's JSON error
//   body and sends a 401 to the login page. Storage answers XML, its 403 means an expired or
//   mismatched signature (not a lost session), and it must never see a cookie or another API
//   header. The instance also refuses absolute URLs (allowAbsoluteUrls: false), on purpose.
// - A second axios instance would break the one-instance rule (decision 26), and axios's own
//   defaults add an Accept header and XSRF handling to every request, while the PUT must carry
//   exactly putHeaders, which are signed into the URL.
// - fetch cannot report upload progress in every browser the app supports (a streamed request
//   body is not in Safari), and the karyawan sees a percent on a weak signal.
// So it is XMLHttpRequest, the browser primitive axios's own xhr adapter uses for
// onUploadProgress, with credentials off, exactly putHeaders, progress, an abort signal and a
// stall timer. It never logs, and no error it raises carries the URL (a signed secret until it
// expires) or the response body.

import type { UploadTicket } from "./generated/model";

// No progress for this long (the signal dropped without the connection failing) ends the PUT as a
// network failure, so the screen offers "Coba lagi" instead of a bar that never moves.
export const STORAGE_STALL_TIMEOUT_MS = 30_000;

export type StorageUploadProgress = {
  loaded: number;
  // Undefined while the browser does not know the body's size.
  total: number | undefined;
};

export type PutToStorageOptions = {
  onProgress?: (progress: StorageUploadProgress) => void;
  // Aborting it stops the PUT; the promise then rejects with kind "aborted".
  signal?: AbortSignal;
  stallTimeoutMs?: number;
};

// network: no answer (offline, connection dropped, stalled, CORS refused); refused: storage
// answered with a status outside 2xx (an expired URL, a body of another size); aborted: the
// caller's signal.
export type StorageUploadFailure = "network" | "refused" | "aborted";

export class StorageUploadError extends Error {
  readonly kind: StorageUploadFailure;
  // Storage's HTTP status for "refused", otherwise 0.
  readonly status: number;

  constructor(kind: StorageUploadFailure, status = 0) {
    super(`storage upload failed: ${kind}${status > 0 ? ` ${status}` : ""}`);
    this.name = "StorageUploadError";
    this.kind = kind;
    this.status = status;
  }
}

export function isStorageUploadError(
  error: unknown,
): error is StorageUploadError {
  return error instanceof StorageUploadError;
}

// PUTs `body` to the ticket's putUrl with exactly its putHeaders. Resolves once storage answered
// 2xx; rejects with a StorageUploadError otherwise.
export function putToStorage(
  ticket: Pick<UploadTicket, "putUrl" | "putHeaders">,
  body: Blob,
  {
    onProgress,
    signal,
    stallTimeoutMs = STORAGE_STALL_TIMEOUT_MS,
  }: PutToStorageOptions = {},
): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    if (signal?.aborted === true) {
      reject(new StorageUploadError("aborted"));
      return;
    }

    const xhr = new XMLHttpRequest();
    let settled = false;
    let stallTimer: ReturnType<typeof setTimeout> | undefined;

    function finish(error: StorageUploadError | null) {
      if (settled) return;
      settled = true;
      clearTimeout(stallTimer);
      signal?.removeEventListener("abort", onAbort);
      if (error === null) resolve();
      else reject(error);
    }

    function onAbort() {
      finish(new StorageUploadError("aborted"));
      xhr.abort();
    }

    function onStall() {
      finish(new StorageUploadError("network"));
      xhr.abort();
    }

    function restartStallTimer() {
      clearTimeout(stallTimer);
      stallTimer = setTimeout(onStall, stallTimeoutMs);
    }

    xhr.open("PUT", ticket.putUrl);
    // Never a cookie or any other credential to the storage origin.
    xhr.withCredentials = false;
    for (const [name, value] of Object.entries(ticket.putHeaders)) {
      xhr.setRequestHeader(name, value);
    }

    xhr.upload.onprogress = (event) => {
      restartStallTimer();
      onProgress?.({
        loaded: event.loaded,
        total: event.lengthComputable ? event.total : undefined,
      });
    };
    // The body is sent; storage's answer is next.
    xhr.upload.onload = restartStallTimer;
    xhr.onload = () => {
      const { status } = xhr;
      finish(
        status >= 200 && status < 300
          ? null
          : new StorageUploadError("refused", status),
      );
    };
    // A CORS refusal looks the same as no connection to the page: status 0, no details.
    xhr.onerror = () => finish(new StorageUploadError("network"));
    xhr.ontimeout = () => finish(new StorageUploadError("network"));
    xhr.onabort = () => finish(new StorageUploadError("aborted"));

    signal?.addEventListener("abort", onAbort, { once: true });
    restartStallTimer();
    xhr.send(body);
  });
}
