import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  StorageUploadError,
  isStorageUploadError,
  putToStorage,
  type StorageUploadProgress,
} from "./storage-upload";

type ProgressLike = {
  loaded: number;
  total: number;
  lengthComputable: boolean;
};

// Stands in for the browser's XMLHttpRequest: records what the helper set and sent, and lets a
// test play the browser's events. No test touches the network.
class FakeXhr {
  static last: FakeXhr | null = null;
  static created = 0;

  method = "";
  url = "";
  withCredentials: boolean | undefined;
  headers: Record<string, string> = {};
  body: unknown = undefined;
  sent = false;
  aborted = false;
  status = 0;
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  ontimeout: (() => void) | null = null;
  onabort: (() => void) | null = null;
  upload: {
    onprogress: ((event: ProgressLike) => void) | null;
    onload: (() => void) | null;
  } = { onprogress: null, onload: null };

  constructor() {
    FakeXhr.last = this;
    FakeXhr.created += 1;
  }

  open(method: string, url: string) {
    this.method = method;
    this.url = url;
  }

  setRequestHeader(name: string, value: string) {
    this.headers[name] = value;
  }

  send(body: unknown) {
    this.sent = true;
    this.body = body;
  }

  abort() {
    this.aborted = true;
    this.onabort?.();
  }

  // Browser events.
  progress(loaded: number, total: number, lengthComputable = true) {
    this.upload.onprogress?.({ loaded, total, lengthComputable });
  }

  answer(status: number) {
    this.status = status;
    this.onload?.();
  }

  fail() {
    this.onerror?.();
  }
}

function xhr(): FakeXhr {
  const current = FakeXhr.last;
  if (current === null) throw new Error("no XMLHttpRequest was created");
  return current;
}

async function rejection(
  promise: Promise<unknown>,
): Promise<StorageUploadError> {
  try {
    await promise;
  } catch (error) {
    if (isStorageUploadError(error)) return error;
    throw error;
  }
  throw new Error("expected the PUT to fail");
}

const PUT_URL =
  "https://storage.example.test/receipts/abc?X-Amz-Signature=secret-signature";
const TICKET = {
  putUrl: PUT_URL,
  putHeaders: { "Content-Type": "image/jpeg" },
};
const BODY = new Blob([new Uint8Array(1234)], { type: "image/jpeg" });

beforeEach(() => {
  FakeXhr.last = null;
  FakeXhr.created = 0;
  vi.stubGlobal("XMLHttpRequest", FakeXhr);
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("putToStorage", () => {
  it("PUTs the body to putUrl with exactly putHeaders and no credentials", async () => {
    const done = putToStorage(
      {
        putUrl: PUT_URL,
        putHeaders: {
          "Content-Type": "image/jpeg",
          "X-Amz-Meta-Purpose": "cash_receipt",
        },
      },
      BODY,
    );
    const request = xhr();
    expect(request.method).toBe("PUT");
    expect(request.url).toBe(PUT_URL);
    expect(request.withCredentials).toBe(false);
    expect(request.headers).toEqual({
      "Content-Type": "image/jpeg",
      "X-Amz-Meta-Purpose": "cash_receipt",
    });
    expect(request.body).toBe(BODY);

    request.answer(200);
    await expect(done).resolves.toBeUndefined();
  });

  it("sends no header of the API client (no Accept, no cookie, no XSRF)", () => {
    void putToStorage(TICKET, BODY);
    const names = Object.keys(xhr().headers).map((name) => name.toLowerCase());
    expect(names).toEqual(["content-type"]);
  });

  it("reports progress, with no total while the size is unknown", async () => {
    const seen: StorageUploadProgress[] = [];
    const done = putToStorage(TICKET, BODY, {
      onProgress: (progress) => seen.push(progress),
    });
    const request = xhr();
    request.progress(0, 0, false);
    request.progress(600, 1234);
    request.progress(1234, 1234);
    request.answer(204);
    await done;
    expect(seen).toEqual([
      { loaded: 0, total: undefined },
      { loaded: 600, total: 1234 },
      { loaded: 1234, total: 1234 },
    ]);
  });

  it("rejects as refused, with the status, when storage answers outside 2xx", async () => {
    const done = putToStorage(TICKET, BODY);
    xhr().answer(403);
    const error = await rejection(done);
    expect(error.kind).toBe("refused");
    expect(error.status).toBe(403);
  });

  it("rejects as network when the connection fails", async () => {
    const done = putToStorage(TICKET, BODY);
    xhr().fail();
    const error = await rejection(done);
    expect(error.kind).toBe("network");
    expect(error.status).toBe(0);
  });

  it("never puts the signed URL in its error", async () => {
    const done = putToStorage(TICKET, BODY);
    xhr().answer(400);
    const error = await rejection(done);
    expect(error.message).not.toContain("storage.example.test");
    expect(error.message).not.toContain("secret-signature");
  });

  it("aborts the PUT when the signal is aborted", async () => {
    const controller = new AbortController();
    const done = putToStorage(TICKET, BODY, { signal: controller.signal });
    const request = xhr();
    request.progress(100, 1234);
    controller.abort();
    expect(request.aborted).toBe(true);
    const error = await rejection(done);
    expect(error.kind).toBe("aborted");
  });

  it("sends nothing when the signal is already aborted", async () => {
    const controller = new AbortController();
    controller.abort();
    const error = await rejection(
      putToStorage(TICKET, BODY, { signal: controller.signal }),
    );
    expect(error.kind).toBe("aborted");
    expect(FakeXhr.created).toBe(0);
  });

  it("settles once: an answer after an abort changes nothing", async () => {
    const controller = new AbortController();
    const done = putToStorage(TICKET, BODY, { signal: controller.signal });
    const request = xhr();
    controller.abort();
    request.answer(200);
    expect((await rejection(done)).kind).toBe("aborted");
  });

  it("ends a PUT with no progress for the stall timeout as a network failure", async () => {
    vi.useFakeTimers();
    const done = putToStorage(TICKET, BODY, { stallTimeoutMs: 1000 });
    const request = xhr();
    vi.advanceTimersByTime(900);
    request.progress(500, 1234);
    // Progress restarted the timer: still running 900ms later.
    vi.advanceTimersByTime(900);
    expect(request.aborted).toBe(false);
    vi.advanceTimersByTime(200);
    expect(request.aborted).toBe(true);
    const error = await rejection(done);
    expect(error.kind).toBe("network");
  });

  it("stops the stall timer once storage has answered", async () => {
    vi.useFakeTimers();
    const done = putToStorage(TICKET, BODY, { stallTimeoutMs: 1000 });
    const request = xhr();
    request.answer(200);
    await done;
    vi.advanceTimersByTime(5000);
    expect(request.aborted).toBe(false);
  });
});
