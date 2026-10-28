"use client";

import { Search } from "lucide-react";
import { useEffect, useEffectEvent, useState } from "react";

import { Input } from "@/components/ui/input";
import { MAX_SEARCH_LENGTH, SEARCH_DELAY_MS } from "./utils";

type ListSearchFieldProps = {
  // The search in the URL.
  value: string;
  // The field's label and placeholder ("Cari nama, nomor, atau unit").
  label: string;
  // Called with the text once the member stops typing for a moment.
  onSearch: (value: string) => void;
};

// The search box of a paginated list. It shows what the member types at once and asks the API
// only after a short pause. When the URL's search changes from elsewhere (back, "Hapus
// penyaring"), the field follows it, unless it is the search this field just sent.
export function ListSearchField({
  value,
  label,
  onSearch,
}: ListSearchFieldProps) {
  const [text, setText] = useState(value);
  const [seen, setSeen] = useState(value);
  const [sent, setSent] = useState(value);
  if (value !== seen) {
    setSeen(value);
    if (value !== sent) {
      setText(value);
      setSent(value);
    }
  }

  const search = useEffectEvent((next: string) => {
    setSent(next);
    onSearch(next);
  });

  useEffect(() => {
    if (text === value) {
      return;
    }
    const timer = setTimeout(() => search(text), SEARCH_DELAY_MS);
    return () => clearTimeout(timer);
  }, [text, value]);

  return (
    <div className="relative w-full">
      <Search
        aria-hidden="true"
        className="pointer-events-none absolute top-1/2 left-3.5 size-5 -translate-y-1/2 text-muted-foreground"
      />
      <Input
        type="search"
        autoComplete="off"
        maxLength={MAX_SEARCH_LENGTH}
        aria-label={label}
        placeholder={label}
        value={text}
        onChange={(event) => setText(event.target.value)}
        className="h-12 rounded-xl border-[1.5px] bg-card pr-3.5 pl-[46px] text-base md:text-[17px]"
      />
    </div>
  );
}
