"use client";

import { Camera } from "lucide-react";

import { Button } from "@/components/ui/button";

import { ProofSheet } from "./proof-sheet";
import type { ProofSender } from "./type";
import { useSendProof } from "./use-send-proof";
import { PROOF_ACCEPT, PROOF_BUTTON_TEXT } from "./utils";

type SendProofButtonProps = {
  proof: ProofSender;
};

// "Kirim bukti transfer" under the transfer instructions (board "Kirim bukti transfer", 1): a
// plain button, so paying stays the page's primary. It opens the phone's camera or gallery; the
// chosen photo then shows in the sheet, which sends it.
export function SendProofButton({ proof }: SendProofButtonProps) {
  const { state, inputRef, pickPhoto, choose, send, close } =
    useSendProof(proof);

  return (
    <>
      <Button
        type="button"
        variant="outline"
        size="lg"
        className="w-full font-semibold"
        onClick={pickPhoto}
      >
        <Camera aria-hidden="true" />
        {PROOF_BUTTON_TEXT}
      </Button>
      <input
        ref={inputRef}
        type="file"
        accept={PROOF_ACCEPT}
        hidden
        tabIndex={-1}
        aria-hidden="true"
        onChange={(event) => {
          const file = event.target.files?.[0];
          // Cleared, so choosing the same photo again still reports a change.
          event.target.value = "";
          if (file !== undefined) choose(file);
        }}
      />
      <ProofSheet
        state={state}
        onPickPhoto={pickPhoto}
        onSend={send}
        onClose={close}
      />
    </>
  );
}
