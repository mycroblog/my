// SPDX-License-Identifier: MIT
import { idempotencyKeyHeader } from "./idempotency.js";
import { isAbort } from "./oauth.js";
import { NetworkError } from "./problem.js";
import type { TokenSource } from "./token.js";

/**
 * Decides how a request answered with 429 or 503 is sent again. Only requests that are safe to repeat are retried:
 * GET, HEAD, and requests with an Idempotency-Key.
 */
export type RetryPolicy = {
  /** Turns the retries off: the 429 or 503 becomes a ProblemError at once. */
  disabled?: boolean;
  /** How many times a request is sent again; 3 by default. */
  maxRetries?: number;
  /**
   * The longest the client waits before a retry, in milliseconds; one minute by default. When Retry-After asks for
   * longer, the answer becomes a ProblemError instead.
   */
  maxWait?: number;
};

const defaultMaxRetries = 3;
const defaultMaxWait = 60_000;

/** Waits for ms milliseconds, or rejects with the reason of signal when it aborts first. */
export type Sleep = (ms: number, signal: AbortSignal) => Promise<void>;

export type TransportOptions = {
  fetch: (input: Request) => Promise<Response>;
  tokenSource: TokenSource | undefined;
  retry: RetryPolicy | undefined;
  sleep?: Sleep;
};

/**
 * The fetch of the generated client. For each request it adds the access token; after a 401 it refreshes the token
 * once and sends the request again; after a 429 or 503 it waits and retries the requests that are safe to repeat;
 * and it turns a failure to get an answer into a NetworkError. Every middleware of the client sees only the last
 * answer.
 */
export function createTransport({ fetch, tokenSource, retry, sleep = sleepSignal }: TransportOptions): (request: Request) => Promise<Response> {
  const maxRetries = retry?.disabled === true ? 0 : (retry?.maxRetries ?? defaultMaxRetries);
  const maxWait = retry?.maxWait ?? defaultMaxWait;
  return async (request) => {
    const method = request.method.toUpperCase();
    const retryable = method === "GET" || method === "HEAD" || request.headers.has(idempotencyKeyHeader);
    // An unsent copy to send again: a body can be read only once.
    const copy = request.clone();
    let refreshed = false;
    let renewed: string | null = null;
    let retries = 0;
    for (let attempt = 0; ; attempt++) {
      const r = attempt === 0 ? request : copy.clone();
      let token: string | null = null;
      if (tokenSource !== undefined) {
        token = renewed ?? (await tokenSource.getAccessToken());
        renewed = null;
        if (token !== null) r.headers.set("Authorization", `Bearer ${token}`);
      }
      let response: Response;
      try {
        response = await fetch(r);
      } catch (error) {
        if (isAbort(error) || request.signal.aborted) throw error;
        throw new NetworkError(method, request.url, error);
      }
      if (response.status === 401 && token !== null && !refreshed && tokenSource?.refresh !== undefined) {
        refreshed = true;
        renewed = await tokenSource.refresh(token);
        if (renewed === null) return response;
        await discard(response);
        continue;
      }
      if ((response.status === 429 || response.status === 503) && retryable && retries < maxRetries) {
        const wait = retryDelay(response.headers.get("Retry-After"), retries, maxWait, Date.now());
        if (wait === null) return response;
        await discard(response);
        retries++;
        await sleep(wait, request.signal);
        continue;
      }
      return response;
    }
  };
}

/** Cancels the body of an answer that is not returned. */
async function discard(response: Response): Promise<void> {
  try {
    await response.body?.cancel();
  } catch {
    // Nothing to release.
  }
}

/**
 * How long to wait, in milliseconds, before retry number n+1 (n from 0): what Retry-After asks for (seconds or an
 * HTTP date), or without it an exponential backoff from one second with jitter. It is null when the wait is longer
 * than limit.
 */
function retryDelay(retryAfter: string | null, n: number, limit: number, now: number): number | null {
  if (retryAfter !== null && retryAfter !== "") {
    let ms = 1000;
    if (/^\d+$/.test(retryAfter.trim())) {
      ms = Number(retryAfter.trim()) * 1000;
    } else {
      const at = Date.parse(retryAfter);
      if (!Number.isNaN(at)) ms = Math.max(at - now, 0);
    }
    return ms <= limit ? ms : null;
  }
  const base = 1000 * 2 ** Math.min(n, 10);
  return Math.min(base + Math.random() * (base / 2), limit);
}

/** Waits for ms, or rejects with the reason of signal when it aborts first. */
function sleepSignal(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(signal.reason as Error);
      return;
    }
    const onAbort = () => {
      clearTimeout(timer);
      reject(signal.reason as Error);
    };
    const timer = setTimeout(() => {
      signal.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    signal.addEventListener("abort", onAbort, { once: true });
  });
}
