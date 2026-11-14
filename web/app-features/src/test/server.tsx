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

/** A server that answers `answers`, and the requests it got as `METHOD path?query`. */
export function server(answers: Answers): { data: AppData; requests: string[] } {
  const requests: string[] = [];
  const api = createClient({
    server: "https://example.com",
    retry: { disabled: true },
    fetch: (request) => {
      const url = new URL(request.url);
      const path = url.pathname.replace(/^\/api\/v1/, "");
      requests.push(`${request.method} ${path}${url.search}`);
      const body = answers[path];
      if (body === undefined || typeof body === "number") {
        const status = typeof body === "number" ? body : 404;
        return Promise.resolve(
          Response.json(
            { type: "about:blank", title: "Problem", status },
            { status, headers: { "Content-Type": "application/problem+json" } },
          ),
        );
      }
      return Promise.resolve(Response.json(body));
    },
  });
  return { data: createAppData(api), requests };
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
