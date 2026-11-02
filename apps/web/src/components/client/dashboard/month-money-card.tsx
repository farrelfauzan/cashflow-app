"use client";

import type { MonthMoneyView } from "./type";

type MonthMoneyCardProps = {
  money: MonthMoneyView;
};

// "Uang sewa bulan November": what came in on the month's invoices, a bar of how much of what
// they bill that is, and what is still owed and by how many tenants.
export function MonthMoneyCard({ money }: MonthMoneyCardProps) {
  const { summary } = money;
  return (
    <div
      role="group"
      aria-label={money.title}
      className="flex min-w-0 flex-col gap-2.5 rounded-[14px] border-2 border-primary/25 bg-accent p-4 md:rounded-2xl md:px-6 md:py-[22px]"
    >
      <p className="text-base font-semibold text-primary md:text-[19px]">
        {money.title}
      </p>
      <p className="text-[28px] leading-tight font-bold tracking-tight md:text-[40px]">
        {money.collected}{" "}
        <span className="text-lg font-semibold md:text-[22px]">
          sudah masuk
        </span>
      </p>
      {money.percent !== null && (
        <div
          role="img"
          aria-label={`${money.percent} persen terkumpul`}
          className="h-4 overflow-hidden rounded-lg border border-primary/25 bg-card"
        >
          <div
            className="h-full bg-primary"
            style={{ width: `${money.percent}%` }}
          />
        </div>
      )}
      <p className="text-[15px] md:text-lg">
        {summary.kind === "none" && "Belum ada tagihan bulan ini."}
        {summary.kind === "paid" && (
          <>Dari {summary.billed}. Semua sudah lunas.</>
        )}
        {summary.kind === "owing" && (
          <>
            Dari {summary.billed}. Masih kurang <b>{summary.outstanding}</b>{" "}
            dari {summary.tenants} penyewa.
          </>
        )}
      </p>
    </div>
  );
}
