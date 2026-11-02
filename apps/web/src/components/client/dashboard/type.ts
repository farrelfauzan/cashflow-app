// The Supervisor's home, Beranda (board "iPad · Beranda Supervisor"). Every value is ready to
// show; every amount in it is a field getDashboard sent, only formatted.

// "Tagihan bulan November": what the month's invoices collected out of what they bill.
export type MonthMoneyView = {
  // "Tagihan bulan November".
  title: string;
  // collected, formatted.
  collected: string;
  // How full the bar is, 0 to 100, from collected and billed; null when nothing is billed, so
  // there is no bar.
  percent: number | null;
  // The line under the bar.
  summary:
    | { kind: "none" }
    | { kind: "paid"; billed: string }
    | {
        kind: "owing";
        billed: string;
        outstanding: string;
        tenants: number;
      };
};

// "Tunggakan": everything owed after its due date, of any month.
export type ArrearsTotalView = {
  value: string;
  note: string;
  tone: "default" | "warning";
};

// One tenant in "Belum bayar", the oldest debt first.
export type ArrearsRowView = {
  tenantId: string;
  name: string;
  // "Kamar 11, Kos Melati", plus "+1 unit lain" when the debt spans several units.
  place: string;
  // overdue, formatted.
  amount: string;
  // "Terlambat 12 hari".
  age: string;
};
