import { apiInternalUrl } from "./lib/api/api-internal-url";

// Startup checks of the Node.js server. A missing or malformed API_INTERNAL_URL stops the
// process here instead of failing on the first page that calls the API. An error thrown from
// register() is only logged by Next.js and the server keeps running, hence the explicit exit.
try {
  apiInternalUrl();
} catch (error) {
  console.error(error instanceof Error ? error.message : error);
  process.exit(1);
}
