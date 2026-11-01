"use client";

import { useState, type ReactNode } from "react";

import { LoginCodeStep } from "./login-code-step";
import { LoginPhoneStep } from "./login-phone-step";
import type { LoginStep } from "./type";

type LoginFlowProps = {
  // Checked on the server with postLoginPath: a safe staff path, never login or /t/...
  redirectTo: string;
  // Server-rendered; on phones only the phone step shows it, on iPad both steps do.
  tenantNote: ReactNode;
};

// The two login steps on one route. The step lives in client state, so going back keeps the
// number as typed and no URL ever holds the phone number.
export function LoginFlow({ redirectTo, tenantNote }: LoginFlowProps) {
  const [step, setStep] = useState<LoginStep>("phone");
  const [phone, setPhone] = useState("");
  const [returnedToPhone, setReturnedToPhone] = useState(false);

  return (
    <div className="flex flex-1 flex-col gap-6 md:w-[460px] md:flex-none">
      <div className="flex flex-1 flex-col md:flex-none md:rounded-[20px] md:border md:bg-card md:px-10 md:py-9 md:shadow-sm">
        {step === "phone" ? (
          <LoginPhoneStep
            phone={phone}
            focusOnMount={returnedToPhone}
            onSent={(sent) => {
              setPhone(sent);
              setStep("code");
            }}
          />
        ) : (
          <LoginCodeStep
            phone={phone}
            redirectTo={redirectTo}
            onChangePhone={() => {
              setReturnedToPhone(true);
              setStep("phone");
            }}
          />
        )}
      </div>
      <div className={step === "code" ? "hidden md:block" : undefined}>
        {tenantNote}
      </div>
    </div>
  );
}
