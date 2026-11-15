// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { viewerQuery } from "./accounts";
import {
  articleFieldLimits,
  defaultArticleLimits,
  editArticle,
  postArticle,
  previewArticle,
  readArticleSource,
  useArticleAuthoring,
  useArticleLimits,
  useArticlePreview,
} from "./articles";
import { instanceQuery } from "./instance";
import type { Permission } from "./rights";
import { cachedStatus } from "./shadow";
import { fakeAppData, json, page, status } from "./test/fake";
import { probe } from "./test/probe";
import { homeTimelineQuery } from "./timeline";

const ids = (data: ReturnType<typeof fakeAppData>["data"]) =>
  data.queryClient.getQueryData(homeTimelineQuery(data).queryKey)?.pages[0]?.items.map((s) => s.id);

/** `METHOD path` of a request, without the API's prefix. */
const line = (request: Request) => `${request.method} ${new URL(request.url).pathname.replace(/^\/api\/v1/, "")}`;

const preview = {
  content: "<h2>Day one</h2><p>Flour and water.</p>",
  summary: "Flour and water.",
  summary_from_body: true,
  bytes: 39,
  max_bytes: 100_000,
  images: 0,
  max_images: 20,
};

const article = (title: string) => ({ title, summary: "A diary of a jar of flour." });

test("the limits of an article are those of FR-123 until the server says its own", () => {
  expect(defaultArticleLimits).toEqual({ max_bytes: 100_000, max_images: 20 });
  expect(articleFieldLimits).toEqual({ title: 200, summary: 500, tags: 10 });
  const { data } = fakeAppData(() => json(404, { status: 404, title: "Not Found" }));
  expect(probe(data, useArticleLimits)).toEqual(defaultArticleLimits);
});

test("the limits of an article are the server's once it answered", async () => {
  const { data } = fakeAppData(() =>
    json(200, { limits: { articles: { max_bytes: 50_000, max_images: 4 } }, article_authoring: "everyone" }),
  );
  await data.queryClient.query(instanceQuery(data));
  expect(probe(data, useArticleLimits)).toEqual({ max_bytes: 50_000, max_images: 4 });
});

/** The data of a reader with `permissions`, on a server whose `article_authoring` is `authoring`. */
async function reader(authoring: "everyone" | "permission", permissions: Permission[]) {
  const { data } = fakeAppData((request) => {
    const path = new URL(request.url).pathname;
    if (path === "/api/v1/accounts/verify_credentials")
      return json(200, { id: "1", username: "yuki", acct: "yuki", permissions });
    if (path === "/api/v1/instance") return json(200, { article_authoring: authoring });
    return json(404, { status: 404, title: "Not Found" });
  });
  await data.queryClient.query(viewerQuery(data));
  await data.queryClient.query(instanceQuery(data));
  return data;
}

test("everyone writes articles on a server that lets everyone", async () => {
  expect(probe(await reader("everyone", []), () => useArticleAuthoring())).toBe(true);
});

test("on a server that keeps articles to a permission, only a reader with create_articles writes them", async () => {
  expect(probe(await reader("permission", ["create_articles"]), () => useArticleAuthoring())).toBe(true);
  expect(probe(await reader("permission", ["manage_reports"]), () => useArticleAuthoring())).toBe(false);
});

test("whether the reader may write articles is not known before the server said", () => {
  const { data } = fakeAppData(() => new Promise<Response>(() => undefined));
  expect(probe(data, () => useArticleAuthoring())).toBeUndefined();
});

test("a preview sends the body, and the summary only when one is written", async () => {
  const sent: unknown[] = [];
  const { data } = fakeAppData(async (request) => {
    sent.push([line(request), await request.json()]);
    return json(200, preview);
  });
  expect(await previewArticle(data, { text: "## Day one\n\nFlour and water.", summary: "" })).toEqual(preview);
  await previewArticle(data, { text: "Flour.", summary: "A jar." });
  expect(sent).toEqual([
    ["POST /articles/preview", { text: "## Day one\n\nFlour and water." }],
    ["POST /articles/preview", { text: "Flour.", summary: "A jar." }],
  ]);
});

test("a preview the server could not render rejects", async () => {
  const { data } = fakeAppData(() => json(500, { status: 500, title: "Internal Server Error" }));
  const outcome = await previewArticle(data, { text: "Flour.", summary: "" }).then(
    () => "rendered",
    () => "rejected",
  );
  expect(outcome).toBe("rejected");
});

test("an article is posted with its idempotency key, the same on a retry, and tops the home timeline as a post does", async () => {
  const keys: (string | null)[] = [];
  const bodies: unknown[] = [];
  const { data } = fakeAppData(async (request) => {
    if (request.method === "POST") {
      expect(line(request)).toBe("POST /articles");
      keys.push(request.headers.get("Idempotency-Key"));
      bodies.push(await request.json());
      return json(201, status("new", { article: article("A sourdough starter") }));
    }
    return json(200, page(["1", "2"]));
  });
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  const body = { title: "A sourdough starter", text: "Flour and water.", visibility: "public" as const };
  const posted = await postArticle(data, body, "key-1");
  expect(posted.id).toBe("new");
  await postArticle(data, body, "key-1");
  expect(keys).toEqual(["key-1", "key-1"]);
  expect(bodies[0]).toEqual(body);
  expect(ids(data)?.[0]).toBe("new");
  expect(cachedStatus(data, "new")).toBeDefined();
});

test("an edit of an article is PUT to it, and every copy shows the article as the server answered", async () => {
  const seen: string[] = [];
  const { data } = fakeAppData((request) => {
    seen.push(line(request));
    if (request.method === "PUT")
      return json(200, status("1", { article: article("Seven days of a starter"), content: "<p>New.</p>" }));
    return json(200, { items: [status("1", { article: article("A sourdough starter") }), status("2")] });
  });
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  seen.length = 0;
  await editArticle(data, "1", { title: "Seven days of a starter", text: "New." });
  expect(seen).toEqual(["PUT /articles/1"]);
  expect(cachedStatus(data, "1")?.article?.title).toBe("Seven days of a starter");
  expect(cachedStatus(data, "1")?.content).toBe("<p>New.</p>");
});

test("an edit the server refuses rejects, and every copy stays as it was", async () => {
  const { data } = fakeAppData((request) => {
    if (request.method === "PUT")
      return json(422, {
        type: "/api/v1/problems/status-rejected",
        title: "Rejected",
        status: 422,
        detail: "This article was not accepted.",
      });
    return json(200, { items: [status("1", { article: article("A sourdough starter") })] });
  });
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  const error = await editArticle(data, "1", { title: "Changed", text: "New." }).catch((e: unknown) => e);
  expect(error).toBeInstanceOf(Error);
  expect(cachedStatus(data, "1")?.article?.title).toBe("A sourdough starter");
  expect(cachedStatus(data, "1")?.content).toBe("<p>1</p>");
});

test("what the writer wrote of an article is read from its source", async () => {
  const source = {
    id: "1",
    text: "## Day one",
    spoiler_text: "",
    article: { ...article("A sourdough starter"), tags: ["sourdough"] },
  };
  const seen: string[] = [];
  const { data } = fakeAppData((request) => {
    seen.push(line(request));
    return json(200, source);
  });
  expect(await readArticleSource(data, "1")).toEqual(source as never);
  expect(seen).toEqual(["GET /statuses/1/source"]);
});

/** Data whose server previews every body, and the bodies it was asked to preview. */
function previewing(answer: () => Response = () => json(200, preview)) {
  const asked: unknown[] = [];
  const { data } = fakeAppData(async (request) => {
    asked.push(await request.json());
    return answer();
  });
  return { data, asked };
}

test("now asks for the preview of the body and summary given at once, and keeps it for the hook", async () => {
  const { data, asked } = previewing();
  const state = probe(data, () => useArticlePreview("Flour and water.", ""));
  expect(state.of).toBeUndefined();
  expect(await state.now("Flour and water.", "")).toEqual(preview);
  expect(await state.now("Flour and water.", "A jar.")).toEqual(preview);
  expect(asked).toEqual([{ text: "Flour and water." }, { text: "Flour and water.", summary: "A jar." }]);
  // Asked once, it comes from what is kept, and the hook's preview is of that very body.
  await state.now("Flour and water.", "");
  expect(asked).toHaveLength(2);
  const shown = probe(data, () => useArticlePreview("Flour and water.", ""));
  expect(shown.preview).toEqual(preview);
  expect(shown.of).toEqual({ text: "Flour and water.", summary: "" });
});

test("the preview shown is of no body while the one at hand is another's, or the body is empty", async () => {
  const { data, asked } = previewing();
  await probe(data, () => useArticlePreview("", "")).now("Flour and water.", "");
  expect(probe(data, () => useArticlePreview("Flour and water, then a lid.", "")).of).toBeUndefined();
  expect(probe(data, () => useArticlePreview("Flour and water.", "A jar.")).of).toBeUndefined();
  const empty = probe(data, () => useArticlePreview("  ", ""));
  expect(empty.preview).toBeUndefined();
  expect(empty.of).toBeUndefined();
  expect(asked).toHaveLength(1);
});

test("now rejects when the server could not render the body", async () => {
  const { data } = previewing(() => json(500, { status: 500, title: "Internal Server Error" }));
  const outcome = await probe(data, () => useArticlePreview("", ""))
    .now("Flour.", "")
    .then(
      () => "rendered",
      () => "rejected",
    );
  expect(outcome).toBe("rejected");
});
