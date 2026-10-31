"use client";

import { useEffect, useId } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { useApiForm } from "@/components/client/form/use-api-form";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useRequestLoginCode } from "@/lib/api/generated/auth/auth";

import { LOGIN_PHONE_API_FIELDS, loginPhoneSchema } from "./utils";

type LoginPhoneStepProps = {
  // The number as last sent, kept when the user comes back from the code step.
  phone: string;
  // True when the user came back from the code step with "Ganti nomor".
  focusOnMount: boolean;
  // The number as typed; the code step shows it and sends it again.
  onSent: (phone: string) => void;
};

// Step 1: ask for a login code. The API answers 202 whether or not the number is registered,
// so this step learns nothing about the number and simply moves on.
export function LoginPhoneStep({
  phone,
  focusOnMount,
  onSent,
}: LoginPhoneStepProps) {
  const id = useId();
  const inputId = `${id}-phone`;
  const hintId = `${id}-phone-hint`;
  const errorId = `${id}-phone-error`;
  const { form, field, showApiError, alert } = useApiForm({
    schema: loginPhoneSchema,
    defaultValues: { phone },
    apiFields: LOGIN_PHONE_API_FIELDS,
  });
  const { setFocus } = form;
  const fieldError = form.formState.errors.phone?.message;
  const requestCode = useRequestLoginCode();

  useEffect(() => {
    if (focusOnMount) setFocus("phone");
  }, [focusOnMount, setFocus]);

  const submit = form.handleSubmit((input) => {
    if (requestCode.isPending) return;
    requestCode.mutate(
      { data: input },
      {
        onSuccess: () => onSent(form.getValues("phone")),
        onError: showApiError,
      },
    );
  });

  return (
    <form noValidate onSubmit={submit} className="flex flex-col gap-6">
      <div className="flex flex-col gap-1.5">
        <h1 className="text-[28px] font-bold tracking-[-0.01em] md:text-[32px]">
          Masuk
        </h1>
        <p className="leading-normal text-muted-foreground md:text-lg">
          Untuk Supervisor, admin, dan karyawan.
        </p>
      </div>

      {alert !== undefined && <FormAlert message={alert} />}

      <div className="flex flex-col gap-2">
        <Label
          htmlFor={inputId}
          className="text-[15px] leading-normal font-semibold md:text-[17px]"
        >
          Nomor WhatsApp
        </Label>
        <Input
          id={inputId}
          type="tel"
          inputMode="tel"
          autoComplete="tel"
          placeholder="0812-3456-7890"
          {...field("phone")}
          aria-invalid={fieldError !== undefined}
          aria-describedby={
            fieldError !== undefined ? `${errorId} ${hintId}` : hintId
          }
          className="h-14 rounded-xl border-[1.5px] bg-card px-4 text-lg aria-invalid:border-2 md:h-15 md:px-[18px] md:text-xl"
        />
        {fieldError !== undefined && (
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
