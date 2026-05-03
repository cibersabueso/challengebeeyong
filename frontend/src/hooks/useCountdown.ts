import { useEffect, useState } from "react";

const TICK_MS = 250;

export function useCountdown(expiresAtIso: string): number {
  const [secondsLeft, setSecondsLeft] = useState<number>(() => computeSeconds(expiresAtIso));

  useEffect(() => {
    setSecondsLeft(computeSeconds(expiresAtIso));
    const id = window.setInterval(() => {
      setSecondsLeft(computeSeconds(expiresAtIso));
    }, TICK_MS);
    return () => window.clearInterval(id);
  }, [expiresAtIso]);

  return secondsLeft;
}

function computeSeconds(expiresAtIso: string): number {
  const expiresMs = Date.parse(expiresAtIso);
  if (Number.isNaN(expiresMs)) return 0;
  const diff = Math.ceil((expiresMs - Date.now()) / 1000);
  return Math.max(0, diff);
}

export function formatCountdown(seconds: number): string {
  const mm = Math.floor(seconds / 60).toString().padStart(2, "0");
  const ss = (seconds % 60).toString().padStart(2, "0");
  return `${mm}:${ss}`;
}
