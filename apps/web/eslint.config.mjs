import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";
import prettier from "eslint-config-prettier/flat";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  // Last, so formatting is left to Prettier.
  prettier,
  globalIgnores([
    // Default ignores of eslint-config-next, restated because globalIgnores replaces them.
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
    // Generated code and test output.
    "src/lib/api/generated/**",
    "playwright-report/**",
    "test-results/**",
    "blob-report/**",
    "coverage/**",
  ]),
]);

export default eslintConfig;
