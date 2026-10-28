import { test as setup } from "@playwright/test";

import { signIn } from "./support/login";
import { HENDRA } from "./support/staff";

// Runs once per `pnpm e2e`, before the ipad and phone projects, which share the saved state.
setup(
  "sign in the seeded staff member",
  async ({ request, playwright, baseURL }) => {
    if (!baseURL) throw new Error("baseURL is not configured.");
    const result = await signIn({
      request,
      newRequestContext: (storageState) =>
        playwright.request.newContext({ baseURL, storageState }),
      baseURL,
      staff: HENDRA,
    });
    setup.info().annotations.push({ type: "session", description: result });
  },
);
