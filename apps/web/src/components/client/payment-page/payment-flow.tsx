"use client";

import { useState } from "react";

import type { PaymentPage } from "@/lib/api/generated/model";
import { formatRupiah } from "@/lib/format/rupiah";

import { BankTransferMethod } from "./bank-transfer-method";
import { ChosenAmountCard } from "./chosen-amount-card";
import { PartialEntry } from "./partial-entry";
import { PayChoiceView } from "./pay-choice-view";
import { PaymentFullView } from "./payment-full-view";
import { PaymentMethods } from "./payment-methods";
import { PaymentPageFooter } from "./payment-page-footer";
import { PaymentPageHeader } from "./payment-page-header";
import { PromiseForm } from "./promise-form";
import { PromiseRecordedView } from "./promise-recorded-view";
import { QrisAnnouncer } from "./qris-announcer";
import type {
  OpenPaymentPageView,
  PayChoice,
  PayFlowMode,
  QrisCard,
  QrisPayment,
} from "./type";
import {
  PARTIAL_TITLE,
  PROMISE_TITLE,
  backFromPromise,
  canChangePartialAmount,
  changePartialAmount,
  choiceMode,
  choicePrimaryLabel,
  chooseFromEntry,
  chosenAmountView,
  editPromise,
  flowModeAfterCard,
  initialFlowMode,
  nextFromChoice,
  offeredChoice,
  payChoices,
  promiseRecordedView,
  promiseSaved,
  qrisAnnouncement,
  showsPayBack,
} from "./utils";

type PaymentFlowProps = {
  view: OpenPaymentPageView;
  token: string;
  // The QRIS state, owned by QrisPaymentFlow; null when QRIS is not set up (bank transfer only).
  qris: QrisPayment | null;
  refetchPage: () => Promise<PaymentPage | undefined>;
  onLinkGone: () => void;
};

// The open page, in views that share one QRIS state: Bayar-Pilih (the choice), the whole total,
// "Bayar sebagian", the Janji screen, the chosen amount with its QR, and Janji tercatat. Every
// move between them is a pure transition of utils.ts.
export function PaymentFlow({
  view,
  token,
  qris,
  refetchPage,
  onLinkGone,
}: PaymentFlowProps) {
  const [mode, setMode] = useState<PayFlowMode>(() => initialFlowMode(view));
  const card = qris === null ? null : qris.model.card;
  const offer = view.qris;

  // Follows the card from render to render (React's "previous props" pattern): a QR found on
  // load skips the choice, and a paid QR of a chosen amount leaves the chosen view.
  const [previousCard, setPreviousCard] = useState<QrisCard | null>(card);
  if (qris !== null && card !== null && previousCard !== card) {
    setPreviousCard(card);
    if (previousCard !== null) {
      const next = flowModeAfterCard(mode, previousCard, qris.model);
      if (next !== mode) setMode(next);
    }
  }

  const partialHeader = { ...view.header, title: PARTIAL_TITLE };

  function choiceBody(chosen: PayChoice) {
    const choices = payChoices(view);
    const selected = offeredChoice(chosen, choices);
    const active = view.activePromise;
    const promiseOffer = view.promise;
    // A QR found on load goes straight to paying, so nothing else starts before the read ends.
    const resuming = card !== null && card.kind === "resuming";
    return (
      <PayChoiceView
        view={view}
        choices={choices}
        selected={selected}
        onSelect={(choice) => setMode(choiceMode(choice))}
        primaryLabel={choicePrimaryLabel(selected, view.total)}
        onContinue={() => setMode(nextFromChoice(selected, view))}
        resuming={resuming}
        activePromise={active}
        onEditPromise={
          active !== null && promiseOffer !== null && !resuming
            ? () =>
                setMode(
                  editPromise(
                    active.promise,
                    promiseOffer,
                    choiceMode(selected),
                  ),
                )
            : null
        }
      />
    );
  }

  function body() {
    switch (mode.kind) {
      case "choice":
        return choiceBody(mode.selected);
      case "pay":
        return (
          <PaymentFullView
            view={view}
            methods={
              qris !== null && offer !== null ? (
                <PaymentMethods
                  token={token}
                  offer={offer}
                  qris={qris}
                  chosen={null}
                  transfer={view.transfer}
                  noTransferText={view.noTransferText}
                />
              ) : (
                <BankTransferMethod
                  transfer={view.transfer}
                  noTransferText={view.noTransferText}
                />
              )
            }
            onBack={
              showsPayBack(view, card) ? () => setMode(choiceMode("all")) : null
            }
          />
        );
      case "entry": {
        if (qris === null || offer === null) return choiceBody("all");
        return (
          <PartialEntry
            header={partialHeader}
            offer={offer}
            token={token}
            text={mode.text}
            promiseOffered={view.promise !== null}
            onTextChange={(text) => setMode({ kind: "entry", text })}
            onChoose={(preview) => {
              const next = chooseFromEntry(mode, preview, view.promise);
              setMode(next);
              if (next.kind === "chosen") qris.start(false, next.amount);
            }}
            onBack={() => setMode(choiceMode("partial"))}
            onLinkGone={onLinkGone}
          />
        );
      }
      case "promise": {
        if (view.promise === null) return choiceBody("all");
        return (
          <PromiseForm
            header={{ ...view.header, title: PROMISE_TITLE }}
            offer={view.promise}
            paying={mode.paying}
            editing={mode.editing}
            active={view.activePromise?.promise ?? null}
            initial={mode.draft}
            token={token}
            refetchPage={refetchPage}
            onLinkGone={onLinkGone}
            onSaved={(saved) => {
              const next = promiseSaved(mode, saved);
              setMode(next);
              if (next.kind === "chosen" && qris !== null) {
                qris.start(false, next.amount);
              }
            }}
            onBack={() => setMode(backFromPromise(mode))}
          />
        );
      }
      case "chosen": {
        if (qris === null || offer === null || card === null) {
          return choiceBody("all");
        }
        return (
          <>
            <PaymentPageHeader header={partialHeader} />
            {card.kind !== "checking" && (
              <ChosenAmountCard
                {...chosenAmountView(mode, card)}
                onChange={
                  canChangePartialAmount(card)
                    ? () => setMode(changePartialAmount(mode))
                    : null
                }
              />
            )}
            <PaymentMethods
              token={token}
              offer={offer}
              qris={qris}
              chosen={mode.amount}
              transfer={view.transfer}
              noTransferText={view.noTransferText}
            />
            <PaymentPageFooter />
          </>
        );
      }
      case "recorded": {
        const promiseOffer = view.promise;
        return (
          <PromiseRecordedView
            header={view.header}
            view={promiseRecordedView(view, mode)}
            onEdit={
              promiseOffer === null
                ? null
                : () => setMode(editPromise(mode.promise, promiseOffer, mode))
            }
          />
        );
      }
    }
  }

  return (
    <>
      {card !== null && <QrisAnnouncer text={qrisAnnouncement(card)} />}
      {body()}
    </>
  );
}
