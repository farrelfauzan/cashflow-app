import { test as setup } from "@playwright/test";

import { signIn } from "./support/login";
import { SETUP_STAFF } from "./support/staff";

// Runs once per `pnpm e2e`, before the ipad and phone projects, which share the saved states.
// One test per member, so each gets its own cookie jar.
for (const staff of SETUP_STAFF) {
  setup(`sign in ${staff.name}`, async ({ request, playwright, baseURL }) => {
    if (!baseURL) throw new Error("baseURL is not configured.");
    const result = await signIn({
      request,
      newRequestContext: (storageState) =>
        playwright.request.newContext({ baseURL, storageState }),
      baseURL,
      staff,
    });
    setup.info().annotations.push({ type: "session", description: result });
  });
}
