"use client";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { CommitButtonView } from "./type";

type ImportCommitButtonProps = {
  commit: CommitButtonView;
  // The commit is on its way: the button waits, and never sends twice.
  busy: boolean;
  onCommit: () => void;
  className?: string;
};

// "Simpan N baris": saves every row in one go, only when no row has an error.
export function ImportCommitButton({
  commit,
  busy,
  onCommit,
  className,
}: ImportCommitButtonProps) {
  return (
    <Button
      type="button"
      onClick={onCommit}
      disabled={!commit.enabled || busy}
      aria-busy={busy}
      className={cn(
        "h-12 w-full rounded-xl px-5 text-[17px] font-semibold",
        className,
      )}
    >
      {busy ? "Menyimpan…" : commit.label}
    </Button>
  );
}
