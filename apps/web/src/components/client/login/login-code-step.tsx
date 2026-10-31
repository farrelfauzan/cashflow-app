"use client";

import { ChevronLeft } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { Controller } from "react-hook-form";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { useApiForm } from "@/components/client/form/use-api-form";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { toFormError } from "@/lib/api/form-error";
import {
  useRequestLoginCode,
  useVerifyLoginCode,
} from "@/lib/api/generated/auth/auth";
import { loginCodeDigits } from "@/lib/auth/login-code";
import { clearQueryCache } from "@/lib/query/query-client";

import {
  CODE_RESENT_MESSAGE,
  LOGIN_CODE_API_FIELDS,
  loginCodeSchema,
} from "./utils";

type LoginCodeStepProps = {
  // As the user typed it; the API normalizes it.
  phone: string;
  redirectTo: string;
  onChangePhone: () => void;
};

// Step 2: verify the code. The text never says whether the number is registered. On success
// the API sets the HttpOnly cf_session cookie; the page then loads the destination fresh, so
// nothing cached from before the login survives.
export function LoginCodeStep({
  phone,
  redirectTo,
  onChangePhone,
}: LoginCodeStepProps) {
  const id = useId();
  const inputId = `${id}-code`;
  const helpId = `${id}-code-help`;
  const errorId = `${id}-code-error`;
  const { form, showApiError, alert } = useApiForm({
    schema: loginCodeSchema,
    defaultValues: { code: "" },
    apiFields: LOGIN_CODE_API_FIELDS,
  });
  const { clearErrors, setError, setFocus, setValue } = form;
  const fieldError = form.formState.errors.code?.message;
  const [status, setStatus] = useState("");
  const verify = useVerifyLoginCode();
  const resend = useRequestLoginCode();

  // After a successful verify the page is navigating away; keep everything disabled.
  const verifying = verify.isPending || verify.isSuccess;
  const busy = verifying || resend.isPending;
  const typedPhone = phone.trim();

  useEffect(() => {
    setFocus("code");
  }, [setFocus]);

  // Any submit, checked or not, clears "Kode baru sudah dikirim.".
  const submit = form.handleSubmit(
    (input) => {
      if (busy) return;
      setStatus("");
      verify.mutate(
        { data: { phone: typedPhone, code: input.code } },
        {
          onSuccess: () => {
            clearQueryCache();
            window.location.assign(redirectTo);
          },
          onError: (error) => {
            showApiError(error);
            // A wrong code is selected, so typing the next one replaces it.
            if (form.getFieldState("code").error !== undefined) {
              setFocus("code", { shouldSelect: true });
            }
          },
        },
      );
    },
    () => setStatus(""),
  );

  function handleResend() {
    if (busy) return;

    clearErrors();
    setStatus("");
    resend.mutate(
      { data: { phone: typedPhone } },
      {
        onSuccess: () => {
          setValue("code", "");
          setStatus(CODE_RESENT_MESSAGE);
          setFocus("code");
        },
        // This step has no phone field, so every error goes to the alert.
        onError: (error) =>
          setError("root.server", {
            type: "server",
            message: toFormError(error, []).message,
          }),
      },
    );
  }

  return (
    <form noValidate onSubmit={submit} className="flex flex-col gap-6">
      <Button
        type="button"
        variant="link"
        onClick={onChangePhone}
        disabled={verifying}
        className="-mt-3 -mb-2 -ml-1 gap-1.5 self-start px-1 text-base font-semibold no-underline hover:underline has-[>svg]:px-1 md:text-[17px]"
      >
        <ChevronLeft aria-hidden="true" className="size-5" strokeWidth={2.2} />
        Ganti nomor
      </Button>

      <div className="flex flex-col gap-1.5">
        <h1 className="text-[28px] font-bold tracking-[-0.01em] md:text-[32px]">
          Masukkan kode
        </h1>
        <p className="leading-normal text-muted-foreground md:text-lg">
          Jika nomor{" "}
          <b className="font-bold whitespace-nowrap text-foreground">
            {typedPhone}
          </b>{" "}
          terdaftar, kode 6 angka sudah dikirim ke{" "}
          <span className="whitespace-nowrap">WhatsApp-nya.</span>
        </p>
      </div>

      {alert !== undefined && <FormAlert message={alert} />}

      <div className="flex flex-col gap-2">
        <Label
          htmlFor={inputId}
          className="text-[15px] leading-normal font-semibold md:text-[17px]"
        >
          Kode dari WhatsApp
        </Label>
        {/* Controlled, so the field keeps digits only, whatever is typed or pasted. */}
        <Controller
          control={form.control}
          name="code"
          render={({ field }) => (
            <Input
              ref={field.ref}
              id={inputId}
              name={field.name}
              type="text"
              inputMode="numeric"
              autoComplete="one-time-code"
              placeholder="6 angka"
              value={field.value}
              onChange={(event) => {
                field.onChange(loginCodeDigits(event.target.value));
                clearErrors("code");
              }}
              onBlur={field.onBlur}
              aria-invalid={fieldError !== undefined}
              aria-describedby={
                fieldError !== undefined ? `${errorId} ${helpId}` : helpId
              }
              className="h-16 rounded-xl border-[1.5px] bg-card px-4 text-center text-[28px] font-semibold tracking-[0.35em] placeholder:tracking-normal aria-invalid:border-2 md:h-[68px] md:px-[18px] md:text-[30px]"
            />
          )}
        />
        {fieldError !== undefined && (
          <FieldError id={errorId} message={fieldError} />
        )}
        <p
          id={helpId}
          className="text-sm leading-[1.45] text-muted-foreground md:text-base"
        >
          Kode berlaku 5 menit.
        </p>
      </div>

      <Button
        type="submit"
        disabled={busy}
        className="h-13 w-full rounded-xl text-[17px] font-semibold md:h-14 md:text-lg"
      >
        {verifying ? "Memeriksa kode…" : "Masuk"}
      </Button>

      <div className="-mt-2 flex flex-col items-center gap-1">
        <Button
          type="button"
          variant="link"
          onClick={handleResend}
          disabled={busy}
          className="px-3 text-base font-semibold no-underline hover:underline md:text-[17px]"
        >
          {resend.isPending ? "Mengirim ulang…" : "Kirim ulang kode"}
        </Button>
        <p
          role="status"
          aria-live="polite"
          className="text-center text-sm text-muted-foreground md:text-base"
        >
          {status}
        </p>
      </div>
    </form>
  );
}
