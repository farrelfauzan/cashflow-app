import { describe, expect, it, vi } from "vitest";

import type { Capability, TenantOpenInvoice } from "@/lib/api/generated/model";
import { createAppAbility } from "@/lib/capabilities";

import {
  canSharePayLink,
  copyPendingText,
  copyToClipboard,
  newestOpenInvoice,
  payLinkInvoice,
} from "./utils";

function invoice(overrides: Partial<TenantOpenInvoice>): TenantOpenInvoice {
  return {
    id: "inv",
    kind: "regular",
    unitLabel: "Kamar 3",
    propertyName: "Kos Melati",
    period: "2026-10-01",
    periodEnd: "2026-10-31",
    dueDate: "2026-10-01",
    amount: 1_100_000,
    balance: 1_100_000,
    ...overrides,
  };
}

describe("newestOpenInvoice", () => {
  it("picks the latest period that still owes something", () => {
    const invoices = [
      invoice({ id: "sep", period: "2026-09-01" }),
      invoice({ id: "oct", period: "2026-10-01", balance: 300_000 }),
      invoice({ id: "nov", period: "2026-11-01" }),
    ];
    expect(newestOpenInvoice(invoices)?.id).toBe("nov");
  });

  it("skips an invoice with nothing left, even a newer one", () => {
    // The API lists only invoices with a balance above 0; the check stays in case it ever
    // sends one.
    const invoices = [
      invoice({ id: "sep", period: "2026-09-01" }),
      invoice({ id: "nov", period: "2026-11-01", balance: 0 }),
    ];
    expect(newestOpenInvoice(invoices)?.id).toBe("sep");
  });

  it("does not rely on the API's order", () => {
    const invoices = [
      invoice({ id: "nov", period: "2026-11-01" }),
      invoice({ id: "sep", period: "2026-09-01" }),
    ];
    expect(newestOpenInvoice(invoices)?.id).toBe("nov");
  });

  it("takes a final invoice over that month's regular one", () => {
    const invoices = [
      invoice({ id: "oct", period: "2026-10-01" }),
      invoice({ id: "final", kind: "final", period: "2026-10-20" }),
    ];
    expect(newestOpenInvoice(invoices)?.id).toBe("final");
  });

  it("keeps the later one in the API's order when two leases share a period", () => {
    const invoices = [
      invoice({ id: "kamar-1", period: "2026-11-01" }),
      invoice({ id: "kamar-2", period: "2026-11-01" }),
    ];
    expect(newestOpenInvoice(invoices)?.id).toBe("kamar-2");
  });

  it("is null when nothing is owed", () => {
    expect(newestOpenInvoice([])).toBeNull();
  });
});

function ability(...capabilities: Capability[]) {
  return createAppAbility(capabilities);
}

describe("canSharePayLink", () => {
  const read = (access: Capability["access"]): Capability => ({
    action: "invoice:read",
    access,
  });
  const share = (access: Capability["access"]): Capability => ({
    action: "invoice:share",
    access,
  });

  it("shows for an admin who may read and share every invoice", () => {
    expect(canSharePayLink(ability(read("allow"), share("allow")))).toBe(true);
  });

  it("shows for a karyawan who may read and share only the tenants they collect", () => {
    expect(
      canSharePayLink(ability(read("conditional"), share("conditional"))),
    ).toBe(true);
  });

  it("hides without invoice:share", () => {
    expect(canSharePayLink(ability(read("allow"), share("deny")))).toBe(false);
    expect(canSharePayLink(ability(read("allow")))).toBe(false);
  });

  it("hides without invoice:read, which finding the open invoices needs", () => {
    expect(canSharePayLink(ability(read("deny"), share("allow")))).toBe(false);
    expect(canSharePayLink(ability(share("allow")))).toBe(false);
  });
});

describe("payLinkInvoice", () => {
  const open = [
    invoice({ id: "oct", period: "2026-10-01" }),
    invoice({ id: "nov", period: "2026-11-01" }),
  ];

  it("is the newest open invoice for a member who may share", () => {
    expect(payLinkInvoice(true, open)?.id).toBe("nov");
  });

  it("hides the button without permission to share", () => {
    expect(payLinkInvoice(false, open)).toBeNull();
  });

  it("hides the button until the open invoices load, or when the read fails or is refused", () => {
    expect(payLinkInvoice(true, undefined)).toBeNull();
  });

  it("hides the button when nothing is owed or the member may see none (an empty list)", () => {
    expect(payLinkInvoice(true, [])).toBeNull();
  });
});

describe("copyToClipboard", () => {
  it("copies the text and reports success", async () => {
    const writeText = vi.fn<(text: string) => Promise<void>>(() =>
      Promise.resolve(),
    );
    await expect(
      copyToClipboard({ writeText }, "https://x/t/abc"),
    ).resolves.toBe(true);
    expect(writeText).toHaveBeenCalledWith("https://x/t/abc");
  });

  it("reports failure when there is no clipboard", async () => {
    await expect(copyToClipboard(undefined, "link")).resolves.toBe(false);
  });

  it("reports failure when the clipboard refuses", async () => {
    const writeText = () => Promise.reject(new Error("NotAllowedError"));
    await expect(copyToClipboard({ writeText }, "link")).resolves.toBe(false);
  });

  it("reports failure when the clipboard throws at once", async () => {
    const writeText = (): Promise<void> => {
      throw new Error("not a secure context");
    };
    await expect(copyToClipboard({ writeText }, "link")).resolves.toBe(false);
  });
});

describe("copyPendingText", () => {
  // A stand-in ClipboardItem that keeps the pending text, so a test can check what was written.
  const item = (text: Promise<string>) =>
    ({ text }) as unknown as ClipboardItem;

  it("starts the write at once with the pending link, before it arrives", async () => {
    let resolve: (value: string) => void = () => undefined;
    const link = new Promise<string>((r) => {
      resolve = r;
    });
    const written: unknown[] = [];
    const write = vi.fn((items: ClipboardItem[]) => {
      written.push(items[0]);
      return Promise.resolve();
    });
    const writeText = vi.fn(() => Promise.resolve());
    const copied = copyPendingText({ write, writeText }, item, link);
    expect(write).toHaveBeenCalledTimes(1);
    resolve("https://x/t/abc");
    await expect(copied).resolves.toBe(true);
    await expect((written[0] as { text: Promise<string> }).text).resolves.toBe(
      "https://x/t/abc",
    );
    expect(writeText).not.toHaveBeenCalled();
  });

  it("falls back to writeText when write refuses", async () => {
    const write = () => Promise.reject(new Error("NotAllowedError"));
    const writeText = vi.fn(() => Promise.resolve());
    await expect(
      copyPendingText({ write, writeText }, item, Promise.resolve("link")),
    ).resolves.toBe(true);
    expect(writeText).toHaveBeenCalledWith("link");
  });

  it("falls back to writeText when write throws at once", async () => {
    const write = (): Promise<void> => {
      throw new Error("no ClipboardItem support");
    };
    const writeText = vi.fn(() => Promise.resolve());
    await expect(
      copyPendingText({ write, writeText }, item, Promise.resolve("link")),
    ).resolves.toBe(true);
  });

  it("uses writeText where there is no ClipboardItem", async () => {
    const writeText = vi.fn(() => Promise.resolve());
    await expect(
      copyPendingText({ writeText }, undefined, Promise.resolve("link")),
    ).resolves.toBe(true);
    expect(writeText).toHaveBeenCalledWith("link");
  });

  it("reports false, without rejecting, when both ways refuse", async () => {
    const write = () => Promise.reject(new Error("NotAllowedError"));
    const writeText = () => Promise.reject(new Error("NotAllowedError"));
    await expect(
      copyPendingText({ write, writeText }, item, Promise.resolve("link")),
    ).resolves.toBe(false);
  });

  it("reports false when the link itself fails", async () => {
    const write = (items: ClipboardItem[]) =>
      (items[0] as unknown as { text: Promise<string> }).text.then(
        () => undefined,
      );
    const writeText = vi.fn(() => Promise.resolve());
    const failed = Promise.reject(new Error("409"));
    await expect(
      copyPendingText({ write, writeText }, item, failed),
    ).resolves.toBe(false);
    expect(writeText).not.toHaveBeenCalled();
  });
});
