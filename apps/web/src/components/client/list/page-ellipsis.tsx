// The gap between page numbers. Decorative: the list item around it is hidden from screen readers.
export function PageEllipsis() {
  return (
    <span
      aria-hidden="true"
      className="flex h-11 min-w-8 items-center justify-center text-base text-muted-foreground"
    >
      …
    </span>
  );
}
