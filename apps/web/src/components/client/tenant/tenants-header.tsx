"use client";

import { tenantsSubtitle } from "./utils";

type TenantsHeaderProps = {
  organizationName: string;
};

// The title and what the screen is for (boards Penyewa.dc.html and Penyewa-Phone.dc.html). The
// "Tambah penyewa" and import buttons come with the screens that add tenants.
export function TenantsHeader({ organizationName }: TenantsHeaderProps) {
  return (
    <header className="flex flex-col gap-1.5">
      <h1 className="text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
        Penyewa
      </h1>
      <p className="text-[15px] leading-normal text-muted-foreground md:text-[17px]">
        {tenantsSubtitle(organizationName)}
      </p>
    </header>
  );
}
