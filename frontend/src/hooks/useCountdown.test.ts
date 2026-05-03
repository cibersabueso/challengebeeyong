import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useCountdown, formatCountdown } from "./useCountdown";

describe("formatCountdown", () => {
  it("formats whole minutes and seconds zero-padded", () => {
    expect(formatCountdown(0)).toBe("00:00");
    expect(formatCountdown(5)).toBe("00:05");
    expect(formatCountdown(60)).toBe("01:00");
    expect(formatCountdown(125)).toBe("02:05");
  });
});

describe("useCountdown", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("returns 60 when expires_at is exactly 60 seconds in the future", () => {
    const now = new Date("2026-05-03T00:00:00Z").getTime();
    vi.setSystemTime(now);
    const expiresAt = new Date(now + 60_000).toISOString();

    const { result } = renderHook(() => useCountdown(expiresAt));
    expect(result.current).toBe(60);
  });

  it("decrements as time advances and bottoms out at 0", () => {
    const now = new Date("2026-05-03T00:00:00Z").getTime();
    vi.setSystemTime(now);
    const expiresAt = new Date(now + 10_000).toISOString();

    const { result } = renderHook(() => useCountdown(expiresAt));
    expect(result.current).toBe(10);

    act(() => {
      vi.advanceTimersByTime(5_000);
    });
    expect(result.current).toBeLessThanOrEqual(5);
    expect(result.current).toBeGreaterThanOrEqual(4);

    act(() => {
      vi.advanceTimersByTime(20_000);
    });
    expect(result.current).toBe(0);
  });

  it("never returns a negative value when expires_at is already past", () => {
    const now = new Date("2026-05-03T00:00:00Z").getTime();
    vi.setSystemTime(now);
    const past = new Date(now - 30_000).toISOString();

    const { result } = renderHook(() => useCountdown(past));
    expect(result.current).toBe(0);
  });

  it("returns 0 for invalid timestamps without throwing", () => {
    const { result } = renderHook(() => useCountdown("not-a-date"));
    expect(result.current).toBe(0);
  });
});
