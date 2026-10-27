// Runs once when a Next.js server starts, before it handles any request. The Node.js-only checks
// live in their own module, imported only in that runtime (the pattern Next.js documents).
export async function register() {
  if (process.env.NEXT_RUNTIME === "nodejs") {
    await import("./instrumentation-node");
  }
}
