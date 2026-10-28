"use client";

import { Users } from "lucide-react";

// The organization has no tenant the member can see. The board's second sentence points at
// "Tambah penyewa" and the import, which are not built yet.
export function TenantsEmpty() {
  return (
    <div className="flex flex-col items-center gap-3 rounded-[14px] border bg-card px-5 py-8 text-center md:rounded-2xl md:px-7 md:py-11">
      <span
        aria-hidden="true"
        className="hidden size-16 items-center justify-center rounded-[18px] bg-secondary md:flex"
      >
        <Users className="size-8 text-muted-foreground" strokeWidth={1.8} />
      </span>
      <h2 className="text-[22px] font-bold md:text-2xl">Belum ada penyewa</h2>
    </div>
  );
}
