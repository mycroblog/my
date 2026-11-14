// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { AppDataProvider, createAppData, homeTimelineQuery } from "@my/app-data";
import { I18nProvider } from "@my/design-system";
import { createClient } from "@mycroblog/mykit";
import { renderToStaticMarkup } from "react-dom/server";
import { HomeTimeline, loadHomeTimeline } from "./home";

/** A server whose home timeline answers `answer`, and the paths of the requests it got. */
function server(answer: () => Response) {
  const paths: string[] = [];
  const api = createClient({
    server: "https://example.com",
    retry: { disabled: true },
    fetch: (request) => {
      paths.push(new URL(request.url).pathname);
      return Promise.resolve(answer());
    },
  });
  return { data: createAppData(api), paths };
}

const page = {
  items: [
    {
      id: "1",
      account: { id: "1", username: "lena", acct: "lena", domain: "my.example", display_name: "Lena", emojis: [] },
      content: "<p>Hello</p>",
      spoiler_text: "",
      visibility: "public",
      language: "en",
      created_at: new Date().toISOString(),
      media_attachments: [],
      mentions: [],
      emojis: [],
      replies_count: 0,
      boosts_count: 0,
      favourites_count: 0,
      reactions: [],
    },
  ],
};

const render = (data: ReturnType<typeof server>["data"], locale = "en") =>
  renderToStaticMarkup(
    <AppDataProvider data={data}>
      <I18nProvider locale={locale}>
        <HomeTimeline />
      </I18nProvider>
    </AppDataProvider>,
  );

test("the loader waits for the first page, and the column shows it under the timeline's name", async () => {
  const { data, paths } = server(() => Response.json(page));
  await loadHomeTimeline(data);
  expect(paths).toEqual(["/api/v1/timelines/home"]);
  const html = render(data, "ja");
  expect(html).toMatch(/role="feed" aria-label="ホームタイムライン"/);
  expect(html).toContain("Hello");
});

test("a failed first page does not fail the loader: the column fetches it again and shows how that goes", async () => {
  const problem = { type: "about:blank", title: "Internal Server Error", status: 500 };
  const { data } = server(() =>
    Response.json(problem, { status: 500, headers: { "Content-Type": "application/problem+json" } }),
  );
  await loadHomeTimeline(data);
  expect(data.queryClient.getQueryState(homeTimelineQuery(data).queryKey)?.status).toBe("error");
  expect(render(data)).toMatch(/role="feed" aria-label="Home timeline" aria-busy="true"/);
});
