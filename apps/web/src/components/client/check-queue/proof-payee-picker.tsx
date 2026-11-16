"use client";

import { Search } from "lucide-react";
import { useId, useState } from "react";

import {
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
} from "@/components/client/form/utils";
import { MAX_SEARCH_LENGTH } from "@/components/client/list/utils";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup } from "@/components/ui/radio-group";

import type { ProofPayee } from "./type";
import {
  LOADING_TEXT,
  PAYEE_EMPTY_TEXT,
  PAYEE_GROUP_HEADING_CLASS,
  PAYEE_INTRO,
  PAYEE_SEARCH_HINT,
  PAYEE_SEARCH_LABEL,
  PAYEE_SENDER_LABEL,
  PAYEE_TITLE,
} from "./utils";

import { CheckCallout } from "./check-callout";
import { ProofPayeeOption } from "./proof-payee-option";
import { usePayeeSearch } from "./use-payee-search";

type ProofPayeePickerProps = {
  orgId: string;
  // The proof's sender: always the first choice, on their own.
  sender: ProofPayee;
  // The tenant chosen in the picker so far.
  selected: ProofPayee;
  onSelect: (payee: ProofPayee) => void;
  // What the search starts with: the chosen tenant's name when it is not the sender, so they are
  // listed again.
  initialSearch: string;
};

// "Pembayaran untuk siapa?" (board Admin-Cek-Bukti-Ganti), in place of the proof in the same
// drawer: "Cari penyewa" searches the tenants the member may see on the server (name, unit or
// WhatsApp number); the sender is always first, then the tenants found, each a choice of one
// radio group. A tenant whose lease ended can be chosen ("Sewa selesai"): they may still owe.
export function ProofPayeePicker({
  orgId,
  sender,
  selected,
  onSelect,
  initialSearch,
}: ProofPayeePickerProps) {
  const id = useId();
  const searchId = `${id}-search`;
  const hintId = `${id}-hint`;
  const senderHeadingId = `${id}-sender`;
  const foundHeadingId = `${id}-found`;
  const [text, setText] = useState(initialSearch);
  const search = usePayeeSearch({ orgId, senderId: sender.id, text });
  const found = search.view?.payees ?? [];

  function choose(payeeId: string) {
    const payee = [sender, ...found].find((each) => each.id === payeeId);
    if (payee !== undefined) onSelect(payee);
  }

  let results = null;
  if (search.error !== null) {
    results = <p className={FIELD_HINT_CLASS}>{search.error}</p>;
  } else if (search.view !== null) {
    results = (
      <div
        role="group"
        aria-labelledby={foundHeadingId}
        aria-busy={search.loading}
        className="flex flex-col gap-2 transition-opacity aria-busy:opacity-70"
      >
        <h3 id={foundHeadingId} className={PAYEE_GROUP_HEADING_CLASS}>
          {search.view.heading}
        </h3>
        {found.length === 0 ? (
          <p className={FIELD_HINT_CLASS}>{PAYEE_EMPTY_TEXT}</p>
        ) : (
          found.map((payee) => (
            <ProofPayeeOption
              key={payee.id}
              inputId={`${id}-${payee.id}`}
              payee={payee}
            />
          ))
        )}
      </div>
    );
  } else if (search.loading) {
    results = <p className={FIELD_HINT_CLASS}>{LOADING_TEXT}</p>;
  }

  return (
    <>
      <CheckCallout tone="info" text={PAYEE_INTRO} />
      <div className="flex flex-col gap-2">
        <Label htmlFor={searchId} className={FIELD_LABEL_CLASS}>
          {PAYEE_SEARCH_LABEL}
        </Label>
        <div className="relative w-full">
          <Search
            aria-hidden="true"
            className="pointer-events-none absolute top-1/2 left-3.5 size-5 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            id={searchId}
            type="search"
            autoComplete="off"
            maxLength={MAX_SEARCH_LENGTH}
            aria-describedby={hintId}
            value={text}
            onChange={(event) => {
              setText(event.target.value);
              // A new search may not list the tenant chosen so far; the choice goes back to the
              // sender, so "Pakai penyewa ini" never applies a tenant who is not on screen.
              if (selected.id !== sender.id) onSelect(sender);
            }}
            className="h-13 rounded-xl border-[1.5px] bg-card pr-3.5 pl-[46px] text-base md:text-[17px]"
          />
        </div>
        <p id={hintId} className={FIELD_HINT_CLASS}>
          {PAYEE_SEARCH_HINT}
        </p>
      </div>
      <RadioGroup
        aria-label={PAYEE_TITLE}
        value={selected.id}
        onValueChange={choose}
        className="gap-[18px]"
      >
        <div
          role="group"
          aria-labelledby={senderHeadingId}
          className="flex flex-col gap-2"
        >
          <h3 id={senderHeadingId} className={PAYEE_GROUP_HEADING_CLASS}>
            {PAYEE_SENDER_LABEL}
          </h3>
          <ProofPayeeOption inputId={`${id}-${sender.id}`} payee={sender} />
        </div>
        {results}
      </RadioGroup>
    </>
  );
}
