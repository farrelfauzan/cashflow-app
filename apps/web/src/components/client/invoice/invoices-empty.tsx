"use client";

type InvoicesEmptyProps = {
  // "Belum ada tagihan bulan ini." (or the month's name).
  text: string;
};

// The month has no invoice and nothing narrows the list. The screen has no primary action: the
// admin makes no invoice by hand, the system does.
export function InvoicesEmpty({ text }: InvoicesEmptyProps) {
  return (
    <div className="flex flex-col items-center gap-2.5 rounded-[14px] border bg-card px-5 py-8 text-center md:gap-3 md:rounded-2xl md:px-7 md:py-11">
      <h2 className="text-xl font-bold md:text-2xl">{text}</h2>
    </div>
  );
}
