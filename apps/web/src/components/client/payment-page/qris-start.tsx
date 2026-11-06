"use client";

import { LoaderCircle, QrCode } from "lucide-react";

import { Button } from "@/components/ui/button";

import { QRIS_CREATING_TEXT } from "./utils";

type QrisStartProps = {
  // "Bayar QRIS Rp2.250.000", or "Bayar QRIS Rp10.000.000" when more is owed.
  label: string;
  // A QR is being made (by this tap, or by another request after 409 qris_busy).
  creating: boolean;
  onStart: () => void;
};

// The QRIS tab before a QR exists: the one primary action.
export function QrisStart({ label, creating, onStart }: QrisStartProps) {
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
