import { fileURLToPath } from "node:url";

import { defineConfig } from "vitest/config";

// Run unit tests in a timezone far from Jakarta, so a formatter that leans on the machine's
// timezone fails here instead of in production. Set before Vitest starts its workers, which
// inherit it; src/lib/format/date.test.ts checks that it really took effect.
process.env.TZ = "America/New_York";

export default defineConfig({
  resolve: {
    // The tsconfig "@/*" path. "#api-environment" resolves from package.json "imports" with
    // the default condition, so tests get the browser environment.
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
});
