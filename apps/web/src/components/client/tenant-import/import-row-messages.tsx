"use client";

import type { ImportRowView } from "./type";

type ImportRowMessagesProps = {
  row: ImportRowView;
};

// A row's problems ("Kolom no_wa: ...") and notes ("Catatan: ..."), under its other lines.
export function ImportRowMessages({ row }: ImportRowMessagesProps) {
  if (row.errors.length === 0 && row.notes.length === 0) return null;
  return (
    <div className="flex flex-col gap-1">
      {row.errors.map((message, index) => (
        <p
          key={`error-${index}`}
          className="text-sm leading-[1.45] text-destructive md:text-base"
        >
          {message.label !== null && (
            <span className="font-bold">{message.label}</span>
          )}
          {message.text}
        </p>
      ))}
      {row.notes.map((message, index) => (
        <p
          key={`note-${index}`}
          className="text-sm leading-[1.45] text-warning md:text-base"
        >
          {message.label !== null && (
            <span className="font-bold">{message.label}</span>
          )}
          {message.text}
        </p>
      ))}
    </div>
  );
}
