"use client";

import { useEffect, useId, useRef, useState, type FormEvent } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { toFormError } from "@/lib/api/form-error";
import { useRequestLoginCode } from "@/lib/api/generated/auth/auth";

const EMPTY_PHONE_MESSAGE = "Isi nomor WhatsApp Anda.";

type LoginPhoneStepProps = {
  phone: string;
  onPhoneChange: (phone: string) => void;
  // True when the user came back from the code step with "Ganti nomor".
  focusOnMount: boolean;
  onSent: () => void;
};

// Step 1: ask for a login code. The API answers 202 whether or not the number is registered,
// so this step learns nothing about the number and simply moves on.
export function LoginPhoneStep({
  phone,
  onPhoneChange,
  focusOnMount,
  onSent,
}: LoginPhoneStepProps) {
  const id = useId();
  const inputId = `${id}-phone`;
  const hintId = `${id}-phone-hint`;
  const errorId = `${id}-phone-error`;
  const inputRef = useRef<HTMLInputElement>(null);
  const [fieldError, setFieldError] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const requestCode = useRequestLoginCode();

  useEffect(() => {
    if (focusOnMount) inputRef.current?.focus();
  }, [focusOnMount]);

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (requestCode.isPending) return;

    const value = phone.trim();
    setFormError(null);
    if (value === "") {
      setFieldError(EMPTY_PHONE_MESSAGE);
      inputRef.current?.focus();
      return;
    }
    setFieldError(null);

    requestCode.mutate(
      { data: { phone: value } },
      {
        onSuccess: onSent,
        onError: (error) => {
          const { field, message } = toFormError(error, ["phone"]);
          if (field === "phone") {
            setFieldError(message);
            inputRef.current?.focus();
          } else {
            setFormError(message);
          }
        },
      },
    );
  }

  return (
    <form noValidate onSubmit={handleSubmit} className="flex flex-col gap-6">
      <div className="flex flex-col gap-1.5">
        <h1 className="text-[28px] font-bold tracking-[-0.01em] md:text-[32px]">
          Masuk
        </h1>
        <p className="leading-normal text-muted-foreground md:text-lg">
          Untuk Supervisor, admin, dan karyawan.
        </p>
      </div>

      {formError !== null && <FormAlert message={formError} />}

      <div className="flex flex-col gap-2">
        <Label
          htmlFor={inputId}
          className="text-[15px] leading-normal font-semibold md:text-[17px]"
        >
          Nomor WhatsApp
        </Label>
        <Input
          ref={inputRef}
          id={inputId}
          name="phone"
          type="tel"
          inputMode="tel"
          autoComplete="tel"
          placeholder="0812-3456-7890"
          value={phone}
          onChange={(event) => {
            onPhoneChange(event.target.value);
            setFieldError(null);
          }}
          aria-invalid={fieldError !== null}
          aria-describedby={
            fieldError !== null ? `${errorId} ${hintId}` : hintId
          }
          className="h-14 rounded-xl border-[1.5px] bg-card px-4 text-lg aria-invalid:border-2 md:h-15 md:px-[18px] md:text-xl"
        />
        {fieldError !== null && (
          <FieldError id={errorId} message={fieldError} />
        )}
        <p
          id={hintId}
          className="text-sm leading-[1.45] text-muted-foreground md:text-base"
        >
          Kami kirim kode masuk 6 angka lewat WhatsApp.
        </p>
      </div>

      <Button
        type="submit"
        disabled={requestCode.isPending}
        className="h-13 w-full rounded-xl text-[17px] font-semibold md:h-14 md:text-lg"
      >
        {requestCode.isPending ? "Mengirim kode…" : "Kirim kode"}
      </Button>
    </form>
  );
}
