// SPDX-License-Identifier: MIT
import { describe, expect, test } from "bun:test";
import { createClient } from "./client.js";
import { NetworkError, ProblemError, problemTypeAboutBlank } from "./problem.js";
import { fakeFetch, fakeSleep, json, problem, server, statusPage } from "./test/fake.js";
import { staticToken, type TokenSource } from "./token.js";
import { sleepSignal } from "./transport.js";

const createStatusBody = { text: "hello", visibility: "public" as const };

describe("requests", () => {
  test("go to the client API with the token, Accept-Language, and the server's origin", async () => {
    const f = fakeFetch(() => json(200, statusPage(["1"])));
    const api = createClient({ server: `${server}/`, fetch: f.fetch, tokenSource: staticToken("t1"), acceptLanguage: "ja" });
    const { data } = await api.GET("/timelines/home", { params: { query: { limit: 5 } } });
    expect(data?.items).toHaveLength(1);
    const [sent] = f.requests;
    expect(sent?.request.url).toBe(`${server}/api/v1/timelines/home?limit=5`);
    expect(sent?.request.headers.get("Authorization")).toBe("Bearer t1");
    expect(sent?.request.headers.get("Accept-Language")).toBe("ja");
  });

  test("read Accept-Language for every request and keep the caller's", async () => {
    const f = fakeFetch(() => json(200, statusPage([])));
    let locale = "en";
    const api = createClient({ server, fetch: f.fetch, acceptLanguage: () => locale });
    await api.GET("/timelines/home");
    locale = "ar";
    await api.GET("/timelines/home");
    await api.GET("/timelines/home", { headers: { "Accept-Language": "fr" } });
    expect(f.requests.map((r) => r.request.headers.get("Accept-Language"))).toEqual(["en", "ar", "fr"]);
    expect(f.requests[0]?.request.headers.has("Authorization")).toBe(false);
  });

  test("reject a server that is not an origin", () => {
    for (const s of ["", "example.com", "ftp://example.com", "https://example.com/api", "https://example.com/?a=1"]) {
      expect(() => createClient({ server: s })).toThrow();
    }
  });
});

describe("errors", () => {
  test("an error answer rejects with its problem, extensions included", async () => {
    const f = fakeFetch(() => problem(422, "/api/v1/problems/media-rejected", { detail: "too large", reason: "size" }, { "Retry-After": "3" }));
    const api = createClient({ server, fetch: f.fetch });
    const error: unknown = await api.POST("/statuses", { body: createStatusBody }).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ProblemError);
    const p = error as ProblemError;
    expect(p.status).toBe(422);
    expect(p.type).toBe("/api/v1/problems/media-rejected");
    expect(p.title).toBe("title of /api/v1/problems/media-rejected");
    expect(p.detail).toBe("too large");
    expect(p.problem["reason"]).toBe("size");
    expect(p.headers.get("Retry-After")).toBe("3");
    expect(p.message).toBe("mykit: 422 /api/v1/problems/media-rejected: title of /api/v1/problems/media-rejected: too large");
  });

  test("an answer that is no problem document becomes about:blank with its status", async () => {
    const f = fakeFetch(() => new Response("<html>bad gateway</html>", { status: 502, statusText: "Bad Gateway", headers: { "Content-Type": "text/html" } }));
    const api = createClient({ server, fetch: f.fetch });
    const error: unknown = await api.GET("/timelines/home").catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ProblemError);
    expect((error as ProblemError).type).toBe(problemTypeAboutBlank);
    expect((error as ProblemError).status).toBe(502);
    expect((error as ProblemError).title).toBe("Bad Gateway");
    expect((error as ProblemError).detail).toBeUndefined();
  });

  test("the status of the answer wins over the document's", async () => {
    const f = fakeFetch(() => json(404, { type: "/api/v1/problems/x", title: "X", status: 200 }));
    const api = createClient({ server, fetch: f.fetch });
    const error = (await api.GET("/timelines/home").catch((e: unknown) => e)) as ProblemError;
    expect(error.status).toBe(404);
  });

  test("a failed fetch rejects with a NetworkError, an aborted one with its AbortError", async () => {
    const cause = new TypeError("connection refused");
    const api = createClient({ server, fetch: () => Promise.reject(cause) });
    const error: unknown = await api.GET("/timelines/home").catch((e: unknown) => e);
    expect(error).toBeInstanceOf(NetworkError);
    expect((error as NetworkError).cause).toBe(cause);
    expect((error as NetworkError).method).toBe("GET");

    const aborting = createClient({ server, fetch: (r) => Promise.reject(r.signal.reason as Error) });
    const controller = new AbortController();
    controller.abort();
    const aborted: unknown = await aborting.GET("/timelines/home", { signal: controller.signal }).catch((e: unknown) => e);
    expect(aborted).toBeInstanceOf(DOMException);
    expect((aborted as DOMException).name).toBe("AbortError");
  });
});

/** A TokenSource whose refresh hands out t2, counting the calls. */
function countingSource(): TokenSource & { refreshes: string[] } {
  let current = "t1";
  const refreshes: string[] = [];
  return {
    refreshes,
    getAccessToken: () => Promise.resolve(current),
    refresh: (refused) => {
      refreshes.push(refused);
      current = "t2";
      return Promise.resolve(current);
    },
  };
}

describe("refresh after a 401", () => {
  test("refreshes once and sends the request again with the new token and the same body", async () => {
    const f = fakeFetch((r) => (r.headers.get("Authorization") === "Bearer t2" ? json(200, { id: "s1" }) : problem(401, "/api/v1/problems/invalid-token")));
    const tokens = countingSource();
    const api = createClient({ server, fetch: f.fetch, tokenSource: tokens });
    const { data } = await api.POST("/statuses", { body: createStatusBody });
    expect(data?.id).toBe("s1");
    expect(tokens.refreshes).toEqual(["t1"]);
    expect(f.requests).toHaveLength(2);
    expect(f.requests[1]?.body).toBe(f.requests[0]?.body ?? "");
    expect(JSON.parse(f.requests[1]?.body ?? "")).toEqual(createStatusBody);
  });

  test("a second 401 stands", async () => {
    const f = fakeFetch(() => problem(401, "/api/v1/problems/invalid-token"));
    const tokens = countingSource();
    const api = createClient({ server, fetch: f.fetch, tokenSource: tokens });
    const error = (await api.GET("/timelines/home").catch((e: unknown) => e)) as ProblemError;
    expect(error.status).toBe(401);
    expect(tokens.refreshes).toEqual(["t1"]);
    expect(f.requests).toHaveLength(2);
  });

  test("no refresh without a token, or with a source that cannot refresh", async () => {
    const f = fakeFetch(() => problem(401, "/api/v1/problems/unauthenticated"));
    for (const tokenSource of [undefined, staticToken("t1")]) {
      const api = createClient({ server, fetch: f.fetch, ...(tokenSource === undefined ? {} : { tokenSource }) });
      const error = (await api.GET("/timelines/home").catch((e: unknown) => e)) as ProblemError;
      expect(error.status).toBe(401);
    }
    expect(f.requests).toHaveLength(2);
  });

  test("a refresh that gives no token leaves the 401", async () => {
    const f = fakeFetch(() => problem(401, "/api/v1/problems/invalid-token"));
    const api = createClient({
      server,
      fetch: f.fetch,
      tokenSource: { getAccessToken: () => Promise.resolve("t1"), refresh: () => Promise.resolve(null) },
    });
    const error = (await api.GET("/timelines/home").catch((e: unknown) => e)) as ProblemError;
    expect(error.status).toBe(401);
    expect(f.requests).toHaveLength(1);
  });

  test("a failed refresh rejects with its error", async () => {
    const f = fakeFetch(() => problem(401, "/api/v1/problems/invalid-token"));
    const failure = new Error("refresh failed");
    const api = createClient({
      server,
      fetch: f.fetch,
      tokenSource: { getAccessToken: () => Promise.resolve("t1"), refresh: () => Promise.reject(failure) },
    });
    expect(await api.GET("/timelines/home").catch((e: unknown) => e)).toBe(failure);
  });
});

describe("retries after 429 and 503", () => {
  test("a GET waits for Retry-After and is sent again", async () => {
    let n = 0;
    const f = fakeFetch(() => (++n === 1 ? problem(429, "/api/v1/problems/rate-limited", {}, { "Retry-After": "2" }) : json(200, statusPage(["1"]))));
    const s = fakeSleep();
    const api = createClient({ server, fetch: f.fetch, sleep: s.sleep });
    const { data } = await api.GET("/timelines/home");
    expect(data?.items).toHaveLength(1);
    expect(s.waits).toEqual([2000]);
    expect(f.requests).toHaveLength(2);
  });

  test("Retry-After as an HTTP date", async () => {
    let n = 0;
    const at = new Date(Date.now() + 5000).toUTCString();
    const f = fakeFetch(() => (++n === 1 ? problem(503, "about:blank", {}, { "Retry-After": at }) : json(200, statusPage([]))));
    const s = fakeSleep();
    const api = createClient({ server, fetch: f.fetch, sleep: s.sleep });
    await api.GET("/timelines/home");
    expect(s.waits).toHaveLength(1);
    expect(s.waits[0]).toBeGreaterThan(3000);
    expect(s.waits[0]).toBeLessThanOrEqual(5000);
  });

  test("a POST without an Idempotency-Key is not retried", async () => {
    const f = fakeFetch(() => problem(429, "/api/v1/problems/rate-limited", {}, { "Retry-After": "1" }));
    const s = fakeSleep();
    const api = createClient({ server, fetch: f.fetch, sleep: s.sleep });
    const error = (await api.POST("/accounts/{id}/follow", { params: { path: { id: "a1" } } }).catch((e: unknown) => e)) as ProblemError;
    expect(error.status).toBe(429);
    expect(f.requests).toHaveLength(1);
    expect(f.requests[0]?.request.headers.has("Idempotency-Key")).toBe(false);
    expect(s.waits).toEqual([]);
  });

  test("a POST with the automatic Idempotency-Key is retried with the same key", async () => {
    let n = 0;
    const f = fakeFetch(() => (++n === 1 ? problem(503, "about:blank", {}, { "Retry-After": "1" }) : json(200, { id: "s1" })));
    const s = fakeSleep();
    const api = createClient({ server, fetch: f.fetch, sleep: s.sleep });
    await api.POST("/statuses", { body: createStatusBody });
    const keys = f.requests.map((r) => r.request.headers.get("Idempotency-Key"));
    expect(keys).toHaveLength(2);
    expect(keys[0]).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/);
    expect(keys[1]).toBe(keys[0] ?? "");
  });

  test("the caller's Idempotency-Key is kept, and every request gets a new automatic one", async () => {
    const f = fakeFetch(() => json(200, { id: "s1" }));
    const api = createClient({ server, fetch: f.fetch });
    await api.POST("/statuses", { body: createStatusBody, params: { header: { "Idempotency-Key": "mine" } } });
    await api.POST("/statuses", { body: createStatusBody });
    await api.POST("/statuses", { body: createStatusBody });
    const keys = f.requests.map((r) => r.request.headers.get("Idempotency-Key"));
    expect(keys[0]).toBe("mine");
    expect(keys[1]).not.toBe(keys[2] ?? "");
  });

  test("an Idempotency-Key on an operation that takes none does not make it retried", async () => {
    const f = fakeFetch(() => problem(429, "/api/v1/problems/rate-limited", {}, { "Retry-After": "1" }));
    const s = fakeSleep();
    const api = createClient({ server, fetch: f.fetch, sleep: s.sleep });
    const error = (await api
      .POST("/accounts/{id}/follow", { params: { path: { id: "a1" } }, headers: { "Idempotency-Key": "mine" } })
      .catch((e: unknown) => e)) as ProblemError;
    expect(error.status).toBe(429);
    expect(f.requests).toHaveLength(1);
    expect(s.waits).toEqual([]);
  });

  test("a post that got no answer reports its Idempotency-Key, to send it again with", async () => {
    const f = fakeFetch(() => Promise.reject(new TypeError("connection reset")));
    const api = createClient({ server, fetch: f.fetch });
    const error = (await api.POST("/statuses", { body: createStatusBody }).catch((e: unknown) => e)) as NetworkError;
    expect(error).toBeInstanceOf(NetworkError);
    expect(error.idempotencyKey).toBe(f.requests[0]?.request.headers.get("Idempotency-Key") ?? "");
    expect(error.idempotencyKey).not.toBe("");
    const get = (await api.GET("/timelines/home").catch((e: unknown) => e)) as NetworkError;
    expect(get.idempotencyKey).toBeUndefined();
  });

  test("retries can be turned off", async () => {
    const f = fakeFetch(() => problem(429, "/api/v1/problems/rate-limited", {}, { "Retry-After": "1" }));
    const api = createClient({ server, fetch: f.fetch, retry: { disabled: true } });
    const error = (await api.GET("/timelines/home").catch((e: unknown) => e)) as ProblemError;
    expect(error.status).toBe(429);
    expect(f.requests).toHaveLength(1);
  });

  test("a Retry-After beyond maxWait is not waited for", async () => {
    const f = fakeFetch(() => problem(429, "/api/v1/problems/rate-limited", {}, { "Retry-After": "120" }));
    const s = fakeSleep();
    const api = createClient({ server, fetch: f.fetch, sleep: s.sleep });
    const error = (await api.GET("/timelines/home").catch((e: unknown) => e)) as ProblemError;
    expect(error.status).toBe(429);
    expect(error.headers.get("Retry-After")).toBe("120");
    expect(s.waits).toEqual([]);
  });

  test("stops after maxRetries, backing off without Retry-After", async () => {
    const f = fakeFetch(() => problem(503, "about:blank"));
    const s = fakeSleep();
    const api = createClient({ server, fetch: f.fetch, sleep: s.sleep, retry: { maxRetries: 2 } });
    const error = (await api.GET("/timelines/home").catch((e: unknown) => e)) as ProblemError;
    expect(error.status).toBe(503);
    expect(f.requests).toHaveLength(3);
    expect(s.waits).toHaveLength(2);
    expect(s.waits[0]).toBeGreaterThanOrEqual(1000);
    expect(s.waits[0]).toBeLessThan(1500);
    expect(s.waits[1]).toBeGreaterThanOrEqual(2000);
    expect(s.waits[1]).toBeLessThan(3000);
  });

  test("an abort while waiting rejects with its reason", async () => {
    const f = fakeFetch(() => problem(429, "/api/v1/problems/rate-limited", {}, { "Retry-After": "30" }));
    const api = createClient({ server, fetch: f.fetch });
    const controller = new AbortController();
    const pending = api.GET("/timelines/home", { signal: controller.signal }).catch((e: unknown) => e);
    await Bun.sleep(10);
    controller.abort();
    const error = await pending;
    expect(error).toBeInstanceOf(DOMException);
    expect(f.requests).toHaveLength(1);
  });
});

describe("sleepSignal", () => {
  test("a wait longer than setTimeout holds does not end at once", async () => {
    const controller = new AbortController();
    let ended = false;
    const sleeping = sleepSignal(1e15, controller.signal).then(
      () => (ended = true),
      () => undefined,
    );
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(ended).toBe(false);
    controller.abort();
    await sleeping;
  });
});
