// SPDX-License-Identifier: AGPL-3.0-only
// The tests of the site's Worker (worker/index.ts, worker/release-notes.ts), with the assets, the cache and GitHub
// replaced by fakes. Bun runs HTMLRewriter as the Workers runtime does.
import assert from "node:assert/strict";
import { test } from "node:test";
import { type Env, handle, type Runtime } from "./index";
import { isReleaseNotesPath, sanitize } from "./release-notes";

const page = `<!DOCTYPE html><html><body><main><div id="release-notes"><p>On <a href="https://github.com/mycroblog/my/releases">GitHub Releases</a>.</p></div></main></body></html>`;

function assets(): Env["ASSETS"] & { requests: Request[] } {
  const requests: Request[] = [];
  return {
    requests,
    fetch(request) {
      requests.push(request);
      return Promise.resolve(
        new Response(page, { headers: { "content-type": "text/html; charset=utf-8", etag: '"asset"' } }),
      );
    },
  };
}

function memoryCache(): WorkerCache {
  const entries = new Map<string, string>();
  return {
    match(request) {
      const body = entries.get(request.url);
      return Promise.resolve(body === undefined ? undefined : new Response(body));
    },
    async put(request, response) {
      entries.set(request.url, await response.text());
    },
  };
}

const release = {
  tag_name: "v0.1.0",
  name: "v0.1.0",
  draft: false,
  prerelease: false,
  published_at: "2026-11-01T00:00:00Z",
  html_url: "https://github.com/mycroblog/my/releases/tag/v0.1.0",
  body_html: `<h2>What changed</h2><p onclick="x()">First <a href="javascript:alert(1)">release</a>.</p><script>alert(1)</script>`,
};

/** A fake of GitHub that gives `answers` in order, then 403 as when the rate limit is reached. */
function github(answers: Response[]): Runtime["fetch"] & { requests: Request[] } {
  const requests: Request[] = [];
  const fn = (request: Request) => {
    requests.push(request);
    return Promise.resolve(answers.shift() ?? new Response("rate limited", { status: 403 }));
  };
  return Object.assign(fn, { requests });
}

function context(): WorkerContext & { waiting: Promise<unknown>[] } {
  const waiting: Promise<unknown>[] = [];
  return {
    waiting,
    waitUntil(promise) {
      waiting.push(promise);
    },
  };
}

const notesURL = "https://mycroblog.dev/next/releases/";

void test("the release notes page of a version, in English or a locale", () => {
  assert.equal(isReleaseNotesPath("/next/releases/"), true);
  assert.equal(isReleaseNotesPath("/v0.1/ja/releases/"), true);
  assert.equal(isReleaseNotesPath("/releases/"), false);
  assert.equal(isReleaseNotesPath("/next/guide/releases/x/"), false);
});

void test("the notes come from GitHub, then from the cache", async () => {
  const fetch = github([Response.json([release, { ...release, tag_name: "v0.2.0-rc.1", prerelease: true }])]);
  const runtime: Runtime = { cache: memoryCache(), fetch, now: () => 0 };
  const env = { ASSETS: assets(), GITHUB_TOKEN: "t" };

  const first = await handle(new Request(notesURL), env, context(), runtime);
  const html = await first.text();
  assert.ok(
    html.includes(
      `<h2 id="release-v0-1-0"><a href="https://github.com/mycroblog/my/releases/tag/v0.1.0">v0.1.0</a></h2>`,
    ),
  );
  assert.ok(html.includes("November 1, 2026"));
  assert.ok(!html.includes("rc.1"), "a prerelease is left out");
  assert.ok(!html.includes("GitHub Releases</a>.</p>"), "the placeholder's text is replaced");
  assert.equal(first.headers.get("etag"), null);
  assert.equal(fetch.requests.at(0)?.headers.get("authorization"), "Bearer t");
  assert.equal(fetch.requests.at(0)?.headers.get("accept"), "application/vnd.github.html+json");

  const second = await handle(new Request("https://mycroblog.dev/v0.1/ja/releases/"), env, context(), runtime);
  assert.ok((await second.text()).includes("release-v0-1-0"));
  assert.equal(fetch.requests.length, 1);
});

void test("a stale answer is served and refreshed in the background", async () => {
  let now = 0;
  const fetch = github([Response.json([release]), Response.json([{ ...release, tag_name: "v0.1.1", name: "v0.1.1" }])]);
  const runtime: Runtime = { cache: memoryCache(), fetch, now: () => now };
  const env = { ASSETS: assets() };
  await handle(new Request(notesURL), env, context(), runtime);

  now = 11 * 60 * 1000;
  const ctx = context();
  const stale = await handle(new Request(notesURL), env, ctx, runtime);
  assert.ok(!(await stale.text()).includes("v0.1.1"));
  await Promise.all(ctx.waiting);
  const fresh = await handle(new Request(notesURL), env, context(), runtime);
  assert.ok((await fresh.text()).includes("v0.1.1"));
  assert.equal(fetch.requests.at(0)?.headers.has("authorization"), false);
});

void test("the page is served as built when GitHub fails and nothing is cached", async () => {
  const runtime: Runtime = { cache: memoryCache(), fetch: github([]), now: () => 0 };
  const response = await handle(new Request(notesURL), { ASSETS: assets() }, context(), runtime);
  assert.equal(await response.text(), page);
});

void test("every page of the Link header is read", async () => {
  const next = "https://api.github.com/repositories/1/releases?per_page=100&page=2";
  const fetch = github([
    Response.json([release], { headers: { link: `<${next}>; rel="next"` } }),
    Response.json([{ ...release, tag_name: "v0.0.9", name: "v0.0.9" }]),
  ]);
  const runtime: Runtime = { cache: memoryCache(), fetch, now: () => 0 };
  const response = await handle(new Request(notesURL), { ASSETS: assets() }, context(), runtime);
  assert.ok((await response.text()).includes("release-v0-0-9"));
  assert.equal(fetch.requests.at(1)?.url, next);
});

void test("the browser's validators of the asset are dropped", async () => {
  const env = { ASSETS: assets() };
  const runtime: Runtime = { cache: memoryCache(), fetch: github([Response.json([])]), now: () => 0 };
  const request = new Request(notesURL, { headers: { "if-none-match": '"asset"' } });
  const response = await handle(request, env, context(), runtime);
  assert.equal(env.ASSETS.requests.at(0)?.headers.has("if-none-match"), false);
  assert.ok((await response.text()).includes("No release has been published yet."));
});

void test("any other path is the asset's", async () => {
  const env = { ASSETS: assets() };
  const fetch = github([]);
  const runtime: Runtime = { cache: memoryCache(), fetch, now: () => 0 };
  const response = await handle(new Request("https://mycroblog.dev/next/guide/"), env, context(), runtime);
  assert.equal(await response.text(), page);
  assert.equal(fetch.requests.length, 0);
});

void test("the notes keep their markup and lose scripts, handlers and other URLs", async () => {
  assert.equal(await sanitize(release.body_html), `<h2>What changed</h2><p>First <a>release</a>.</p>`);
  assert.equal(
    await sanitize(`<a href="https://example.com/" style="x">x</a><custom>y</custom><img src="data:x">`),
    `<a href="https://example.com/">x</a>y<img>`,
  );
});
