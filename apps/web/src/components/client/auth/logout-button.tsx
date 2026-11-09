"use client";

import { useMutation } from "@tanstack/react-query";
import { LogOut } from "lucide-react";

import { FormAlert } from "@/components/client/form/form-alert";
import { Button } from "@/components/ui/button";
import { toFormError } from "@/lib/api/form-error";
import { signOut } from "@/lib/auth/sign-out";

import type { LogoutButtonVariant } from "./type";
import { LOGOUT_BUTTON_CLASS } from "./utils";

type LogoutButtonProps = {
  variant?: LogoutButtonVariant;
};

// "Keluar": ends the session and goes to /login. A secondary action, never the screen's
// primary one.
export function LogoutButton({ variant = "outline" }: LogoutButtonProps) {
  const logout = useMutation({ mutationFn: signOut });
  // After success the page is navigating to /login; keep the button disabled.
  const busy = logout.isPending || logout.isSuccess;

  return (
    <div className="flex flex-col gap-3">
      {logout.isError && (
        <FormAlert message={toFormError(logout.error, []).message} />
      )}
      <Button
        type="button"
        variant={variant === "outline" ? "outline" : "ghost"}
        className={LOGOUT_BUTTON_CLASS[variant]}
        disabled={busy}
        onClick={() => logout.mutate()}
      >
        <LogOut aria-hidden="true" className="size-5" />
        <span className="rail:sr-only">{busy ? "Keluar…" : "Keluar"}</span>
      </Button>
    </div>
  );
}
