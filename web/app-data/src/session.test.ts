// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { createClient } from "@mycroblog/mykit";
import type { TabGroup } from "@my/platform";
import { openSession, SessionTokenSource } from "./session";
import { json, page } from "./test/fake";
import { fakeBrowser } from "./test/tabs";

const server = "https://example.com";

test("without a session the server refuses the cookie as an invalid grant, and there is no session", async () => {
  const requests: Request[] = [];
  const data = await openSession({
    server,
    fetch: (request) => {
      requests.push(request);
      return Promise.resolve(json(400, { error: "invalid_grant" }));
    },
  });
  expect(data).toBeNull();
  expect(requests).toHaveLength(1);
  expect(requests[0]?.method).toBe("POST");
  expect(requests[0]?.url).toBe("https://example.com/oauth/token/refresh");
  expect(requests[0]?.headers.get("X-My-Token-Refresh")).toBe("1");
});

test("with a session the client API gets the access token the cookie traded for", async () => {
  const authorizations: (string | null)[] = [];
  const data = await openSession({
    server,
    fetch: (request) => {
      if (new URL(request.url).pathname === "/oauth/token/refresh")
        return Promise.resolve(
          json(200, { access_token: "a1", token_type: "Bearer", expires_in: 3600, scope: "read" }),
        );
      authorizations.push(request.headers.get("Authorization"));
      return Promise.resolve(json(200, page(["1"])));
    },
  });
  expect(data).not.toBeNull();
  await data?.api.GET("/timelines/home");
  expect(authorizations).toEqual(["Bearer a1"]);
});

test("a server that cannot say rejects, rather than looking like no session", async () => {
  const fail = () => Promise.resolve(json(500, { type: "about:blank", title: "Internal Server Error", status: 500 }));
  const failure = await openSession({ server, fetch: fail }).then(
    () => null,
    (error: unknown) => error,
  );
  expect(failure).toBeInstanceOf(Error);
  expect(String(failure)).toContain("answered 500");
});

test("the token is traded again once it expires, and once for the requests that ask meanwhile", async () => {
  let now = 0;
  let trades = 0;
  const source = new SessionTokenSource(
    server,
    () => {
      trades += 1;
      return Promise.resolve(json(200, { access_token: `a${String(trades)}`, expires_in: 60 }));
    },
    { now: () => now },
  );
  expect(await Promise.all([source.getAccessToken(), source.getAccessToken()])).toEqual(["a1", "a1"]);
  expect(await source.getAccessToken()).toBe("a1");
  now = 60_000;
  expect(await source.getAccessToken()).toBe("a2");
  expect(await source.refresh("a2")).toBe("a3");
  expect(await source.refresh("a2")).toBe("a3");
  expect(trades).toBe(3);
});

/**
 * The server's side of the session: every refresh rotates the cookie and revokes the access token it replaces
 * (ADR-0090 §16), and the client API refuses a revoked token with a 401.
 */
function rotatingServer() {
  let refreshes = 0;
  let valid: string | null = null;
  let ended = false;
  const fetch = (request: Request): Promise<Response> => {
    if (new URL(request.url).pathname === "/oauth/token/refresh") {
      if (ended) return Promise.resolve(json(400, { error: "invalid_grant" }));
      refreshes += 1;
      valid = `t${String(refreshes)}`;
      return Promise.resolve(json(200, { access_token: valid, token_type: "Bearer", expires_in: 3600 }));
    }
    const token = request.headers.get("Authorization")?.replace("Bearer ", "");
    return Promise.resolve(token === valid ? json(200, page(["1"])) : json(401, { status: 401 }));
  };
  return {
    fetch,
    refreshes: () => refreshes,
    /** Whether the server still accepts token, as an open stream checks it at each heartbeat. */
    accepts: (token: string | null) => token !== null && token === valid,
    /** Revokes the access token without a refresh, as the server does on its own. */
    revoke: () => {
      valid = null;
    },
    /** Ends the session: the browser signed out. */
    end: () => {
      valid = null;
      ended = true;
    },
  };
}

/** A browser whose tabs share one cookie with the server: each call opens a tab and answers its token source. */
function browser(rotating: ReturnType<typeof rotatingServer>, now: () => number = Date.now) {
  const group = fakeBrowser();
  return () => new SessionTokenSource(server, rotating.fetch, { now, tabs: group.join() });
}

test("tabs that need a token at once cause one refresh, and every tab gets its token", async () => {
  let now = 0;
  const rotating = rotatingServer();
  const tab = browser(rotating, () => now);
  const [a, b, c] = [tab(), tab(), tab()];
  expect(await Promise.all([a.getAccessToken(), b.getAccessToken(), c.getAccessToken()])).toEqual(["t1", "t1", "t1"]);
  expect(rotating.refreshes()).toBe(1);
  now = 3600_000;
  expect(await Promise.all([c.getAccessToken(), a.getAccessToken(), b.getAccessToken()])).toEqual(["t2", "t2", "t2"]);
  expect(rotating.refreshes()).toBe(2);
});

test("a tab that opens takes the token of the open tabs rather than refreshing", async () => {
  const rotating = rotatingServer();
  const tab = browser(rotating);
  const leader = await tab().getAccessToken();
  expect(await tab().getAccessToken()).toBe("t1");
  expect(rotating.refreshes()).toBe(1);
  expect(rotating.accepts(leader)).toBe(true);
});

test("a 401 for a token another tab already renewed takes the new token, and the leader's stream stays up", async () => {
  let now = 0;
  const rotating = rotatingServer();
  const tab = browser(rotating, () => now);
  const [leader, follower] = [tab(), tab()];
  expect(await follower.getAccessToken()).toBe("t1");
  now = 3600_000;
  // The leader renews the token for its stream while a request of the follower that left with the old one comes back
  // refused.
  const [stream, renewed] = await Promise.all([leader.getAccessToken(), follower.refresh("t1")]);
  expect([stream, renewed]).toEqual(["t2", "t2"]);
  const api = createClient({ server, tokenSource: follower, fetch: rotating.fetch, retry: { disabled: true } });
  const { response } = await api.GET("/timelines/home");
  expect(response.status).toBe(200);
  expect(rotating.refreshes()).toBe(2);
  expect(rotating.accepts(stream)).toBe(true);
});

test("when the server refuses the token every tab holds, the tabs refresh it once", async () => {
  const rotating = rotatingServer();
  const tab = browser(rotating);
  const [a, b] = [tab(), tab()];
  await a.getAccessToken();
  rotating.revoke();
  expect(await Promise.all([a.refresh("t1"), b.refresh("t1")])).toEqual(["t2", "t2"]);
  expect(rotating.refreshes()).toBe(2);
});

test("a session that ended in one tab ends in every tab", async () => {
  const rotating = rotatingServer();
  const tab = browser(rotating);
  const [a, b] = [tab(), tab()];
  await a.getAccessToken();
  expect(await b.getAccessToken()).toBe("t1");
  rotating.end();
  expect(await b.refresh("t1")).toBeNull();
  expect(await a.getAccessToken()).toBeNull();
});

test("without Web Locks or BroadcastChannel, each tab refreshes on its own", async () => {
  const rotating = rotatingServer();
  const a = new SessionTokenSource(server, rotating.fetch, { tabs: null });
  const b = new SessionTokenSource(server, rotating.fetch, { tabs: null });
  expect(await a.getAccessToken()).toBe("t1");
  expect(await b.getAccessToken()).toBe("t2");
  expect(rotating.refreshes()).toBe(2);
});

test("a browser that refuses the lock refreshes in the tab", async () => {
  const rotating = rotatingServer();
  const refusing: TabGroup = {
    lead: () => Promise.reject(new DOMException("no storage", "SecurityError")),
    post: () => {},
    listen: () => () => {},
    close: () => {},
  };
  const source = new SessionTokenSource(server, rotating.fetch, { tabs: refusing });
  expect(await source.getAccessToken()).toBe("t1");
});

test("a message of another version of the client that it cannot read is left alone", async () => {
  const rotating = rotatingServer();
  const browser = fakeBrowser();
  const source = new SessionTokenSource(server, rotating.fetch, { tabs: browser.join() });
  const other = browser.join();
  other.post({ kind: "token", accessToken: 1, expiresAt: "later" });
  other.post({ kind: "something new" });
  expect(await source.getAccessToken()).toBe("t1");
});
