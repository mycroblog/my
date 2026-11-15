// SPDX-License-Identifier: AGPL-3.0-only
// A fake server for the features' tests, answering each path the test names, and the screens rendered under the
// providers the app places. No test reaches a live server.

import { AppDataProvider, createAppData, type AppData } from "@my/app-data";
import { I18nProvider } from "@my/design-system";
import { createClient } from "@mycroblog/mykit";
import type { ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";

/** The answers by the path after /api/v1, each a body (200) or a status; any other path is a 404. */
export type Answers = Readonly<Record<string, unknown>>;

/**
 * A server that answers `answers`, the requests it got as `METHOD path?query`, and `received(request)`, the JSON body
 * of the first request it gets of `METHOD path`, once it got it.
 */
export function server(answers: Answers): {
  data: AppData;
  requests: string[];
  received: (request: string) => Promise<unknown>;
} {
  const requests: string[] = [];
  type Waiting = { promise: Promise<unknown>; resolve: (body: unknown) => void };
  const bodies = new Map<string, Waiting>();
  const bodyOf = (request: string): Waiting => {
    const known = bodies.get(request);
    if (known !== undefined) return known;
    let resolve: (body: unknown) => void = () => undefined;
    const promise = new Promise<unknown>((r) => {
      resolve = r;
    });
    const waiting = { promise, resolve };
    bodies.set(request, waiting);
    return waiting;
  };
  const api = createClient({
    server: "https://example.com",
    retry: { disabled: true },
    fetch: async (request) => {
      const url = new URL(request.url);
      const path = url.pathname.replace(/^\/api\/v1/, "");
      requests.push(`${request.method} ${path}${url.search}`);
      const text = await request.text();
      bodyOf(`${request.method} ${path}`).resolve(text === "" ? undefined : JSON.parse(text));
      const body = answers[path];
      if (body === undefined || typeof body === "number") {
        const status = typeof body === "number" ? body : 404;
        return Response.json(
          { type: "about:blank", title: "Problem", status },
          { status, headers: { "Content-Type": "application/problem+json" } },
        );
      }
      return Response.json(body);
    },
  });
  return { data: createAppData(api), requests, received: (request) => bodyOf(request).promise };
}

/** The markup of `children` under the app's providers, in `locale`. */
export const render = (data: AppData, children: ReactNode, locale = "en") =>
  renderToStaticMarkup(
    <AppDataProvider data={data}>
      <I18nProvider locale={locale}>{children}</I18nProvider>
    </AppDataProvider>,
  );

/** An account of the API with the fields a test sets. */
export const account = (id: string, fields: Record<string, unknown> = {}) => ({
  id,
  username: id,
  acct: id,
  domain: "my.example",
  display_name: id.charAt(0).toUpperCase() + id.slice(1),
  emojis: [],
  url: `https://my.example/@${id}`,
  note: "",
  locked: false,
  bot: false,
  created_at: "2025-04-01T00:00:00Z",
  fields: [],
  badges: [],
  ...fields,
});

/** An open poll of the API, which the reader has not voted in, with the fields a test sets. */
export const poll = (id: string, fields: Record<string, unknown> = {}) => ({
  id,
  expires_at: "2026-11-21T03:00:00Z",
  expired: false,
  multiple: false,
  votes_count: 0,
  voters_count: 0,
  options: [
    { title: "Sourdough", votes_count: 0 },
    { title: "Rye", votes_count: 0 },
  ],
  voted: false,
  own_votes: [],
  emojis: [],
  ...fields,
});

/** A status of the API by `author`, with the fields a test sets. */
export const status = (id: string, author = "lena", fields: Record<string, unknown> = {}) => ({
  id,
  account: account(author),
  content: `<p>Post ${id}</p>`,
  spoiler_text: "",
  visibility: "public",
  language: "en",
  created_at: "2026-11-14T03:00:00Z",
  media_attachments: [],
  mentions: [],
  emojis: [],
  replies_count: 0,
  boosts_count: 0,
  favourites_count: 0,
  reactions: [],
  ...fields,
});
