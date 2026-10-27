import type { NextConfig } from "next";

const nextConfig: NextConfig = {
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
};

export default nextConfig;
