// SPDX-License-Identifier: MIT
// A fake server for the unit tests: the client's fetch hands each request to a handler, and the test reads what the
// server received.

export const server = "https://example.com";

export type Handler = (request: Request) => Response | Promise<Response>;

/** A fetch that answers with handler and records every request it received, with its body read as text. */
export function fakeFetch(handler: Handler): {
  fetch: (input: Request) => Promise<Response>;
  requests: { request: Request; body: string }[];
} {
  const requests: { request: Request; body: string }[] = [];
  return {
    requests,
    fetch: async (input) => {
      const body = await input.clone().text();
      requests.push({ request: input, body });
      return handler(input);
    },
  };
}

/** A JSON answer. */
export function json(status: number, body: unknown, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", ...headers } });
}

/** An RFC 9457 problem answer. */
export function problem(
  status: number,
  type: string,
  extra: Record<string, unknown> = {},
  headers: Record<string, string> = {},
): Response {
  return new Response(JSON.stringify({ type, title: `title of ${type}`, status, ...extra }), {
    status,
    headers: { "Content-Type": "application/problem+json", ...headers },
  });
}

/** A sleep that records the waits and returns at once. */
export function fakeSleep(): { sleep: (ms: number) => Promise<void>; waits: number[] } {
  const waits: number[] = [];
  return {
    waits,
    sleep: (ms) => {
      waits.push(ms);
      return Promise.resolve();
    },
  };
}

/** A page of the home timeline with the IDs given. */
export function statusPage(ids: string[], next?: string): unknown {
  return { items: ids.map((id) => ({ id })), ...(next === undefined ? {} : { next_cursor: next }) };
}
