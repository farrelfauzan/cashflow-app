"use client";

type NoTransferNoteProps = {
  text: string;
};

// The organization has set no bank account: the tenant is told whom to contact instead.
export function NoTransferNote({ text }: NoTransferNoteProps) {
  return (
    <p className="rounded-xl border bg-card px-4 py-3.5 text-sm leading-normal md:px-5 md:py-4 md:text-base">
      {text}
    </p>
  );
}
