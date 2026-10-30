import { useSyncExternalStore } from "react";

// Nothing to subscribe to: the value only changes from the server snapshot to the client one,
// which React does by itself after hydration.
function subscribe(): () => void {
  return () => {};
}

function clientSnapshot(): boolean {
  return true;
}

function serverSnapshot(): boolean {
  return false;
}

// False while the server renders and while the browser hydrates that HTML, true afterwards and
// for anything first rendered in the browser (a client navigation). A client view that renders
// from the shared query cache reads its data only once this is true, so its first render in
// the browser matches the server's even when another component (the shell's getOrgMe) has
// already filled the cache.
export function useHydrated(): boolean {
  return useSyncExternalStore(subscribe, clientSnapshot, serverSnapshot);
}
