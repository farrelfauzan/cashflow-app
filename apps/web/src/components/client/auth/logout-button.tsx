"use client";

import { useMutation } from "@tanstack/react-query";
import { LogOut } from "lucide-react";

import { FormAlert } from "@/components/client/form/form-alert";
import { Button } from "@/components/ui/button";
import { toFormError } from "@/lib/api/form-error";
import { signOut } from "@/lib/auth/sign-out";

type LogoutButtonProps = {
  // "outline" on the organization picker, "menu" at the foot of the side menu.
  variant?: "outline" | "menu";
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
        variant={variant === "menu" ? "ghost" : "outline"}
        className={
          variant === "menu"
            ? "justify-start px-2 font-semibold text-muted-foreground"
            : "font-semibold"
        }
        disabled={busy}
        onClick={() => logout.mutate()}
      >
        <LogOut aria-hidden="true" className="size-5" />
        {busy ? "Keluar…" : "Keluar"}
      </Button>
    </div>
  );
}
