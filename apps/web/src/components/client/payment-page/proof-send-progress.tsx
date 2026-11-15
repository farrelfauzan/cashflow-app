"use client";

import { Progress } from "@/components/ui/progress";

import { proofSendingLine } from "./utils";

type ProofSendProgressProps = {
  // Null until the browser reports the upload's size.
  percent: number | null;
};

// "Mengirim… 62%" under a bar, while the photo goes to our own API.
export function ProofSendProgress({ percent }: ProofSendProgressProps) {
  const line = proofSendingLine(percent);
  return (
    <div className="flex flex-col gap-2">
      <Progress
        value={percent}
        aria-label={line}
        className="h-1.5 bg-secondary"
      />
      <p className="text-sm text-muted-foreground md:text-base">{line}</p>
    </div>
  );
}
