import { ApiError, type ApiErrorPayload } from "../lib/errors";

const BASE_URL = "/api/v1";

export interface RequestOptions {
  method?: "GET" | "POST" | "DELETE";
  userId: string;
  idempotencyKey?: string;
  body?: unknown;
}

export async function apiRequest<T>(path: string, opts: RequestOptions): Promise<T> {
  const headers: Record<string, string> = {
    "X-User-Id": opts.userId,
  };
  if (opts.body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  if (opts.idempotencyKey) {
    headers["Idempotency-Key"] = opts.idempotencyKey;
  }

  let res: Response;
  try {
    res = await fetch(BASE_URL + path, {
      method: opts.method ?? "GET",
      headers,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
    });
  } catch (e) {
    throw new ApiError(0, {
      code: "NETWORK_ERROR",
      message: e instanceof Error ? e.message : "network error",
    });
  }

  if (res.status === 204 || res.headers.get("content-length") === "0") {
    return undefined as T;
  }

  const text = await res.text();
  let parsed: unknown = null;
  if (text.length > 0) {
    try {
      parsed = JSON.parse(text);
    } catch {
      throw new ApiError(res.status, {
        code: "INTERNAL_ERROR",
        message: `non-json response: ${text.slice(0, 120)}`,
      });
    }
  }

  if (!res.ok) {
    const payload = (parsed ?? {}) as Partial<ApiErrorPayload>;
    throw new ApiError(res.status, {
      code: (payload.code ?? "INTERNAL_ERROR") as ApiErrorPayload["code"],
      message: payload.message ?? `request failed with status ${res.status}`,
      details: payload.details,
    });
  }

  return parsed as T;
}
