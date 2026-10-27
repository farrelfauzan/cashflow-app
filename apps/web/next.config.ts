import type { NextConfig } from "next";
import { PHASE_DEVELOPMENT_SERVER } from "next/constants";

const DEFAULT_API_DEV_ORIGIN = "http://localhost:8080";

// The /api rewrite to the Go API. Behind Caddy (staging, production) Caddy sends /api/* to the
// API and this rewrite does not exist. It exists:
// - in `next dev`, to API_DEV_ORIGIN or http://localhost:8080;
// - in a production build only when API_DEV_ORIGIN is set at build time. Playwright runs
//   `next build && next start` and sets it, because there is no Caddy in front of the app in
//   e2e runs. Never set it for a build that is deployed.
// The API's routes already start with /api, so the path is not changed. The browser's Origin
// (http://localhost:3000) and the API's Set-Cookie pass through unchanged.
function apiRewriteOrigin(phase: string): string | undefined {
  const configured = process.env.API_DEV_ORIGIN?.trim();
  if (phase === PHASE_DEVELOPMENT_SERVER) {
    return configured || DEFAULT_API_DEV_ORIGIN;
  }
  return configured || undefined;
}

export default function config(phase: string): NextConfig {
  const apiOrigin = apiRewriteOrigin(phase);

  return {
    // create-next-app 16.4 defaults. Cache Components makes data fetching dynamic by default
    // and caching opt-in with `use cache`; Next.js says both become the default in the next
    // major and these options will then be removed.
    cacheComponents: true,
    partialPrefetching: true,
    // Tailwind 4 through its Turbopack loader (create-next-app's default; no PostCSS config).
    turbopack: {
      rules: {
        "*.css": {
          loaders: ["@tailwindcss/turbopack"],
          as: "*.css",
        },
      },
    },
    async rewrites() {
      if (apiOrigin === undefined) return [];
      return [
        {
          source: "/api/:path*",
          destination: `${new URL(apiOrigin).origin}/api/:path*`,
        },
      ];
    },
  };
}
