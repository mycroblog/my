// SPDX-License-Identifier: AGPL-3.0-only
// The one subscription of the app to the streaming API (web-client/README.md S10). The stream is an external
// system that the app stays in sync with while a user is signed in, which is what an Effect is for
// (react.dev, "Synchronizing with Effects"; "Lifecycle of Reactive Effects"): it connects when the session
// is signed in, and its cleanup aborts the request when the session ends or the app unmounts. The
// callbacks are Effect Events (react.dev, "Separating Events from Effects"), so new callbacks on a
// re-render do not reconnect.
//
// The connection is the streaming client of @mycroblog/mykit (Api.stream, lib/api/client.ts): it reads the body of
// fetch with the session's Bearer token, refreshes the token once after a 401, and connects again after every end or
// failure, with an exponential backoff and jitter, Retry-After after a 429 or 503, and a heartbeat timeout. The server
// replays nothing, so a stream that opens again tells the caller to read again what happened in between.
import type { StreamEvent as MykitStreamEvent } from "@mycroblog/mykit";
import { useEffect, useEffectEvent } from "react";
import { useApi, useSessionStatus } from "../auth/SessionProvider";

/** An event of the stream that the app reads; it skips the events that a newer server may add. */
type StreamEvent = Exclude<MykitStreamEvent, { type: "unknown" }>;

export type EventStreamHandlers = {
  /** An event arrived: write what it means into the Query cache. */
  onEvent: (event: StreamEvent) => void;
  /** A stream opened again after one ended: read again what the gap may have missed. */
  onReconnect: () => void;
};

/** Keeps the signed-in user's event stream open, reconnecting with backoff, and hands its events to `handlers`. */
export function useEventStream({ onEvent, onReconnect }: EventStreamHandlers): void {
  const api = useApi();
  const signedIn = useSessionStatus() === "signed-in";
  const handleEvent = useEffectEvent((event: StreamEvent) => {
    onEvent(event);
  });
  const handleOpen = useEffectEvent((reconnected: boolean) => {
    if (reconnected) onReconnect();
  });
  useEffect(() => {
    if (!signedIn) return;
    const controller = new AbortController();
    api
      .stream({
        signal: controller.signal,
        onOpen: (reconnected) => {
          handleOpen(reconnected);
        },
        onEvent: (event) => {
          if (event.type !== "unknown") handleEvent(event);
        },
        onError: (error) => {
          // Offline, a 429 (too many streams), a server that went away: the stream connects again.
          console.warn(error);
        },
      })
      .catch((error: unknown) => {
        // The server refused the stream for good (a 401 that the refresh did not cure, a 403): no live updates
        // until the session changes.
        console.warn("the event stream stopped", error);
      });
    return () => {
      controller.abort();
    };
  }, [api, signedIn]);
}
