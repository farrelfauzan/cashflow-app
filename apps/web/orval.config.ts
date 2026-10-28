import { defineConfig } from "orval";

// Generates the API client from the contract: types, plain request functions (for server
// components) and TanStack Query hooks (for client components), one folder per OpenAPI tag.
// Everything under src/lib/api/generated/ is output: never edit it, run `pnpm gen`.
export default defineConfig({
  api: {
    input: {
      target: "../../api/openapi.yaml",
    },
    output: {
      mode: "tags-split",
      target: "src/lib/api/generated/endpoints.ts",
      schemas: "src/lib/api/generated/model",
      client: "react-query",
      httpClient: "fetch",
      // Removes files of deleted tags or schemas, so the output always matches the spec.
      clean: true,
      override: {
        // Every request goes through the one hand-written mutator.
        mutator: {
          path: "src/lib/api/client.ts",
          name: "apiFetch",
        },
        fetch: {
          // The mutator throws ApiError on any non-2xx, so functions return the success body
          // alone instead of a {data, status, headers} envelope.
          includeHttpResponseReturnType: false,
        },
        query: {
          version: 5,
          signal: true,
          shouldExportKeys: true,
          // Query keys start with the organization (src/lib/api/query-key.ts).
          queryKey: {
            path: "src/lib/api/query-key.ts",
            name: "apiQueryKey",
          },
        },
      },
    },
  },
});
