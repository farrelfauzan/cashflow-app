"use client";

import { Search } from "lucide-react";

import { Input } from "@/components/ui/input";
import { SEARCH_LABEL } from "./utils";

type TenantSearchFieldProps = {
  value: string;
  onChange: (value: string) => void;
};

// "Cari nama, nomor, atau unit": narrows the list as the member types.
export function TenantSearchField({ value, onChange }: TenantSearchFieldProps) {
  return (
    <div className="relative w-full">
      <Search
        aria-hidden="true"
        className="pointer-events-none absolute top-1/2 left-3.5 size-5 -translate-y-1/2 text-muted-foreground"
      />
      <Input
        type="text"
        autoComplete="off"
        aria-label={SEARCH_LABEL}
        placeholder={SEARCH_LABEL}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className="h-12 rounded-xl border-[1.5px] bg-card pr-3.5 pl-[46px] text-base md:text-[17px]"
      />
    </div>
  );
}
