// SPDX-License-Identifier: MIT
// The client of GET /api/v1/streaming (streamEvents): the Server-Sent Events of the user's new home entries,
// notifications, and announcements; and, with `timeline`, of GET /api/v1/streaming/timelines/{timeline}
// (streamTimeline): the new posts of the local or the federated timeline. It reads the body of fetch, since EventSource
// cannot send the Authorization header.
import { apiBasePath, type ClientOptions, parseOrigin } from "./client.js";
import { isAbort, OAuthError } from "./oauth.js";
import { NetworkError, ProblemError, problemFromResponse } from "./problem.js";
import type { components } from "./schema.gen.js";
import { EventStreamParser, type ServerSentEvent } from "./sse.js";
import { parseRetryAfter, type Sleep, sleepSignal } from "./transport.js";

type Status = components["schemas"]["Status"];
type Notification = components["schemas"]["Notification"];
type Announcement = components["schemas"]["Announcement"];

/** A timeline whose new posts GET /streaming/timelines/{timeline} streams. */
export type StreamTimeline = "local" | "federated";

/**
 * An event of the stream, with its data parsed. `unknown` is an event that this version of the library does not know,
 * which a newer server may send: `event` is its name (`message` when it has none) and `data` its data, unparsed.
 */
export type StreamEvent =
  | { type: "update"; status: Status }
  | { type: "list_update"; listId: string; status: Status }
  | { type: "notification"; notification: Notification }
  | { type: "announcement"; announcement: Announcement }
  | { type: "announcement_removed"; id: string }
  | { type: "unknown"; event: string; data: string };

/** How long a stream may stay silent before the client drops it: two of the server's heartbeats, 30 seconds apart. */
export const defaultHeartbeatTimeout = 60_000;

/** Configures runStream: the client's options (but its retry policy), and the stream's. */
export type StreamOptions = Omit<ClientOptions, "retry"> & {
  /** Ends the stream; runStream resolves once it aborts. */
  signal: AbortSignal;
  /**
   * Reads the stream of the local or the federated timeline (GET /streaming/timelines/{timeline}) instead of the
   * user's (GET /streaming). It sends `update` events only, needs the scope read:statuses and a signed-in user, and is a
   * 404 while the instance turns the timeline off, which ends runStream.
   */
  timeline?: StreamTimeline;
  /**
   * Called whenever a stream opens: `reconnected` is false for the first one and true for every later one. The server
   * replays nothing, so after a reconnection read the home timeline and the notifications again for what happened
   * while the stream was closed.
   */
  onOpen?: (reconnected: boolean) => void;
  /** Receives the events in the order they arrive. */
  onEvent: (event: StreamEvent) => void;
  /**
   * Receives every failure that the client recovers from by connecting again (a network error, a 429, a 5xx, a silent
   * stream) and every event whose data it cannot read, which it skips.
   */
  onError?: (error: unknown) => void;
  /**
   * How long the server may send nothing, heartbeats included, before the client drops the stream and connects again,
   * in milliseconds; defaultHeartbeatTimeout when absent.
   */
  heartbeatTimeout?: number;
};

/** The failure of a stream on which nothing arrived for the heartbeat timeout. */
export class HeartbeatTimeoutError extends Error {
  override readonly name = "HeartbeatTimeoutError";

  constructor(timeout: number) {
    super(`mykit: no heartbeat from the stream for ${String(timeout)} ms`);
  }
}

/** The failure of a 2xx answer to GET /streaming that is no event stream, such as a page of a proxy. */
export class NotEventStreamError extends Error {
  override readonly name = "NotEventStreamError";
}

/** The wait after a stream that ended, before connecting again. */
const baseDelay = 1_000;
/** The longest wait between failed connections. */
const maxDelay = 60_000;

/**
 * The wait before the next connection: a second after a stream that ended normally, doubling with each failure in a
 * row up to a minute, and spread over its upper half at random so that the clients a server restart disconnected do
 * not all come back at once.
 */
export function streamDelay(failures: number): number {
  const delay = Math.min(maxDelay, baseDelay * 2 ** Math.min(failures, 10));
  return delay / 2 + Math.random() * (delay / 2);
}

/**
 * Reads GET /streaming (or, with `timeline`, GET /streaming/timelines/{timeline}) until `signal` aborts. It sends the access token of the TokenSource and, after a 401, refreshes
 * it once per connection. It never sends Last-Event-ID: the server does not resume a stream. Whenever the stream ends
 * or fails, it connects again: after a 429 (too many streams) or a 503 it waits for Retry-After, and otherwise for an
 * exponential backoff with jitter. A stream that is silent for the heartbeat timeout counts as failed.
 *
 * It resolves once `signal` aborts, and rejects without connecting again when the server refuses the request for good:
 * a ProblemError of a 4xx other than 408 and 429, such as a 401 that a refresh did not cure or a 403 for a missing
 * scope (read:statuses and read:notifications), or a failure of the TokenSource other than a network one, such as an
 * OAuthError of a refresh token that was revoked.
 */
export async function runStream(options: StreamOptions): Promise<void> {
  const path = options.timeline === undefined ? "/streaming" : `/streaming/timelines/${options.timeline}`;
  const url = `${parseOrigin(options.server)}${apiBasePath}${path}`;
  const fetch = options.fetch ?? ((request: Request) => globalThis.fetch(request));
  const sleep: Sleep = options.sleep ?? sleepSignal;
  const heartbeat = options.heartbeatTimeout ?? defaultHeartbeatTimeout;
  const { signal } = options;
  let opened = false;
  let failures = 0;
  // signal may abort while the stream is awaited, which narrowing cannot see.
  const aborted = () => signal.aborted;
  while (!aborted()) {
    let wait: number | null = null;
    try {
      await connect(url, fetch, heartbeat, options, opened, () => {
        opened = true;
        failures = 0;
      });
    } catch (error) {
      if (aborted()) return;
      if (!recoverable(error)) throw error;
      options.onError?.(error);
      failures++;
      if (error instanceof ProblemError && (error.status === 429 || error.status === 503)) {
        wait = parseRetryAfter(error.headers.get("Retry-After"), Date.now());
      }
    }
    if (aborted()) return;
    try {
      await sleep(wait ?? streamDelay(failures), signal);
    } catch (error) {
      if (aborted()) return;
      throw error;
    }
  }
}

/** Whether connecting again may cure error: a failure of the network or of the server, a silent stream, a 408 or 429. */
function recoverable(error: unknown): boolean {
  if (error instanceof ProblemError) return error.status >= 500 || error.status === 408 || error.status === 429;
  if (error instanceof OAuthError) return error.status >= 500 || error.status === 429;
  return (
    error instanceof NetworkError || error instanceof HeartbeatTimeoutError || error instanceof NotEventStreamError
  );
}

/**
 * Opens one stream and reads it to its end, calling onOpened once it opened. It resolves when the server ends the
 * stream and rejects with the failure that ended it or kept it from opening.
 */
async function connect(
  url: string,
  fetch: (request: Request) => Promise<Response>,
  heartbeat: number,
  options: StreamOptions,
  reconnected: boolean,
  onOpened: () => void,
): Promise<void> {
  // The watchdog covers the connection too: a server that never answers is as silent as one that stopped.
  const silence = new AbortController();
  let reader: ReadableStreamDefaultReader<Uint8Array> | null = null;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const arm = () => {
    clearTimeout(timer);
    timer = setTimeout(() => {
      silence.abort(new HeartbeatTimeoutError(heartbeat));
    }, heartbeat);
  };
  const disarm = () => {
    clearTimeout(timer);
  };
  const signal = AbortSignal.any([options.signal, silence.signal]);
  // A body that the fetch does not tie to the signal stops here too.
  const stop = () => void reader?.cancel().catch(() => undefined);
  signal.addEventListener("abort", stop, { once: true });
  const silent = (error: unknown) =>
    silence.signal.aborted ? (silence.signal.reason as HeartbeatTimeoutError) : error;
  arm();
  try {
    let response: Response;
    try {
      response = await open(url, fetch, options, signal);
    } catch (error) {
      throw silent(error);
    }
    const body = response.body;
    disarm();
    options.onOpen?.(reconnected);
    onOpened();
    arm();
    if (body === null) return;
    reader = body.getReader();
    const decoder = new TextDecoder("utf-8");
    const parser = new EventStreamParser();
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (options.signal.aborted) return;
        if (silence.signal.aborted) throw silence.signal.reason as HeartbeatTimeoutError;
        if (done) break;
        disarm();
        dispatch(parser.push(decoder.decode(value, { stream: true })), options);
        arm();
      }
      dispatch([...parser.push(decoder.decode()), ...parser.end()], options);
      // The server ended the stream (its shutdown, a token that stopped being valid, a client that fell behind).
    } catch (error) {
      if (options.signal.aborted) throw error;
      throw silent(isAbort(error) ? error : new NetworkError("GET", url, error));
    } finally {
      await reader.cancel().catch(() => undefined);
    }
  } finally {
    disarm();
    signal.removeEventListener("abort", stop);
  }
}

/** Sends GET /streaming with the access token, refreshing it once after a 401, and returns an event stream's answer. */
async function open(
  url: string,
  fetch: (request: Request) => Promise<Response>,
  options: StreamOptions,
  signal: AbortSignal,
): Promise<Response> {
  const { tokenSource } = options;
  let renewed: string | null = null;
  for (let refreshed = false; ; refreshed = true) {
    const headers = new Headers({ Accept: "text/event-stream", "Cache-Control": "no-cache" });
    const language = typeof options.acceptLanguage === "function" ? options.acceptLanguage() : options.acceptLanguage;
    if (language !== undefined && language !== null && language !== "") headers.set("Accept-Language", language);
    const token = renewed ?? (await tokenSource?.getAccessToken()) ?? null;
    if (token !== null) headers.set("Authorization", `Bearer ${token}`);
    let response: Response;
    try {
      response = await fetch(new Request(url, { headers, signal }));
    } catch (error) {
      if (isAbort(error) || signal.aborted) throw signal.aborted ? signal.reason : error;
      throw new NetworkError("GET", url, error);
    }
    if (response.status === 401 && token !== null && !refreshed && tokenSource?.refresh !== undefined) {
      renewed = await tokenSource.refresh(token);
      if (renewed !== null) {
        await response.body?.cancel().catch(() => undefined);
        continue;
      }
    }
    if (response.status >= 400) throw await problemFromResponse(response);
    if (!isEventStream(response)) {
      await response.body?.cancel().catch(() => undefined);
      throw new NotEventStreamError(
        `mykit: GET ${url} answered ${response.headers.get("Content-Type") ?? "no content type"}, not an event stream`,
      );
    }
    return response;
  }
}

/** Whether `response` is an event stream. */
function isEventStream(response: Response): boolean {
  return response.headers.get("Content-Type")?.split(";")[0]?.trim().toLowerCase() === "text/event-stream";
}

/** Hands the events to onEvent, typed by their names; one whose data it cannot read goes to onError instead. */
function dispatch(events: ServerSentEvent[], options: StreamOptions): void {
  for (const e of events) {
    let parsed: StreamEvent;
    try {
      parsed = parseStreamEvent(e);
    } catch (error) {
      options.onError?.(error);
      continue;
    }
    options.onEvent(parsed);
  }
}

/** The events of the spec, whose data is an object with an `id`, but `list_update`'s. */
const knownEvents: ReadonlySet<string> = new Set([
  "update",
  "list_update",
  "notification",
  "announcement",
  "announcement_removed",
]);

/**
 * Types an event by its name: the data of `update`, `notification`, and `announcement` is the server's Status,
 * Notification, and Announcement, that of `list_update` a list's ID and a Status (once for each of the user's lists
 * whose timeline shows the post of an `update`), and that of `announcement_removed` the announcement's ID. An unknown
 * name is an `unknown` event. It throws for a known event whose data is no JSON object with an `id` (for
 * `list_update`, with a `list_id` and a `status` that has an `id`).
 */
export function parseStreamEvent({ event, data }: ServerSentEvent): StreamEvent {
  if (!knownEvents.has(event)) return { type: "unknown", event, data };
  let value: unknown;
  try {
    value = JSON.parse(data);
  } catch (error) {
    throw new Error(`mykit: the data of a stream event "${event}" is no JSON`, { cause: error });
  }
  if (event === "list_update") {
    const { list_id: listId, status } = (value ?? {}) as { list_id?: unknown; status?: unknown };
    if (
      typeof listId !== "string" ||
      typeof status !== "object" ||
      status === null ||
      typeof (status as Record<string, unknown>)["id"] !== "string"
    ) {
      throw new Error(`mykit: the data of a stream event "${event}" has no list_id or status`);
    }
    return { type: "list_update", listId, status: status as Status };
  }
  if (typeof value !== "object" || value === null || typeof (value as Record<string, unknown>)["id"] !== "string") {
    throw new Error(`mykit: the data of a stream event "${event}" has no id`);
  }
  switch (event) {
    case "update":
      return { type: "update", status: value as Status };
    case "notification":
      return { type: "notification", notification: value as Notification };
    case "announcement":
      return { type: "announcement", announcement: value as Announcement };
    default:
      return { type: "announcement_removed", id: (value as { id: string }).id };
  }
}
