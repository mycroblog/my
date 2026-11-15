// SPDX-License-Identifier: AGPL-3.0-only
// What the shell says about the server itself, read from every answer of the client API (ADR-0088 §10, §15,
// ADR-0028 §11, §12): whether the server is under maintenance, whether a failed request means the reader is offline or
// the server is down, and whether this build of the app is too old for the server. Every request of the app goes
// through the watch's `fetch`; the watch asks the server's health (`GET /instance/health`, which needs no token) to
// tell the reader's own network from an outage, and asks it again until the server answers.
import { apiBasePath } from "@mycroblog/mykit";
import { watchOnline } from "@my/platform";

/**
 * The number of the last change of the client API that this build cannot work with (ADR-0028 §12); raised by hand,
 * together with the server's `clientapi.WebClientEpoch`, in a change that breaks older web clients.
 */
export const webClientEpoch = 1;

/** The response header that carries the server's epoch. */
const epochHeader = "X-Web-Client-Epoch";

/** The problem type of the maintenance mode's answers (ADR-0088 §10). */
const maintenanceType = "/api/v1/problems/maintenance";

/** How long the watch waits before it asks the health again while the server does not answer, by default. */
const defaultRecheckMs = 30_000;

/** The server's maintenance, with what its operator set: their notice, when it should end, the status page. */
export type Maintenance = {
  readonly message?: string | undefined;
  /** When the maintenance should end, as RFC 3339. */
  readonly until?: string | undefined;
  readonly statusPageUrl?: string | undefined;
};

/** Whether the server answers: `offline` when the reader's device reaches nothing, `outage` when the server is down. */
export type Connection = "online" | "offline" | "outage";

/**
 * Whether the app should be loaded again: `ready` when a newer build waits (the service worker's, ADR-0028 §11),
 * `required` when the server's epoch is above this build's (§12).
 */
export type Update = "none" | "ready" | "required";

export type ServerState = {
  readonly connection: Connection;
  readonly maintenance: Maintenance | null;
  readonly update: Update;
};

export type ServerWatch = {
  /** The origin of the server's client API. */
  readonly server: string;
  /** Sends a request of the client API, and reads its answer for the state. */
  readonly fetch: (request: Request) => Promise<Response>;
  /** The state now; the same object until it changes. */
  readonly state: () => ServerState;
  /** Calls `listener` on every change of the state, until the function it returns is called. */
  readonly subscribe: (listener: () => void) => () => void;
  /** Asks the server's health now, and settles the connection and the maintenance by its answer. */
  readonly check: () => Promise<void>;
  /** Says that a newer build of the app waits, for the toast that offers it (ADR-0028 §11). */
  readonly offerUpdate: () => void;
  /** Closes the toast of a newer build until the app loads again; a required update stays. */
  readonly dismissUpdate: () => void;
  /** Stops asking the health and listening to the device's network. */
  readonly stop: () => void;
};

export type ServerWatchOptions = {
  /** The origin of the server's client API. */
  server: string;
  /** Sends the requests; the global `fetch` by default. */
  fetch?: (request: Request) => Promise<Response>;
  /** This build's epoch; webClientEpoch by default. */
  epoch?: number;
  /** How long to wait before asking the health again while the server does not answer, in ms. */
  recheckMs?: number;
  /** Whether to follow the device's network as the browser tells it; true by default. */
  followDevice?: boolean;
};

const healthy: ServerState = { connection: "online", maintenance: null, update: "none" };

/** The members of a maintenance problem that the operator set. */
function maintenanceOf(body: unknown): Maintenance | null {
  if (typeof body !== "object" || body === null) return null;
  const problem = body as Record<string, unknown>;
  if (problem["type"] !== maintenanceType) return null;
  const text = (key: string) => (typeof problem[key] === "string" && problem[key] !== "" ? problem[key] : undefined);
  return { message: text("message"), until: text("until"), statusPageUrl: text("status_page_url") };
}

/** Seconds of a Retry-After header, in ms; undefined for none or a date. */
function retryAfterMs(response: Response): number | undefined {
  const value = Number(response.headers.get("Retry-After"));
  return Number.isFinite(value) && value > 0 ? value * 1000 : undefined;
}

/** Watches the server of `server` through the answers of the requests sent with the watch's `fetch`. */
export function watchServer({
  server,
  fetch = (request) => globalThis.fetch(request),
  epoch = webClientEpoch,
  recheckMs = defaultRecheckMs,
  followDevice = true,
}: ServerWatchOptions): ServerWatch {
  let state = healthy;
  let dismissed = false;
  const listeners = new Set<() => void>();
  let timer: ReturnType<typeof setTimeout> | undefined;
  let checking: Promise<void> | null = null;

  const set = (next: Partial<ServerState>) => {
    const merged = { ...state, ...next };
    if (
      merged.connection === state.connection &&
      merged.maintenance === state.maintenance &&
      merged.update === state.update
    )
      return;
    state = merged;
    for (const listener of listeners) listener();
  };

  /** Asks the health again after `ms`, unless a check comes first. */
  const recheckIn = (ms: number) => {
    clearTimeout(timer);
    timer = setTimeout(() => {
      void check();
    }, ms);
  };

  const answered = () => {
    clearTimeout(timer);
    if (state.connection !== "online" || state.maintenance !== null) set({ connection: "online", maintenance: null });
  };

  async function askHealth(): Promise<void> {
    let response: Response;
    try {
      response = await fetch(new Request(`${server}${apiBasePath}/instance/health`, { cache: "no-store" }));
    } catch {
      set({ connection: "offline" });
      recheckIn(recheckMs);
      return;
    }
    if (response.ok) {
      answered();
      return;
    }
    const body = (await response.json().catch(() => null)) as { status?: unknown } | null;
    if (body?.status === "maintenance") {
      set({ connection: "online", maintenance: state.maintenance ?? {} });
      recheckIn(retryAfterMs(response) ?? recheckMs);
      return;
    }
    set({ connection: "outage" });
    recheckIn(recheckMs);
  }

  function check(): Promise<void> {
    checking ??= askHealth().finally(() => {
      checking = null;
    });
    return checking;
  }

  async function read(response: Response): Promise<void> {
    const theirs = Number(response.headers.get(epochHeader));
    if (Number.isInteger(theirs) && theirs > epoch) set({ update: "required" });
    if (response.status === 503) {
      const maintenance = maintenanceOf(
        await response
          .clone()
          .json()
          .catch(() => null),
      );
      if (maintenance !== null) {
        set({ connection: "online", maintenance });
        recheckIn(retryAfterMs(response) ?? recheckMs);
        return;
      }
    }
    if (response.status >= 500) void check();
    else answered();
  }

  async function send(request: Request): Promise<Response> {
    let response: Response;
    try {
      response = await fetch(request);
    } catch (err) {
      if (!request.signal.aborted) void check();
      throw err;
    }
    await read(response);
    return response;
  }

  const unfollow = followDevice
    ? watchOnline((online) => {
        if (online) void check();
        else set({ connection: "offline" });
      })
    : () => undefined;

  return {
    server,
    fetch: send,
    state: () => state,
    subscribe: (listener) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    check,
    offerUpdate: () => {
      if (state.update === "none" && !dismissed) set({ update: "ready" });
    },
    dismissUpdate: () => {
      if (state.update !== "ready") return;
      dismissed = true;
      set({ update: "none" });
    },
    stop: () => {
      clearTimeout(timer);
      unfollow();
    },
  };
}

/** The state of a server no watch follows: answering, not under maintenance, with nothing to update. */
export const unwatchedServer: ServerState = healthy;
