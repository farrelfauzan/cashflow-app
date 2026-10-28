"use client";

import { useMutation } from "@tanstack/react-query";

import { FormAlert } from "@/components/client/form/form-alert";
import { Button } from "@/components/ui/button";
import { toFormError } from "@/lib/api/form-error";
import { signOut } from "@/lib/auth/sign-out";

// "Keluar": ends the session and goes to /login. A secondary action, never the screen's
// primary one.
export function LogoutButton() {
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
        variant="outline"
        disabled={busy}
        onClick={() => logout.mutate()}
      >
        {busy ? "Keluar…" : "Keluar"}
      </Button>
    </div>
  );
}
