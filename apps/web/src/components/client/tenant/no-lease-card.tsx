"use client";

// A tenant who has no lease the member may see.
export function NoLeaseCard() {
  return (
    <section
      aria-labelledby="no-lease"
      className="rounded-[14px] border bg-card p-4 md:rounded-2xl md:px-6 md:py-[22px]"
    >
      <h2 id="no-lease" className="text-xl font-bold md:text-2xl">
        Belum punya sewa
      </h2>
    </section>
  );
}
