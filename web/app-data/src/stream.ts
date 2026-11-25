// SPDX-License-Identifier: AGPL-3.0-only
// The stream of the session (FR-207, ADR-0028 §18): one per browser, not one per tab. The tabs form a group of
// @my/platform; the tab that leads it reads GET /api/v1/streaming with mykit's runStream and passes each event to the
// other tabs, and every tab puts the events into its own cache. The server replays nothing (ADR-0093 §6): whenever a
// stream opens after a break, a reconnection or another tab taking the stream over, every tab reads again what it may
// have missed. A browser without the group runs a stream in each tab. The leading tab shares the state of its stream
// too, so that every tab shows the same one.
import { runStream, type Sleep, type StreamEvent, type StreamOptions } from "@mycroblog/mykit";
import { joinTabGroup, type TabGroup } from "@my/platform";
import { useSyncExternalStore } from "react";
import type { AppData } from "./app-data";
import { receiveDeletion, receiveHomeGap, receiveHomeStatuses, receiveListsGap, receiveListStatuses } from "./arrivals";
import { receiveConversation, receiveConversationsGap } from "./conversations";
import {
  receiveNotification,
  receiveNotificationGap,
  receiveNotificationRequests,
  receiveNotificationsRead,
} from "./notifications";
import { useAppData } from "./provider";
import { onStreamRetry } from "./stream-retry";

/**
 * The state of the session's stream: `connecting` until it first opens, `open` while it is open, `reconnecting`
 * from a break (a failure it recovers from, or the server ending it) until it opens again, and `stopped` once the
 * server refused it for good.
 */
export type StreamState = "connecting" | "open" | "reconnecting" | "stopped";

const streamStates: ReadonlySet<string> = new Set<StreamState>(["connecting", "open", "reconnecting", "stopped"]);

/**
 * What the leading tab passes to the others: an event of the stream, a break after which they read again, or the
 * state of its stream; what a tab that joins asks of the leading one: that state; and what a tab that retries a
 * refused stream asks of the others: to wait for the lead again.
 */
type StreamMessage =
  | { readonly kind: "event"; readonly event: StreamEvent }
  | { readonly kind: "gap" }
  | { readonly kind: "state"; readonly state: StreamState }
  | { readonly kind: "ask" }
  | { readonly kind: "retry" };

type StateStore = { state: StreamState | undefined; readonly listeners: Set<() => void> };

const stateStores = new WeakMap<AppData, StateStore>();

function stateStoreOf(data: AppData): StateStore {
  let store = stateStores.get(data);
  if (!store) {
    store = { state: undefined, listeners: new Set() };
    stateStores.set(data, store);
  }
  return store;
}

function setStreamState(data: AppData, state: StreamState | undefined): void {
  const store = stateStoreOf(data);
  if (store.state === state) return;
  store.state = state;
  for (const listener of store.listeners) listener();
}

/** The state of the stream of `data`; undefined while it has none (no session, or its stream is not started). */
export function streamStateOf(data: AppData): StreamState | undefined {
  return stateStoreOf(data).state;
}

/** Calls listener whenever the state of the stream of `data` changes, until the function it returns is called. */
export function watchStreamState(data: AppData, listener: () => void): () => void {
  const store = stateStoreOf(data);
  store.listeners.add(listener);
  return () => {
    store.listeners.delete(listener);
  };
}

/**
 * The state of the session's stream, the same in every tab of the browser; undefined while there is no stream (the
 * reader is signed out).
 */
export function useStreamState(): StreamState | undefined {
  const data = useAppData();
  return useSyncExternalStore(
    (listener) => watchStreamState(data, listener),
    () => streamStateOf(data),
    () => streamStateOf(data),
  );
}

/** What startStream runs on; the browser's tab group and mykit's runStream unless a test gives its own. */
export type StreamDeps = {
  /** The tabs that share one stream; null runs a stream in this tab alone. */
  readonly tabs?: TabGroup | null;
  /**
   * Reads the stream until its signal aborts, waiting with the options' `sleep` between a connection that ended or
   * failed and the next one, and rejects once the server refused it for good.
   */
  readonly run?: (options: StreamOptions) => Promise<void>;
};

/** The longest delay of setTimeout, which runs a longer one at once. */
const maxTimeout = 2 ** 31 - 1;

/** Waits for ms, or rejects with the reason of signal when it aborts first. */
const wait: Sleep = (ms, signal) =>
  new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(signal.reason as Error);
      return;
    }
    const abort = () => {
      clearTimeout(timer);
      reject(signal.reason as Error);
    };
    const timer = setTimeout(
      () => {
        signal.removeEventListener("abort", abort);
        resolve();
      },
      Math.min(ms, maxTimeout),
    );
    signal.addEventListener("abort", abort, { once: true });
  });

/**
 * Starts the stream of the session, which runs until the function it returns is called. Data without a connection
 * (the browser has no session) has no stream. Once the server refused it, retryStream starts it again.
 */
export function startStream(data: AppData, deps: StreamDeps = {}): () => void {
  const { connection } = data;
  if (connection === undefined) return () => {};
  const tabs = deps.tabs === undefined ? joinTabGroup(`my:stream:${connection.server}`) : deps.tabs;
  const run = deps.run ?? runStream;
  const stop = new AbortController();
  // missed: whether the tabs may have missed events before this stream (another tab led one, or it was refused), and
  // so read again once it opens.
  const read = (share: (message: SharedMessage) => void, missed: boolean) => {
    const state = (next: StreamState) => {
      if (!stop.signal.aborted) share({ kind: "state", state: next });
    };
    state("connecting");
    return run({
      ...connection,
      signal: stop.signal,
      // runStream waits here, and only here, between a connection that ended or failed and the next one.
      sleep: (ms, signal) => {
        state("reconnecting");
        return wait(ms, signal);
      },
      onOpen: (reconnected) => {
        state("open");
        if (reconnected || missed) share({ kind: "gap" });
      },
      onEvent: (event) => {
        share({ kind: "event", event });
      },
    }).catch(() => {
      // The server refused the stream for good (the session ended, or a scope is missing): the screens read the
      // server as they open, the next tab to lead tries again, and a pull of a timeline retries it (retryStream).
      state("stopped");
    });
  };
  const receive = (message: SharedMessage) => {
    route(data, message);
  };
  // Whether this tab reads a stream of its own (alone, or where the browser refused the lock), until it is refused.
  let reading = false;
  const readAlone = (missed: boolean) => {
    if (reading || stop.signal.aborted) return;
    reading = true;
    void read(receive, missed).finally(() => {
      reading = false;
    });
  };
  if (tabs === null) {
    readAlone(false);
    const unretry = onStreamRetry(data, () => {
      if (streamStateOf(data) === "stopped") readAlone(true);
    });
    return () => {
      stop.abort();
      unretry();
      setStreamState(data, undefined);
    };
  }
  setStreamState(data, "connecting");
  // Whether this tab leads the group, and so answers the tabs that ask for the state of its stream.
  let leading = false;
  // Whether this tab waits for the lead or holds it: a tab whose stream was refused gave the lead up and waits no more.
  let queued = false;
  // Whether the browser refused this tab the lock, so that it reads alone.
  let alone = false;
  const join = (missed: boolean) => {
    if (alone) {
      readAlone(missed);
      return;
    }
    if (queued || stop.signal.aborted) return;
    queued = true;
    tabs
      .lead(stop.signal, async (tookOver) => {
        leading = true;
        try {
          await read((message) => {
            tabs.post(message);
            receive(message);
          }, tookOver || missed);
        } finally {
          leading = false;
        }
      })
      .catch(() => {
        // The browser refused the lock (a document without storage access): this tab reads a stream of its own.
        alone = true;
        readAlone(missed);
      })
      .finally(() => {
        queued = false;
      });
  };
  const unlisten = tabs.listen((message) => {
    if (!isStreamMessage(message)) return;
    if (message.kind === "ask") {
      const state = streamStateOf(data);
      if (leading && state !== undefined) tabs.post({ kind: "state", state } satisfies StreamMessage);
    } else if (message.kind === "retry") join(true);
    else receive(message);
  });
  join(false);
  // A retry brings every tab that gave the lead up back into the queue: the first to take it reads, and the others
  // follow it and take over when it goes, as before the refusal.
  const unretry = onStreamRetry(data, () => {
    if (streamStateOf(data) !== "stopped") return;
    tabs.post({ kind: "retry" } satisfies StreamMessage);
    join(true);
  });
  // A tab that joins while another leads learns the state of its stream.
  tabs.post({ kind: "ask" } satisfies StreamMessage);
  return () => {
    stop.abort();
    unretry();
    unlisten();
    tabs.close();
    setStreamState(data, undefined);
  };
}

/** What a tab puts into its own data, from its stream or from the leading tab's. */
type SharedMessage = Exclude<StreamMessage, { kind: "ask" | "retry" }>;

/**
 * Puts what the stream says into the cache: new statuses of the home timeline or of a list, a deleted status, a
 * notification, the count of the notification requests, what the reader read on a device, a conversation of the direct messages, or a break;
 * and the state of the stream into the tab's own.
 */
function route(data: AppData, message: SharedMessage): void {
  if (message.kind === "state") {
    setStreamState(data, message.state);
    return;
  }
  if (message.kind === "gap") {
    receiveHomeGap(data);
    receiveListsGap(data);
    receiveNotificationGap(data);
    receiveConversationsGap(data);
    return;
  }
  const { event } = message;
  switch (event.type) {
    case "update":
      receiveHomeStatuses(data, [event.status]);
      break;
    case "list_update":
      receiveListStatuses(data, event.listId, [event.status]);
      break;
    case "delete":
      receiveDeletion(data, event.id);
      break;
    case "notification":
      receiveNotification(data, event.notification);
      break;
    case "notification_requests":
      receiveNotificationRequests(data, event.summary);
      break;
    case "notifications_read":
      receiveNotificationsRead(data, event.read);
      break;
    case "conversation":
      receiveConversation(data, event.conversation);
      break;
    default:
      // The announcements are read again as the shell lists them.
      break;
  }
}

/** Whether a message from another tab, which may run another version of the client (ADR-0028 §14), is one it reads. */
function isStreamMessage(message: unknown): message is StreamMessage {
  if (typeof message !== "object" || message === null || !("kind" in message)) return false;
  if (message.kind === "gap" || message.kind === "ask" || message.kind === "retry") return true;
  if (message.kind === "state") {
    return "state" in message && typeof message.state === "string" && streamStates.has(message.state);
  }
  return (
    message.kind === "event" &&
    "event" in message &&
    typeof message.event === "object" &&
    message.event !== null &&
    "type" in message.event &&
    typeof message.event.type === "string"
  );
}
