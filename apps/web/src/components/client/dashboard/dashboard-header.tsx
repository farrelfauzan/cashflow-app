"use client";

type DashboardHeaderProps = {
  // "Selamat pagi".
  greeting: string;
  // "Kamis, 22 Oktober 2026".
  date: string;
};

// The top of Beranda: the greeting and today's date in Jakarta.
export function DashboardHeader({ greeting, date }: DashboardHeaderProps) {
  return (
    <header className="flex flex-col gap-0.5">
      <h1 className="text-[26px] font-bold tracking-tight md:text-[34px]">
        {greeting}
      </h1>
      <p className="text-base text-muted-foreground md:text-xl">{date}</p>
    </header>
  );
}
