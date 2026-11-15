"use client";

type VisitNoteBoxProps = {
  // "Catatan Mas Dimas Saputra · Sabtu 14 Nov, 17.02".
  caption: string;
  // The note in quotes.
  note: string;
};

// What the karyawan wrote when they recorded the new number.
export function VisitNoteBox({ caption, note }: VisitNoteBoxProps) {
  return (
    <figure className="flex flex-col gap-1 rounded-xl border px-3.5 py-3">
      <figcaption className="text-sm text-muted-foreground md:text-base">
        {caption}
      </figcaption>
      <blockquote className="text-[15px] leading-normal md:text-base">
        {note}
      </blockquote>
    </figure>
  );
}
