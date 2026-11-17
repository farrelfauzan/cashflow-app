import { describe, expect, it } from "vitest";

import type {
  ActivePromise,
  FieldPromiseOffer,
} from "@/lib/api/generated/model";

import {
  fieldPromiseAmountHint,
  fieldPromiseDialogView,
  fieldPromiseKindOptions,
} from "./utils";

const SUBTITLE = "Mbak Sinta Dewi · Kamar 15 · Kos Melati";
const DATES = { earliest: "2026-11-16", latest: "2027-01-15" };

const offer = (over: Partial<FieldPromiseOffer> = {}): FieldPromiseOffer => ({
  activePromise: null,
  promiseDates: DATES,
  nextInvoiceDueDate: "2026-12-15",
  outstanding: 1_050_000,
  ...over,
});

const active = (over: Partial<ActivePromise> = {}): ActivePromise => ({
  id: "promise-1",
  version: 1,
  kind: "specific_date",
  promisedDate: "2026-11-20",
  promisedAmount: null,
  payingNow: 0,
  source: "karyawan",
  note: null,
  createdBy: null,
  createdAt: "2026-11-16T03:00:00Z",
  coveredInvoiceIds: ["inv-nov"],
  ...over,
});

describe("fieldPromiseKindOptions", () => {
  it("offers Pilih tanggal first, then the next due date", () => {
    expect(fieldPromiseKindOptions("2026-12-15")).toEqual([
      {
        value: "specific_date",
        label: "Pilih tanggal",
        sub: "Tanggal yang disepakati dengan penyewa",
      },
      {
        value: "with_next_invoice",
        label: "Bareng jatuh tempo berikutnya",
        sub: "15 Des 2026",
      },
    ]);
  });

  it("hides Bareng jatuh tempo berikutnya without a next due date", () => {
    expect(fieldPromiseKindOptions(null).map((option) => option.value)).toEqual(
      ["specific_date"],
    );
  });
});

describe("fieldPromiseAmountHint", () => {
  it("names everything the tenant owes", () => {
    expect(fieldPromiseAmountHint(1_050_000)).toBe(
      "Kosong = semua sisa, Rp1.050.000.",
    );
  });
});

describe("fieldPromiseDialogView", () => {
  it("is null when the offer has no dates", () => {
    expect(
      fieldPromiseDialogView(
        offer({ promiseDates: null, nextInvoiceDueDate: null }),
        SUBTITLE,
      ),
    ).toBeNull();
  });

  it("opens empty on Pilih tanggal with the board's copy", () => {
    const view = fieldPromiseDialogView(offer(), SUBTITLE);
    expect(view).toMatchObject({
      title: "Catat janji bayar",
      subtitle: SUBTITLE,
      intro: "Tanpa denda. Admin dan penyewa diberi tahu.",
      replacesLine: null,
      dateLabel: "Tanggal janji",
      dateMin: "2026-11-16",
      dateMax: "2027-01-15",
      amountLabel: "Jumlah janji (boleh kosong)",
      amountHint: "Kosong = semua sisa, Rp1.050.000.",
      primaryLabel: "Simpan janji",
      showNote: false,
      offer: {
        outstanding: 1_050_000,
        earliest: "2026-11-16",
        latest: "2027-01-15",
        nextInvoiceDueDate: "2026-12-15",
      },
      defaults: { kind: "specific_date", date: "", amountText: "", note: "" },
    });
    expect(view?.kindOptions).toHaveLength(2);
  });

  it("opens filled from the active promise, which saving replaces", () => {
    const view = fieldPromiseDialogView(
      offer({ activePromise: active({ promisedAmount: 500_000 }) }),
      SUBTITLE,
    );
    expect(view?.replacesLine).toBe(
      "Janji aktif Jum, 20 Nov, Rp500.000 akan diganti.",
    );
    expect(view?.primaryLabel).toBe("Simpan perubahan");
    expect(view?.defaults).toEqual({
      kind: "specific_date",
      date: "2026-11-20",
      amountText: "500.000",
      note: "",
    });
  });

  it("keeps an active Bareng promise on its choice", () => {
    const view = fieldPromiseDialogView(
      offer({
        activePromise: active({
          kind: "with_next_invoice",
          promisedDate: "2026-12-15",
        }),
      }),
      SUBTITLE,
    );
    expect(view?.defaults.kind).toBe("with_next_invoice");
  });

  it("takes what the tenant owes from the offer, read when the sheet opens", () => {
    const view = fieldPromiseDialogView(
      offer({ outstanding: 250_000 }),
      SUBTITLE,
    );
    expect(view?.amountHint).toBe("Kosong = semua sisa, Rp250.000.");
    expect(view?.offer.outstanding).toBe(250_000);
  });
});
