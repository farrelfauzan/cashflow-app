import { describe, expect, it, vi } from "vitest";

import type {
  AccountInvoice,
  Capability,
  TenantAccount,
} from "@/lib/api/generated/model";
import { createAppAbility } from "@/lib/capabilities";

import {
  canSharePayLink,
  copyPendingText,
  copyToClipboard,
  newestOpenInvoice,
  payLinkInvoice,
} from "./utils";

function invoice(overrides: Partial<AccountInvoice>): AccountInvoice {
  return {
    id: "inv",
    kind: "regular",
    leaseId: "lease-1",
    unitLabel: "Kamar 3",
    propertyName: "Kos Melati",
    period: "2026-10-01",
    periodEnd: "2026-10-31",
    dueDate: "2026-10-01",
    issuedOn: "2026-09-28",
    amount: 1_100_000,
    billed: 1_100_000,
    paid: 0,
    outstanding: 1_100_000,
    status: "open",
    lines: [],
    corrections: [],
    ...overrides,
  };
}

const paid = (period: string, id: string) =>
  invoice({ id, period, paid: 1_100_000, outstanding: 0, status: "paid" });

function account(invoices: AccountInvoice[]): TenantAccount {
  return {
    tenantId: "tenant-1",
    invoices,
    payments: [],
    overdue: 0,
    outstanding: 0,
    credit: 0,
  };
}

describe("newestOpenInvoice", () => {
  it("picks the latest period that still owes something", () => {
    const invoices = [
      invoice({ id: "sep", period: "2026-09-01" }),
      invoice({ id: "oct", period: "2026-10-01", status: "partial" }),
      invoice({ id: "nov", period: "2026-11-01" }),
    ];
    expect(newestOpenInvoice(invoices)?.id).toBe("nov");
  });

  it("skips paid and cancelled invoices, even newer ones", () => {
    const invoices = [
      invoice({ id: "sep", period: "2026-09-01" }),
      paid("2026-10-01", "oct"),
      invoice({
        id: "nov",
        period: "2026-11-01",
        billed: 0,
        outstanding: 0,
        status: "cancelled",
      }),
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

  it("is null when nothing is owed", () => {
    expect(newestOpenInvoice([])).toBeNull();
    expect(
      newestOpenInvoice([paid("2026-10-01", "oct"), paid("2026-11-01", "nov")]),
    ).toBeNull();
  });
});

function ability(...capabilities: Capability[]) {
  return createAppAbility(capabilities);
}

describe("canSharePayLink", () => {
  it("shows for allow and conditional, hides for deny and a missing action", () => {
    expect(
      canSharePayLink(ability({ action: "invoice:share", access: "allow" })),
    ).toBe(true);
    expect(
      canSharePayLink(
        ability({ action: "invoice:share", access: "conditional" }),
      ),
    ).toBe(true);
    expect(
      canSharePayLink(ability({ action: "invoice:share", access: "deny" })),
    ).toBe(false);
    expect(
      canSharePayLink(ability({ action: "invoice:read", access: "allow" })),
    ).toBe(false);
  });
});

describe("payLinkInvoice", () => {
  const open = account([
    invoice({ id: "oct", period: "2026-10-01" }),
    invoice({ id: "nov", period: "2026-11-01" }),
  ]);

  it("is the newest open invoice for a member who may share", () => {
    expect(payLinkInvoice(true, open)?.id).toBe("nov");
  });

  it("hides the button without invoice:share", () => {
    expect(payLinkInvoice(false, open)).toBeNull();
  });

  it("hides the button until the account has loaded, or when it cannot be read", () => {
    expect(payLinkInvoice(true, undefined)).toBeNull();
  });

  it("hides the button when nothing is owed", () => {
    expect(payLinkInvoice(true, account([paid("2026-11-01", "nov")]))).toBe(
      null,
    );
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
