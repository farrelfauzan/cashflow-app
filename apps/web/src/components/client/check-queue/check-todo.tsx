"use client";

import type { ReactNode } from "react";

import { TODO_TITLE } from "./utils";

type CheckTodoProps = {
  text: string;
  // A link or button under the text ("Lihat setoran ini", "Buka Pak Joko").
  children?: ReactNode;
};

// "Yang perlu Anda lakukan" and what to do about the item.
export function CheckTodo({ text, children }: CheckTodoProps) {
  return (
    <section className="flex flex-col gap-1.5">
      <h3 className="text-base font-bold md:text-[17px]">{TODO_TITLE}</h3>
      <p className="text-[15px] leading-normal md:text-base">{text}</p>
      {children}
    </section>
  );
}
