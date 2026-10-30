// SPDX-License-Identifier: AGPL-3.0-only
// GET /streaming (operationId streamEvents, lib/sse) and GET /streaming/timelines/{timeline} (streamTimeline): the
// default handlers and the event streams of the tests that send events (web-client/10-api-client-and-auth.md §10).
import { http, HttpResponse } from "msw";
import { preferencesFixture } from "../fixtures";
import { apiBase, authorized, unauthenticated } from "./http";

/** One open GET /streaming response of a test. */
export type TestEventStream = {
  /** Sends an event, as the server writes one (`event` and one `data` line of JSON). */
  send: (event: string, data: unknown) => void;
  /** Ends the stream, as the server does when it shuts down or the token stops being valid. */
  close: () => void;
};

/**
 * A text/event-stream response that starts with the server's `: connected` comment and stays open until
 * `close` or until the client aborts the request.
 */
export function eventStreamResponse(): { response: Response; stream: TestEventStream } {
  const encoder = new TextEncoder();
  let controller: ReadableStreamDefaultController<Uint8Array> | undefined;
  let closed = false;
  const body = new ReadableStream<Uint8Array>({
    start(c) {
      controller = c;
      c.enqueue(encoder.encode(": connected\n\n"));
    },
    cancel() {
      closed = true;
    },
  });
  const stream: TestEventStream = {
    send: (event, data) => {
      if (!closed) controller?.enqueue(encoder.encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`));
    },
    close: () => {
      if (closed) return;
      closed = true;
      controller?.close();
    },
  };
  const response = new HttpResponse(body, {
    headers: { "Content-Type": "text/event-stream", "Cache-Control": "no-store" },
  });
  return { response, stream };
}

/**
 * GET /streaming (operationId streamEvents), or the stream at `path` (`/streaming/timelines/local`), for a test that
 * sends events: every connection opens a new stream, and `current()` is the latest. Pass `handler` to `server.use()`.
 */
export function streamingServer(path = "/streaming") {
  const streams: TestEventStream[] = [];
  const handler = http.get(`${apiBase}${path}`, ({ request }) => {
    if (!authorized(request)) return unauthenticated(request);
    const { response, stream } = eventStreamResponse();
    streams.push(stream);
    return response;
  });
  return {
    handler,
    /** How many times the client has connected. */
    connections: () => streams.length,
    /** The latest stream; throws before the first connection. */
    current: (): TestEventStream => {
      const stream = streams.at(-1);
      if (stream === undefined) throw new Error(`the client has not connected to GET ${path}`);
      return stream;
    },
  };
}

/**
 * The default GET /streaming and GET /streaming/timelines/{timeline}: a stream that sends nothing and ends at once, as
 * a server ends a stream; it needs a token. It does not stay open: MSW intercepts Node's fetch at the socket, and a response that is
 * still streaming when a test ends (its request aborted by the unmount) has let a later test's request
 * reach the real network. A test that sends events opens its streams with `streamingServer()`.
 */
/**
 * GET /preferences of a user who turned the automatic insertion of new posts off (ADR-0077), for the tests of the
 * "N new posts" button: new posts wait behind it instead of going into the feed.
 */
export function newPostsHeld() {
  return http.get(`${apiBase}/preferences`, () =>
    HttpResponse.json({ ...preferencesFixture, timeline_auto_insert: false }),
  );
}

export const handlers = [
  http.get(`${apiBase}/streaming`, ({ request }) => {
    if (!authorized(request)) return unauthenticated(request);
    const { response, stream } = eventStreamResponse();
    stream.close();
    return response;
  }),
  http.get(`${apiBase}/streaming/timelines/:timeline`, ({ request }) => {
    if (!authorized(request)) return unauthenticated(request);
    const { response, stream } = eventStreamResponse();
    stream.close();
    return response;
  }),
];
