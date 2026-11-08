import { cn } from "@/lib/utils";

import { MARK_ARC_PATH, MARK_ARROWHEAD_PATH, MARK_VIEW_BOX } from "./utils";

type AppMarkProps = {
  className?: string;
};

// The ReCash mark (board Logo-Opsi-3), drawn inline so it costs no request. Decorative: the
// wordmark next to it carries the name.
export function AppMark({ className }: AppMarkProps) {
  return (
    <svg
      aria-hidden="true"
      viewBox={MARK_VIEW_BOX}
      className={cn("block shrink-0", className)}
    >
      <rect width="64" height="64" rx="16" className="fill-primary" />
      <path
        d={MARK_ARC_PATH}
        fill="none"
        strokeWidth={5}
        strokeLinecap="round"
        className="stroke-primary-foreground"
      />
      <path
        d={MARK_ARROWHEAD_PATH}
        strokeWidth={2}
        strokeLinejoin="round"
        className="fill-primary-foreground stroke-primary-foreground"
      />
      <circle cx="32" cy="32" r="10.5" className="fill-coin" />
      <circle
        cx="32"
        cy="32"
        r="6.8"
        fill="none"
        strokeWidth={1.8}
        className="stroke-coin-ring"
      />
    </svg>
  );
}
