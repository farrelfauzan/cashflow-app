"use client";

type CountBadgeProps = {
  count: number;
};

// A small accent count beside a label: "Menunggu dihitung [2]", "Setoran tunai [2]". The label
// around it carries the count for screen readers, so the badge itself is hidden from them.
export function CountBadge({ count }: CountBadgeProps) {
  return (
    <span
      aria-hidden="true"
      className="inline-flex min-w-6 items-center justify-center rounded-full bg-primary px-1.5 text-sm leading-6 font-bold text-primary-foreground tabular-nums"
    >
      {count}
    </span>
  );
}
