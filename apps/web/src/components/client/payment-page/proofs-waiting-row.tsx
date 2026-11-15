"use client";

import { Clock } from "lucide-react";

import type { ProofRowView } from "./type";
import { PROOF_WAITING_TEXT } from "./utils";

type ProofsWaitingRowProps = {
  row: ProofRowView;
};

// One proof still waiting: when it was sent, and that the admin has yet to check it.
export function ProofsWaitingRow({ row }: ProofsWaitingRowProps) {
  return (
    <li className="flex items-start gap-2.5 py-2">
      <Clock
        aria-hidden="true"
        className="mt-0.5 size-5 shrink-0 text-muted-foreground"
      />
      <div className="flex min-w-0 flex-col">
        <span className="text-sm font-semibold md:text-base">
          {row.sentLine}
        </span>
        <span className="text-sm text-muted-foreground md:text-base">
          {PROOF_WAITING_TEXT}
        </span>
      </div>
    </li>
  );
}
