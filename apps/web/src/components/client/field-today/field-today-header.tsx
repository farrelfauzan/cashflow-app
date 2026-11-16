"use client";

import { TITLE } from "./utils";

type FieldTodayHeaderProps = {
  // "Senin, 16 Nov", today in Jakarta.
  date: string;
};

// The top of Tagihan hari ini: today's date above the title.
export function FieldTodayHeader({ date }: FieldTodayHeaderProps) {
  return (
    <header className="flex flex-col gap-0.5">
      <p className="text-sm text-muted-foreground md:text-base">{date}</p>
      <h1 className="text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
        {TITLE}
      </h1>
    </header>
  );
}
