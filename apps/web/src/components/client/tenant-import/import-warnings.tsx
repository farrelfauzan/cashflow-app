"use client";

import { TriangleAlert } from "lucide-react";

import type { ImportWarning } from "@/lib/api/generated/model";

type ImportWarningsProps = {
  warnings: readonly ImportWarning[];
};

// "Peringatan untuk seluruh berkas": the API's notes about the file as a whole, in its words.
export function ImportWarnings({ warnings }: ImportWarningsProps) {
  return (
    <section
      aria-labelledby="import-warnings-title"
      className="flex flex-col gap-2 rounded-2xl border border-warning/40 bg-warning/10 px-3.5 py-3"
    >
      <div className="flex items-center gap-2">
        <TriangleAlert
          aria-hidden="true"
          className="size-5 shrink-0 text-warning"
          strokeWidth={2.2}
        />
        <h2 id="import-warnings-title" className="text-base font-bold">
          Peringatan untuk seluruh berkas
        </h2>
      </div>
      <ul className="flex flex-col gap-1">
        {warnings.map((warning) => (
          <li
            key={warning.code}
            className="text-sm leading-relaxed md:text-base"
          >
            {warning.message}
          </li>
        ))}
      </ul>
    </section>
  );
}
