"use client";

import { useRef, type ChangeEvent } from "react";

import { FormAlert } from "@/components/client/form/form-alert";
import type { ChooseProblem } from "./type";
import {
  CHECK_SAVES_NOTHING_TEXT,
  chooseAction,
  FILE_ACCEPT,
  TOO_LARGE_MESSAGE,
} from "./utils";

import { ImportChooseAction } from "./import-choose-action";
import { ImportFileCard } from "./import-file-card";
import { ImportFilePicker } from "./import-file-picker";
import { ImportFormatCard } from "./import-format-card";
import { ImportPhoneFooter } from "./import-phone-footer";
import { ImportUnreadableBox } from "./import-unreadable-box";

type ImportChooseStepProps = {
  orgId: string;
  file: File | null;
  problem: ChooseProblem | null;
  busy: boolean;
  onChoose: (file: File) => void;
  onCheck: () => void;
};

// Step 1, "Pilih berkas" (boards Impor.dc.html and Impor-Phone.dc.html): the file, the format
// card, and "Periksa berkas". The file input is hidden; the buttons open it.
export function ImportChooseStep({
  orgId,
  file,
  problem,
  busy,
  onChoose,
  onCheck,
}: ImportChooseStepProps) {
  const input = useRef<HTMLInputElement>(null);
  const pick = () => input.current?.click();
  const action = chooseAction(file !== null, problem);

  function change(event: ChangeEvent<HTMLInputElement>) {
    const chosen = event.target.files?.[0];
    // Cleared, so choosing the same file again (after fixing it) is a change too.
    event.target.value = "";
    if (chosen !== undefined) onChoose(chosen);
  }

  return (
    <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:gap-5">
      <section
        aria-labelledby="import-file-title"
        className="flex min-w-0 flex-1 flex-col gap-4 lg:gap-[18px] lg:rounded-2xl lg:border lg:bg-card lg:p-[22px]"
      >
        <h2
          id="import-file-title"
          className="sr-only lg:not-sr-only lg:text-[22px] lg:font-bold"
        >
          Berkas penyewa
        </h2>
        <input
          ref={input}
          type="file"
          accept={FILE_ACCEPT}
          aria-label="Berkas CSV"
          className="hidden"
          onChange={change}
        />
        {file === null ? (
          <ImportFilePicker onPick={pick} />
        ) : (
          <ImportFileCard
            name={file.name}
            sizeBytes={file.size}
            onChange={pick}
          />
        )}
        {problem?.kind === "tooLarge" && (
          <FormAlert message={TOO_LARGE_MESSAGE} />
        )}
        {problem?.kind === "unreadable" && (
          <ImportUnreadableBox message={problem.message} />
        )}
        {problem?.kind === "alert" && <FormAlert message={problem.message} />}
        <p className="text-[15px] leading-normal text-muted-foreground md:text-base">
          {CHECK_SAVES_NOTHING_TEXT}
        </p>
        <div className="hidden justify-end lg:flex">
          <ImportChooseAction
            action={action}
            busy={busy}
            onCheck={onCheck}
            onPick={pick}
          />
        </div>
      </section>
      <div className="lg:w-[360px] lg:shrink-0">
        <ImportFormatCard orgId={orgId} />
      </div>
      <ImportPhoneFooter>
        <ImportChooseAction
          action={action}
          busy={busy}
          onCheck={onCheck}
          onPick={pick}
          className="h-13 w-full"
        />
      </ImportPhoneFooter>
    </div>
  );
}
