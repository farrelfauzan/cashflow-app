"use client";

type ImportColumnChipsProps = {
  title: string;
  columns: readonly string[];
};

// A group of column names as the template writes them ("Kolom wajib": properti, kamar, ...).
export function ImportColumnChips({ title, columns }: ImportColumnChipsProps) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-[15px] font-bold md:text-base">{title}</span>
      <ul aria-label={title} className="flex flex-wrap gap-1.5">
        {columns.map((column) => (
          <li
            key={column}
            className="rounded-md bg-secondary px-2 py-0.5 font-mono text-sm font-semibold md:text-base"
          >
            {column}
          </li>
        ))}
      </ul>
    </div>
  );
}
