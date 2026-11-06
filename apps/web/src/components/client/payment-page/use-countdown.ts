"use client";

import { useEffect, useEffectEvent, useState } from "react";

import type { Countdown } from "./type";
import { remainingSeconds } from "./utils";

// Ticks every second towards `expiresAt` by the device's clock. When it reaches zero it calls
// `onZero` once per expiry, so the page asks the API instead of deciding by itself that the QR
// expired.
export function useCountdown(expiresAt: string, onZero: () => void): Countdown {
  const [now, setNow] = useState(() => Date.now());
  const reachZero = useEffectEvent(onZero);

  useEffect(() => {
    // Already at zero when it starts (the tenant came back to the QRIS tab): the poll has run.
    let fired = remainingSeconds(expiresAt, Date.now()) === 0;
    const timer = window.setInterval(() => {
      const tick = Date.now();
      setNow(tick);
      if (!fired && remainingSeconds(expiresAt, tick) === 0) {
        fired = true;
        reachZero();
      }
    }, 1000);
    return () => window.clearInterval(timer);
  }, [expiresAt]);

  return { now, seconds: remainingSeconds(expiresAt, now) };
}
