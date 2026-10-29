"use client";

import { CircleAlert } from "lucide-react";

import { UNREADABLE_TITLE } from "./utils";

type ImportUnreadableBoxProps = {
  // The API's message, already in Bahasa Indonesia.
  message: string;
};

// "Berkas tidak bisa dibaca": the API refused the file as a whole (400, field file).
export function ImportUnreadableBox({ message }: ImportUnreadableBoxProps) {
  return (
    <div
      role="alert"
      className="flex items-start gap-2.5 rounded-xl bg-destructive/10 p-3.5 text-destructive"
    >
      <CircleAlert
        aria-hidden="true"
        className="mt-0.5 size-5 shrink-0"
        strokeWidth={2.2}
      />
      <div className="flex flex-col gap-0.5">
        <span className="text-base font-bold">{UNREADABLE_TITLE}</span>
        <span className="text-[15px] leading-normal md:text-[17px]">
          {message}
        </span>
      </div>
    </div>
  );
}
