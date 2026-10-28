// SPDX-License-Identifier: AGPL-3.0-only
// The API client on @mycroblog/mykit against MSW: the session's Bearer token, the one refresh and retry after a 401,
// errors turned into localized ApiError / NetworkError objects, the server's maintenance, which keeps the session, and
// the event stream with the same token and reports.
import { onlineManager } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, test } from "vitest";
import { apiBase, oauthError, oauthPaths, problem, tokenResponse } from "../../test/msw/http";
import { instanceFixture, timelineStatusFixture, viewerFixture } from "../../test/fixtures";
import { eventStreamResponse } from "../../test/msw/streaming";
import { server } from "../../test/msw/server";
import { Session } from "../auth/session";
import { ApiError, MaintenanceError, NetworkError, rateLimitedUntil } from "../errors";
import { loadI18n } from "../i18n";
import { AppLocale } from "../locale";
import { Maintenance } from "../maintenance";
import { type Api, createApi } from "./client";

const apiOrigin = window.location.origin;
const sessions: Session[] = [];

afterEach(() => {
  for (const session of sessions.splice(0)) session.dispose();
});

async function setUp(locale = "en") {
  const reloads = { count: 0 };
  const maintenance = new Maintenance({
    reload: () => {
      reloads.count++;
    },
  });
  const session = new Session({ apiOrigin, maintenance });
  sessions.push(session);
  const i18n = await loadI18n(locale);
  return {
    session,
    maintenance,
    reloads,
    api: createApi({ apiOrigin, session, locale: new AppLocale(i18n), maintenance }),
  };
}

test("resumes the session before the first request and sends its token as a Bearer token", async () => {
  server.use(
    http.post(oauthPaths.refresh, () => tokenResponse("access-1")),
    http.get(`${apiBase}/accounts/verify_credentials`, ({ request }) =>
      request.headers.get("Authorization") === "Bearer access-1"
        ? HttpResponse.json(viewerFixture)
        : problem(401, "/api/v1/problems/invalid-token", "Invalid token"),
    ),
  );
  const { api, session } = await setUp();

  const { data } = await api.fetch.GET("/accounts/verify_credentials");

  expect(data).toEqual(viewerFixture);
  expect(session.status).toBe("signed-in");
});

test("after a 401 refreshes the token once and sends the request again with the new one", async () => {
  let refreshes = 0;
  const authorizations: (string | null)[] = [];
  server.use(
    http.post(oauthPaths.refresh, () => {
      refreshes++;
      return tokenResponse(`access-${refreshes}`);
    }),
    http.get(`${apiBase}/accounts/verify_credentials`, ({ request }) => {
      authorizations.push(request.headers.get("Authorization"));
      return request.headers.get("Authorization") === "Bearer access-2"
        ? HttpResponse.json(viewerFixture)
        : problem(401, "/api/v1/problems/invalid-token", "Invalid token");
    }),
  );
  const { api } = await setUp();

  const { data } = await api.fetch.GET("/accounts/verify_credentials");

  expect(data).toEqual(viewerFixture);
  expect(authorizations).toEqual(["Bearer access-1", "Bearer access-2"]);
  expect(refreshes).toBe(2);
});

test("a second 401 stands as an ApiError, and a refused refresh signs out", async () => {
  let refreshes = 0;
  server.use(
    http.post(oauthPaths.refresh, () => {
      refreshes++;
      return refreshes === 1 ? tokenResponse("access-1") : oauthError("invalid_grant");
    }),
    http.get(`${apiBase}/accounts/verify_credentials`, () =>
      problem(401, "/api/v1/problems/invalid-token", "The access token is invalid."),
    ),
  );
  const { api, session } = await setUp();

  const failure = api.fetch.GET("/accounts/verify_credentials");

  await expect(failure).rejects.toBeInstanceOf(ApiError);
  await expect(failure).rejects.toMatchObject({
    status: 401,
    type: "/api/v1/problems/invalid-token",
    message: "The access token is invalid.",
  });
  expect(session.status).toBe("signed-out");
  expect(refreshes).toBe(2);
});

test("sends the page's locale and keeps the server's localized problem as the message", async () => {
  server.use(
    http.get(`${apiBase}/instance`, ({ request }) =>
      HttpResponse.json(
        {
          type: "about:blank",
          title: "サービスを利用できません",
          detail: `${request.headers.get("Accept-Language") ?? ""} で説明しています`,
          status: 503,
        },
        { status: 503, headers: { "Content-Type": "application/problem+json" } },
      ),
    ),
  );
  const { api } = await setUp("ja");

  await expect(api.fetch.GET("/instance")).rejects.toMatchObject({
    name: "ApiError",
    status: 503,
    title: "サービスを利用できません",
    message: "ja で説明しています",
  });
});

test("describes an error without a problem document from the client's catalog", async () => {
  server.use(
    http.get(
      `${apiBase}/instance`,
      () => new HttpResponse("<h1>Bad Gateway</h1>", { status: 502, headers: { "Content-Type": "text/html" } }),
    ),
  );
  const { api } = await setUp();

  await expect(api.fetch.GET("/instance")).rejects.toMatchObject({
    status: 502,
    type: "about:blank",
    message: "The server could not handle the request (error 502). Try again later.",
  });
});

test("keeps when a rate-limited request may be sent again, from Retry-After in seconds or as a date", async () => {
  const { api } = await setUp();
  const before = Date.now();
  server.use(
    http.get(`${apiBase}/instance`, () =>
      problem(429, "about:blank", "Too many requests.", { headers: { "Retry-After": "30" } }),
    ),
  );

  const inSeconds: unknown = await api.fetch.GET("/instance").catch((error: unknown) => error);

  expect(inSeconds).toBeInstanceOf(ApiError);
  expect(rateLimitedUntil(inSeconds)).toBeGreaterThanOrEqual(before + 30_000);
  expect(rateLimitedUntil(inSeconds)).toBeLessThanOrEqual(Date.now() + 30_000);

  server.use(
    http.get(`${apiBase}/instance`, () =>
      problem(429, "about:blank", "Too many requests.", {
        headers: { "Retry-After": "Wed, 21 Oct 2026 07:28:00 GMT" },
      }),
    ),
  );
  const atDate = await api.fetch.GET("/instance").catch((error: unknown) => error);
  expect(rateLimitedUntil(atDate)).toBe(Date.UTC(2026, 9, 21, 7, 28));

  // Without Retry-After, or for another status, nothing says when to try again.
  server.use(http.get(`${apiBase}/instance`, () => problem(429, "about:blank", "Too many requests.")));
  expect(rateLimitedUntil(await api.fetch.GET("/instance").catch((error: unknown) => error))).toBeUndefined();
  server.use(
    http.get(`${apiBase}/instance`, () =>
      problem(503, "about:blank", "Unavailable", { headers: { "Retry-After": "30" } }),
    ),
  );
  expect(rateLimitedUntil(await api.fetch.GET("/instance").catch((error: unknown) => error))).toBeUndefined();
});

test("turns a network failure into a localized NetworkError", async () => {
  server.use(http.get(`${apiBase}/instance`, () => HttpResponse.error()));
  const { api } = await setUp("ja");

  const failure = api.fetch.GET("/instance");

  await expect(failure).rejects.toBeInstanceOf(NetworkError);
  await expect(failure).rejects.toThrow("サーバーに接続できませんでした。接続を確かめて、もう一度試してください。");
});

test("answers without a token when signed out", async () => {
  let authorization: string | null = "unset";
  server.use(
    http.get(`${apiBase}/instance`, ({ request }) => {
      authorization = request.headers.get("Authorization");
      return HttpResponse.json(instanceFixture);
    }),
  );
  const { api, session } = await setUp();

  const { data } = await api.fetch.GET("/instance");

  expect(data).toEqual(instanceFixture);
  expect(authorization).toBeNull();
  expect(session.status).toBe("signed-out");
});

/** The 503 that the server's maintenance mode answers every operation and the OAuth endpoints with (ADR-0084 §3). */
function maintenanceResponse(members: Record<string, string> = {}, retryAfter = "600") {
  return HttpResponse.json(
    { type: "/api/v1/problems/maintenance", title: "Under maintenance", status: 503, ...members },
    {
      status: 503,
      headers: { "Content-Type": "application/problem+json", "Retry-After": retryAfter, "Cache-Control": "no-store" },
    },
  );
}

describe("the server's maintenance", () => {
  test("is a MaintenanceError with what the server says, keeps the session's token, and ends with the next answer", async () => {
    let underMaintenance = false;
    let refreshes = 0;
    server.use(
      http.post(oauthPaths.refresh, () => {
        refreshes++;
        return underMaintenance ? maintenanceResponse() : tokenResponse("access-1");
      }),
      http.get(`${apiBase}/accounts/verify_credentials`, ({ request }) => {
        if (underMaintenance)
          return maintenanceResponse(
            {
              message: "Upgrading the database.",
              until: "2026-10-25T12:00:00Z",
              status_page_url: "https://status.my.example/",
            },
            "30",
          );
        return request.headers.get("Authorization") === "Bearer access-1"
          ? HttpResponse.json(viewerFixture)
          : problem(401, "/api/v1/problems/invalid-token", "Invalid token");
      }),
    );
    const { api, session, maintenance, reloads } = await setUp();
    await api.fetch.GET("/accounts/verify_credentials");
    const signOuts: string[] = [];
    session.onUserChange((status) => signOuts.push(status));

    underMaintenance = true;
    const before = Date.now();
    const failure: unknown = await api.fetch.GET("/accounts/verify_credentials").catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(MaintenanceError);
    expect(failure).toBeInstanceOf(ApiError);
    expect(failure).toMatchObject({
      status: 503,
      type: "/api/v1/problems/maintenance",
      notice: "Upgrading the database.",
      until: Date.UTC(2026, 9, 25, 12),
      statusPageUrl: "https://status.my.example/",
    });
    expect((failure as MaintenanceError).retryAt).toBeGreaterThanOrEqual(before + 30_000);
    expect(maintenance.current).toBe(failure);
    // The 503 is no 401: the token stays, nothing refreshes, and the session goes on.
    expect(session.accessToken).toBe("access-1");
    expect(session.status).toBe("signed-in");
    expect(refreshes).toBe(1);
    expect(signOuts).toEqual([]);

    // The first answer after the maintenance loads the page again, once.
    underMaintenance = false;
    await api.fetch.GET("/accounts/verify_credentials");
    await api.fetch.GET("/accounts/verify_credentials");
    expect(reloads.count).toBe(1);
    expect(session.accessToken).toBe("access-1");
  });

  test("a refresh after a 401 that meets the maintenance keeps the token and throws the MaintenanceError", async () => {
    let underMaintenance = false;
    server.use(
      http.post(oauthPaths.refresh, () => (underMaintenance ? maintenanceResponse() : tokenResponse("access-1"))),
      http.get(`${apiBase}/accounts/verify_credentials`, () =>
        problem(401, "/api/v1/problems/invalid-token", "Invalid token"),
      ),
    );
    const { api, session, maintenance } = await setUp();
    await session.restore();
    underMaintenance = true;

    const failure = api.fetch.GET("/accounts/verify_credentials");

    await expect(failure).rejects.toBeInstanceOf(MaintenanceError);
    expect(maintenance.current).toBeInstanceOf(MaintenanceError);
    expect(session.accessToken).toBe("access-1");
    expect(session.status).toBe("signed-in");
  });

  test("resuming the session during the maintenance neither signs out nor revokes, and resumes after it", async () => {
    let underMaintenance = true;
    let revokes = 0;
    server.use(
      http.post(oauthPaths.refresh, () => (underMaintenance ? maintenanceResponse() : tokenResponse("access-1"))),
      http.post(oauthPaths.revoke, () => {
        revokes++;
        return new HttpResponse(null, { status: 200 });
      }),
    );
    const { session, maintenance } = await setUp();
    const changes: string[] = [];
    session.onUserChange((status) => changes.push(status));

    await session.restore();

    expect(maintenance.current).toBeInstanceOf(MaintenanceError);
    // As when the server cannot be reached: the tab waits for the server, and the next restore tries the cookie again.
    expect(session.unreachable).toBe(true);
    expect(changes).toEqual([]);
    expect(revokes).toBe(0);

    underMaintenance = false;
    await session.restore();
    expect(session.status).toBe("signed-in");
    expect(session.accessToken).toBe("access-1");
  });

  test("keeps only the well-formed members it shows", async () => {
    server.use(
      http.get(`${apiBase}/instance`, () =>
        maintenanceResponse({ message: "  ", until: "soon", status_page_url: "http://status.my.example/" }),
      ),
    );
    const { api } = await setUp();

    const failure: unknown = await api.fetch.GET("/instance").catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(MaintenanceError);
    expect(failure).toMatchObject({ notice: undefined, until: undefined, statusPageUrl: undefined });
  });

  test("another 503 is no maintenance", async () => {
    server.use(
      http.get(`${apiBase}/instance`, () =>
        problem(503, "about:blank", "Unavailable", { headers: { "Retry-After": "30" } }),
      ),
    );
    const { api, maintenance } = await setUp();

    const failure: unknown = await api.fetch.GET("/instance").catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(ApiError);
    expect(failure).not.toBeInstanceOf(MaintenanceError);
    expect(maintenance.current).toBeNull();
  });
});

test("leaves a 429 to the app instead of retrying it", async () => {
  let requests = 0;
  server.use(
    http.get(`${apiBase}/instance`, () => {
      requests++;
      return problem(429, "about:blank", "Too many requests.", { headers: { "Retry-After": "1" } });
    }),
  );
  const { api } = await setUp();

  await expect(api.fetch.GET("/instance")).rejects.toMatchObject({ name: "ApiError", status: 429 });
  expect(requests).toBe(1);
});

test("keeps the caller's Idempotency-Key", async () => {
  let key: string | null = null;
  server.use(
    http.post(oauthPaths.refresh, () => tokenResponse("access-1")),
    http.post(`${apiBase}/statuses`, ({ request }) => {
      key = request.headers.get("Idempotency-Key");
      return HttpResponse.json(timelineStatusFixture);
    }),
  );
  const { api } = await setUp();

  await api.fetch.POST("/statuses", {
    params: { header: { "Idempotency-Key": "draft-1" } },
    body: { text: "Hello", visibility: "public" },
  });

  expect(key).toBe("draft-1");
});

describe("the event stream", () => {
  type Seen = { opens: boolean[]; events: unknown[]; errors: unknown[]; online: boolean[] };

  /** Runs the stream of `api` until `until` holds for what it delivered, then closes it. */
  async function streamUntil(api: Api, until: (seen: Seen) => boolean): Promise<Seen> {
    const controller = new AbortController();
    const seen: Seen = { opens: [], events: [], errors: [], online: [] };
    await new Promise<void>((resolve, reject) => {
      const check = () => {
        if (until(seen)) resolve();
      };
      api
        .stream({
          signal: controller.signal,
          onOpen: (reconnected) => {
            seen.opens.push(reconnected);
            check();
          },
          onEvent: (event) => {
            seen.events.push(event);
            check();
          },
          onError: (error) => {
            seen.errors.push(error);
            seen.online.push(onlineManager.isOnline());
            check();
          },
        })
        .then(() => {
          reject(new Error("the stream ended"));
        }, reject);
    });
    controller.abort();
    return seen;
  }

  test("sends the session's token, refreshes it once after a 401, and passes on the events it reads", async () => {
    let refreshes = 0;
    const authorizations: (string | null)[] = [];
    server.use(
      http.post(oauthPaths.refresh, () => {
        refreshes++;
        return tokenResponse(`access-${String(refreshes)}`);
      }),
      http.get(`${apiBase}/streaming`, ({ request }) => {
        authorizations.push(request.headers.get("Authorization"));
        if (request.headers.get("Authorization") !== "Bearer access-2")
          return problem(401, "/api/v1/problems/invalid-token", "Invalid token");
        const { response, stream } = eventStreamResponse();
        stream.send("something_new", { id: "x" });
        stream.send("update", timelineStatusFixture);
        return response;
      }),
    );
    const { api } = await setUp();

    const seen = await streamUntil(api, ({ events }) => events.length >= 2);

    expect(authorizations).toEqual(["Bearer access-1", "Bearer access-2"]);
    expect(seen.opens).toEqual([false]);
    expect(seen.events).toEqual([
      { type: "unknown", event: "something_new", data: '{"id":"x"}' },
      { type: "update", status: timelineStatusFixture },
    ]);
  });

  test("reports a network failure as offline and the maintenance, and connects again after each", async () => {
    let attempts = 0;
    server.use(
      http.post(oauthPaths.refresh, () => tokenResponse("access-1")),
      http.get(`${apiBase}/streaming`, () => {
        attempts++;
        return attempts === 1 ? HttpResponse.error() : maintenanceResponse({}, "0");
      }),
    );
    const { api, maintenance } = await setUp();

    const seen = await streamUntil(api, ({ errors }) => errors.length >= 2);

    // Offline after the network failure, online again once the server answered.
    expect(seen.online).toEqual([false, true]);
    expect(maintenance.current).toBeInstanceOf(MaintenanceError);
    expect(attempts).toBe(2);
  });
});
