// SPDX-License-Identifier: AGPL-3.0-only
// A fake server for the tests: the client's fetch hands each request to a handler, and the test reads what the
// server received. No test reaches a live server.
import { createClient } from "@mycroblog/mykit";
import { createAppData, type AppData } from "../app-data";
import type { Status } from "../statuses";

export type Handler = (request: Request) => Response | Promise<Response>;

/** AppData whose client sends every request to handler, without retries, and the URLs of the requests it sent. */
export function fakeAppData(handler: Handler): { data: AppData; requests: URL[] } {
  const requests: URL[] = [];
  const api = createClient({
    server: "https://example.com",
    retry: { disabled: true },
    fetch: async (request) => {
      requests.push(new URL(request.url));
      return handler(request);
    },
  });
  return { data: createAppData(api), requests };
}

/** A JSON answer. */
export function json(status: number, body: unknown): Response {
  const type = status >= 400 ? "application/problem+json" : "application/json";
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": type } });
}

/** A status with the ID given and the fields a test sets; the rest are left out. */
export function status(id: string, fields: Partial<Status> = {}): Status {
  return { id, content: `<p>${id}</p>`, ...fields } as Status;
}

/** A page of the home timeline with the statuses of the IDs given. */
export function page(ids: string[], next?: string): unknown {
  return { items: ids.map((id) => status(id)), ...(next === undefined ? {} : { next_cursor: next }) };
}
