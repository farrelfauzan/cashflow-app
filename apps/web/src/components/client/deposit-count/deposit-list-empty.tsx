"use client";

type DepositListEmptyProps = {
  text: string;
};

// A tab with no deposit: one calm line, no button.
export function DepositListEmpty({ text }: DepositListEmptyProps) {
  return (
    <p className="rounded-[14px] border bg-card px-4 py-4 text-base">{text}</p>
  );
}
