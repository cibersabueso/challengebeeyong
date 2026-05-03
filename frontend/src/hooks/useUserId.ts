import { useEffect, useState } from "react";
import { isUuidV4, newUuidV4 } from "../lib/uuid";

const STORAGE_KEY = "challengebeeyong:user_id";

function readOrCreate(): string {
  try {
    const existing = localStorage.getItem(STORAGE_KEY);
    if (existing && isUuidV4(existing)) return existing;
  } catch {
    // localStorage may be unavailable in private mode; fall through to ephemeral id.
  }
  const fresh = newUuidV4();
  try {
    localStorage.setItem(STORAGE_KEY, fresh);
  } catch {
    // ignore: ephemeral session
  }
  return fresh;
}

export function useUserId(): string {
  const [userId] = useState<string>(() => readOrCreate());
  useEffect(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (!stored || !isUuidV4(stored)) {
        localStorage.setItem(STORAGE_KEY, userId);
      }
    } catch {
      // ignore
    }
  }, [userId]);
  return userId;
}
