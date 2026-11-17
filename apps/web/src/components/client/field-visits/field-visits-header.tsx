"use client";

import { TITLE } from "./utils";

type FieldVisitsHeaderProps = {
  // "Senin, 16 Nov · disusun otomatis setiap pagi", today in Jakarta.
  dateLine: string;
};

// The top of Kunjungan hari ini: today's date and how the list is made, above the title.
export function FieldVisitsHeader({ dateLine }: FieldVisitsHeaderProps) {
  return (
    <header className="flex flex-col gap-0.5">
      <p className="text-sm text-muted-foreground md:text-base">{dateLine}</p>
      <h1 className="text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
        {TITLE}
      </h1>
    </header>
  );
}
