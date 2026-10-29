"use client";

import { EFFECTS_TITLE } from "./utils";

type LeaseEffectsBoxProps = {
  items: readonly string[];
};

// "Yang terjadi setelah ini": what a planned end or a move-out does, in a grey box.
export function LeaseEffectsBox({ items }: LeaseEffectsBoxProps) {
  return (
    <div className="flex flex-col gap-2.5 rounded-xl bg-muted p-3.5">
      <p className="text-[15px] font-bold md:text-base">{EFFECTS_TITLE}</p>
      <ul className="flex list-disc flex-col gap-2 pl-5 text-[15px] leading-normal md:text-base">
        {items.map((item) => (
          <li key={item}>{item}</li>
        ))}
      </ul>
    </div>
  );
}
