"use client";

import { useCallback, useState } from "react";

import type { TenantOpening, TenantOpenings } from "./type";
import { markOpening, nextOpenings } from "./utils";

// The openings of one kind of sheet on Kunjungan hari ini ("Catat hasil kunjungan", or the
// "Terima tunai" it opens), held by the screen rather than by a row, so a row that moves to "Sudah
// dicatat hari ini" or leaves the list never takes an open sheet with it. Each opening gets a
// requestId, sent unchanged on every retry. An opening whose save got no answer reports itself
// unsure and is kept: opening that tenant's sheet again brings it back as it was (same requestId,
// same values) instead of a fresh one that could record the same thing twice. The screen renders
// one sheet per opening, keyed by its requestId; it stays mounted after it closes, so it can
// animate out.
export function useTenantOpenings<
  T extends { tenantId: string },
>(): TenantOpenings<T> {
  const [openings, setOpenings] = useState<readonly TenantOpening<T>[]>([]);
  const [activeTenantId, setActiveTenantId] = useState<string | null>(null);
  const [open, setOpen] = useState(false);

  function start(item: T) {
    const requestId = crypto.randomUUID();
    setOpenings((current) => nextOpenings(current, item, requestId));
    setActiveTenantId(item.tenantId);
    setOpen(true);
  }

  // Stable, since each sheet reports through it from an effect.
  const markUnsure = useCallback((tenantId: string, unsure: boolean) => {
    setOpenings((current) => markOpening(current, tenantId, unsure));
  }, []);

  return { openings, activeTenantId, open, start, setOpen, markUnsure };
}
