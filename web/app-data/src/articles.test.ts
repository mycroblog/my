// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { scenario, type FakeOptions } from "@my/e2e/fake";
import { viewerQuery } from "./accounts";
import type { AppData } from "./app-data";
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
import { cachedStatus, shadowed } from "./shadow";
import type { Status } from "./statuses";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { statusQuery } from "./thread";
import { homeTimelineQuery } from "./timeline";

const ids = (data: AppData) =>
  data.queryClient.getQueryData(homeTimelineQuery(data).queryKey)?.pages[0]?.items.map((s) => s.id);

/** A server that holds an article of the reader's and a post of theirs after it. */
const written: FakeOptions = {
  seed: {
    people: scenario.people,
    posts: [
      {
        key: "article",
        author: "yuki",
        text: "## Day one\n\nFlour and water.",
        visibility: "public",
        language: "en",
        article: { title: "A sourdough starter", summary: "A diary of a jar of flour." },
      },
      { key: "post", author: "yuki", text: "The first loaf.", visibility: "public", language: "en" },
    ],
  },
};

test("the limits of an article are those of FR-123 until the server says its own", () => {
  expect(defaultArticleLimits).toEqual({ max_bytes: 100_000, max_images: 20 });
  expect(articleFieldLimits).toEqual({ title: 200, summary: 500, tags: 10 });
  const { data } = fakeData();
  expect(probe(data, useArticleLimits)).toEqual(defaultArticleLimits);
});

test("the limits of an article are the server's once it answered", async () => {
  const { fake, data } = fakeData();
  fake.script("GET /instance", {
    change: (instance) => ({
      ...instance,
      limits: { ...instance.limits, articles: { max_bytes: 50_000, max_images: 4 } },
    }),
  });
  await data.queryClient.query(instanceQuery(data));
  expect(probe(data, useArticleLimits)).toEqual({ max_bytes: 50_000, max_images: 4 });
});

/** The data of the reader `viewer`, whose rights and instance are read, on a server of `options`. */
async function reader(viewer: string, options: FakeOptions, grant = false) {
  const { fake, data } = fakeData({ viewer, ...options });
  // No person of the scenario holds a role that writes articles: the server grants this reader the permission.
  if (grant)
    fake.script("GET /accounts/verify_credentials", {
      change: (account) => ({ ...account, permissions: [...account.permissions, "create_articles" as const] }),
    });
  await data.queryClient.query(viewerQuery(data));
  await data.queryClient.query(instanceQuery(data));
  return data;
}

test("everyone writes articles on a server that lets everyone", async () => {
  expect(probe(await reader("yuki", {}), () => useArticleAuthoring())).toBe(true);
});

test("on a server that keeps articles to a permission, only a reader with create_articles writes them", async () => {
  const kept: FakeOptions = { articles: { authoring: "permission" } };
  expect(probe(await reader("yuki", kept, true), () => useArticleAuthoring())).toBe(true);
  // marco, a moderator, manages reports and writes no articles.
  expect(probe(await reader("marco", kept), () => useArticleAuthoring())).toBe(false);
});

test("whether the reader may write articles is not known before the server said", () => {
  const { data } = fakeData();
  expect(probe(data, () => useArticleAuthoring())).toBeUndefined();
});

test("a preview sends the body, and the summary only when one is written", async () => {
  const { fake, data, requests } = fakeData();
  const asked = fake.script("POST /articles/preview");
  const preview = await previewArticle(data, { text: "## Day one\n\nFlour and water.", summary: "" });
  expect(preview).toMatchObject({ summary: "Flour and water.", summary_from_body: true, max_bytes: 100_000 });
  expect(preview.content).toContain("Flour and water.");
  await previewArticle(data, { text: "Flour.", summary: "A jar." });
  expect(requests).toEqual(["POST /articles/preview", "POST /articles/preview"]);
  expect(asked.seen.map((s) => s.body)).toEqual([
    { text: "## Day one\n\nFlour and water." },
    { text: "Flour.", summary: "A jar." },
  ]);
});

test("a preview the server could not render rejects", async () => {
  const { data } = fakeData({ articles: { preview: "fails" } });
  const outcome = await previewArticle(data, { text: "Flour.", summary: "" }).then(
    () => "rendered",
    () => "rejected",
  );
  expect(outcome).toBe("rejected");
});

test("an article is posted with its idempotency key, the same on a retry, and tops the home timeline as a post does", async () => {
  const { fake, data } = fakeData(written);
  const sent = fake.script("POST /articles");
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  const body = { title: "Seven days of a starter", text: "Flour and water.", visibility: "public" as const };
  const posted = await postArticle(data, body, "key-1");
  expect(posted.article?.title).toBe("Seven days of a starter");
  expect((await postArticle(data, body, "key-1")).id).toBe(posted.id);
  expect(sent.seen.map((s) => s.headers.get("Idempotency-Key"))).toEqual(["key-1", "key-1"]);
  expect(sent.seen[0]?.body).toEqual(body);
  expect(ids(data)).toEqual([posted.id, fake.idOf("post"), fake.idOf("article")]);
  expect(cachedStatus(data, posted.id)).toBeDefined();
});

test("an edit of an article is PUT to it, and every copy shows the article as the server answered", async () => {
  const { fake, data, requests } = fakeData(written);
  const id = fake.idOf("article");
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  await data.queryClient.query(statusQuery(data, id));
  requests.length = 0;
  await editArticle(data, id, { title: "Seven days of a starter", text: "New." });
  expect(requests).toEqual([`PUT /articles/${id}`]);
  // The copy of a list holds no body (ADR-0160 §8); the article read alone does.
  const listed = data.queryClient
    .getQueryData(homeTimelineQuery(data).queryKey)
    ?.pages[0]?.items.find((s) => s.id === id);
  const alone = data.queryClient.getQueryData<Status>(statusQuery(data, id).queryKey);
  const shown = [listed, alone].map((copy) => copy && shadowed(data, copy));
  expect(shown.map((copy) => copy?.article?.title)).toEqual(["Seven days of a starter", "Seven days of a starter"]);
  expect(shown.map((copy) => copy?.content)).toEqual(["", "<p>New.</p>"]);
});

test("an edit the server refuses rejects, and every copy stays as it was", async () => {
  const { fake, data } = fakeData({ ...written, articles: { publish: "refused" } });
  const id = fake.idOf("article");
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  const before = cachedStatus(data, id);
  const error = await editArticle(data, id, { title: "Changed", text: "New." }).catch((e: unknown) => e);
  expect(error).toBeInstanceOf(Error);
  expect(before?.article?.title).toBe("A sourdough starter");
  expect(cachedStatus(data, id)).toEqual(before);
});

test("what the writer wrote of an article is read from its source", async () => {
  const { fake, data, requests } = fakeData(written);
  const id = fake.idOf("article");
  expect(await readArticleSource(data, id)).toEqual({
    id,
    text: "## Day one\n\nFlour and water.",
    spoiler_text: "",
    language: "en",
    article: { title: "A sourdough starter", summary: "A diary of a jar of flour.", tags: [] },
  });
  expect(requests).toEqual([`GET /statuses/${id}/source`]);
});

/** Data whose server previews every body, and the bodies it was asked to preview. */
function previewing(options: FakeOptions = {}) {
  const { fake, data } = fakeData(options);
  const asked = fake.script("POST /articles/preview");
  return { data, asked: () => asked.seen.map((s) => s.body) };
}

test("now asks for the preview of the body and summary given at once, and keeps it for the hook", async () => {
  const { data, asked } = previewing();
  const state = probe(data, () => useArticlePreview("Flour and water.", ""));
  expect(state.of).toBeUndefined();
  const preview = await state.now("Flour and water.", "");
  expect(preview).toMatchObject({ summary: "Flour and water.", summary_from_body: true });
  expect(await state.now("Flour and water.", "A jar.")).toMatchObject({ summary: "A jar.", summary_from_body: false });
  expect(asked()).toEqual([{ text: "Flour and water." }, { text: "Flour and water.", summary: "A jar." }]);
  // Asked once, it comes from what is kept, and the hook's preview is of that very body.
  await state.now("Flour and water.", "");
  expect(asked()).toHaveLength(2);
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
  expect(asked()).toHaveLength(1);
});

test("now rejects when the server could not render the body", async () => {
  const { data } = previewing({ articles: { preview: "fails" } });
  const outcome = await probe(data, () => useArticlePreview("", ""))
    .now("Flour.", "")
    .then(
      () => "rendered",
      () => "rejected",
    );
  expect(outcome).toBe("rejected");
});
