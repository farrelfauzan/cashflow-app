"use client";

import { LoaderCircle, QrCode } from "lucide-react";

import { Button } from "@/components/ui/button";

import { QRIS_ABOVE_LIMIT_TEXT, QRIS_CREATING_TEXT } from "./utils";

type QrisStartProps = {
  // "Bayar QRIS Rp2.250.000", or null above Rp10.000.000.
  label: string | null;
  // A QR is being made (by this tap, or by another request after 409 qris_busy).
  creating: boolean;
  onStart: () => void;
};

// The QRIS tab before a QR exists: the one primary action, or why QRIS cannot take this total.
export function QrisStart({ label, creating, onStart }: QrisStartProps) {
  if (label === null && !creating) {
    return (
      <p className="rounded-xl border bg-card px-4 py-3.5 text-sm leading-normal md:px-5 md:py-4 md:text-base">
        {QRIS_ABOVE_LIMIT_TEXT}
      </p>
    );
  }
  return (
    <Button
      type="button"
      size="lg"
      className="w-full font-semibold"
      disabled={creating}
      onClick={onStart}
    >
      {creating ? (
        <LoaderCircle aria-hidden="true" className="animate-spin" />
      ) : (
        <QrCode aria-hidden="true" />
      )}
      {creating ? QRIS_CREATING_TEXT : label}
    </Button>
  );
}
