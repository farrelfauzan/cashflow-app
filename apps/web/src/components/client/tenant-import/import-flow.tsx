"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";

import { toFormError } from "@/lib/api/form-error";
import { useImportTenants } from "@/lib/api/generated/tenants/tenants";
import { orgQueryKeyPrefix } from "@/lib/api/query-key";
import type { ChooseProblem, ImportStep } from "./type";
import { checkProblem, isTooLarge, stepHeading } from "./utils";

import { ImportCheckActions } from "./import-check-actions";
import { ImportCheckStep } from "./import-check-step";
import { ImportChooseStep } from "./import-choose-step";
import { ImportDoneStep } from "./import-done-step";
import { ImportHeader } from "./import-header";
import { ImportStepper } from "./import-stepper";

type ImportFlowProps = {
  orgId: string;
  organizationName: string;
};

// "Impor penyewa": choose a CSV file, check it (a dry run that saves nothing), then save every
// row in one go. The steps live in this screen's state. The file's text is read once, in the
// browser, and the commit sends that same text; it is never logged.
export function ImportFlow({ orgId, organizationName }: ImportFlowProps) {
  const queryClient = useQueryClient();
  const importTenants = useImportTenants();
  const [step, setStep] = useState<ImportStep>({ kind: "choose" });
  const [file, setFile] = useState<File | null>(null);
  const [problem, setProblem] = useState<ChooseProblem | null>(null);
  const [commitError, setCommitError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  // Blocks a second request while one is on its way, even before `busy` renders.
  const sending = useRef(false);

  function showStep(next: ImportStep) {
    setStep(next);
    window.scrollTo({ top: 0 });
  }

  function choose(next: File) {
    setFile(next);
    setProblem(isTooLarge(next.size) ? { kind: "tooLarge" } : null);
  }

  function restart() {
    setFile(null);
    setProblem(null);
    setCommitError(null);
    showStep({ kind: "choose" });
  }

  async function check() {
    if (file === null || isTooLarge(file.size) || sending.current) return;
    sending.current = true;
    setBusy(true);
    setProblem(null);
    try {
      const text = await file.text();
      const result = await importTenants.mutateAsync({
        orgId,
        data: text,
        params: { dryRun: true },
      });
      setCommitError(null);
      showStep({
        kind: "check",
        fileName: file.name,
        text,
        result,
        changed: false,
      });
    } catch (error) {
      setProblem(checkProblem(error));
    } finally {
      sending.current = false;
      setBusy(false);
    }
  }

  async function commit() {
    if (step.kind !== "check" || sending.current) return;
    sending.current = true;
    setBusy(true);
    setCommitError(null);
    try {
      const result = await importTenants.mutateAsync({
        orgId,
        data: step.text,
        params: { dryRun: false },
      });
      if (result.committed) {
        // An import can create properties, units, tenants and leases anywhere in the
        // organization, so every cached query of it may be stale: the whole organization prefix
        // is invalidated rather than a list of keys that would miss one.
        void queryClient.invalidateQueries({
          queryKey: orgQueryKeyPrefix(orgId),
        });
        showStep({ kind: "done", fileName: step.fileName, result });
      } else {
        // Nothing was saved: the rows changed since the check. The new result is the check now.
        showStep({ ...step, result, changed: true });
      }
    } catch (error) {
      setCommitError(toFormError(error, []).message);
    } finally {
      sending.current = false;
      setBusy(false);
    }
  }

  const stepNumber = step.kind === "choose" ? 1 : step.kind === "check" ? 2 : 3;
  const heading = stepHeading(
    stepNumber,
    step.kind === "choose" ? "" : step.fileName,
  );

  return (
    <div className="flex flex-col gap-4 md:gap-[22px]">
      <ImportHeader
        orgId={orgId}
        title={heading.title}
        subtitle={heading.subtitle}
        actions={
          step.kind === "check" ? (
            <ImportCheckActions
              orgId={orgId}
              hasErrors={step.result.summary.rowsWithErrors > 0}
              onRestart={restart}
            />
          ) : undefined
        }
      />
      <ImportStepper step={stepNumber} />
      {step.kind === "choose" && (
        <ImportChooseStep
          orgId={orgId}
          file={file}
          problem={problem}
          busy={busy}
          onChoose={choose}
          onCheck={() => void check()}
        />
      )}
      {step.kind === "check" && (
        <ImportCheckStep
          result={step.result}
          changed={step.changed}
          commitError={commitError}
          busy={busy}
          onCommit={() => void commit()}
          onRestart={restart}
        />
      )}
      {step.kind === "done" && (
        <ImportDoneStep
          orgId={orgId}
          fileName={step.fileName}
          organizationName={organizationName}
          summary={step.result.summary}
          onRestart={restart}
        />
      )}
    </div>
  );
}
