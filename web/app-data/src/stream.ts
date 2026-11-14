// SPDX-License-Identifier: AGPL-3.0-only
// The stream of the session (FR-207, ADR-0028 §18): one per browser, not one per tab. The tabs form a group of
// @my/platform; the tab that leads it reads GET /api/v1/streaming with mykit's runStream and passes each event to the
// other tabs, and every tab puts the events into its own cache. The server replays nothing (ADR-0093 §6): whenever a
// stream opens after a break, a reconnection or another tab taking the stream over, every tab reads again what it may
// have missed. A browser without the group runs a stream in each tab.
import { runStream, type StreamEvent, type StreamOptions } from "@mycroblog/mykit";
import { joinTabGroup, type TabGroup } from "@my/platform";
import type { AppData } from "./app-data";
import { receiveHomeGap, receiveHomeStatuses } from "./arrivals";
import { receiveNotification, receiveNotificationGap } from "./notifications";

/** What the leading tab passes to the others: an event of the stream, or a break after which they read again. */
type StreamMessage = { readonly kind: "event"; readonly event: StreamEvent } | { readonly kind: "gap" };

/** What startStream runs on; the browser's tab group and mykit's runStream unless a test gives its own. */
export type StreamDeps = {
  /** The tabs that share one stream; null runs a stream in this tab alone. */
  readonly tabs?: TabGroup | null;
  /** Reads the stream until its signal aborts. */
  readonly run?: (options: StreamOptions) => Promise<void>;
};

/**
 * Starts the stream of the session, which runs until the function it returns is called. Data without a connection
 * (the browser has no session) has no stream.
 */
export function startStream(data: AppData, deps: StreamDeps = {}): () => void {
  const { connection } = data;
  if (connection === undefined) return () => {};
  const tabs = deps.tabs === undefined ? joinTabGroup(`my:stream:${connection.server}`) : deps.tabs;
  const run = deps.run ?? runStream;
  const stop = new AbortController();
  const read = (share: (message: StreamMessage) => void, tookOver: boolean) =>
    run({
      ...connection,
      signal: stop.signal,
      onOpen: (reconnected) => {
        if (reconnected || tookOver) share({ kind: "gap" });
      },
      onEvent: (event) => {
        share({ kind: "event", event });
      },
    }).catch(() => {
      // The server refused the stream for good (the session ended, or a scope is missing): the screens read the
      // server as they open, and the next tab to lead tries again.
    });
  const receive = (message: StreamMessage) => {
    route(data, message);
  };
  if (tabs === null) {
    void read(receive, false);
    return () => {
      stop.abort();
    };
  }
  const unlisten = tabs.listen((message) => {
    if (isStreamMessage(message)) receive(message);
  });
  tabs
    .lead(stop.signal, (tookOver) =>
      read((message) => {
        tabs.post(message);
        receive(message);
      }, tookOver),
    )
    .catch(() => {
      // The browser refused the lock (a document without storage access): this tab reads a stream of its own.
      if (!stop.signal.aborted) void read(receive, false);
    });
  return () => {
    stop.abort();
    unlisten();
    tabs.close();
  };
}

/** Puts what the stream says into the cache: new home statuses, an unread notification, or a break. */
function route(data: AppData, message: StreamMessage): void {
  if (message.kind === "gap") {
    receiveHomeGap(data);
    receiveNotificationGap(data);
    return;
  }
  const { event } = message;
  switch (event.type) {
    case "update":
      receiveHomeStatuses(data, [event.status]);
      break;
    case "notification":
      receiveNotification(data);
      break;
    default:
      // The lists, the direct messages and the announcements have no screen yet.
      break;
  }
}

/** Whether a message from another tab, which may run another version of the client (ADR-0028 §14), is one it reads. */
function isStreamMessage(message: unknown): message is StreamMessage {
  if (typeof message !== "object" || message === null || !("kind" in message)) return false;
  if (message.kind === "gap") return true;
  return (
    message.kind === "event" &&
    "event" in message &&
    typeof message.event === "object" &&
    message.event !== null &&
    "type" in message.event &&
    typeof message.event.type === "string"
  );
}
