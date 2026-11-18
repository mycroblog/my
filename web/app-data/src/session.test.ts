// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { createFakeApi, fakeOrigin } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import type { TabGroup } from "@my/platform";
import { preferencesQuery } from "./preferences";
import { closeSession, endSession, onSignedOutElsewhere, openSession, SessionTokenSource } from "./session";
import { fakeBrowser } from "./test/tabs";

const server = fakeOrigin;

// The trade of the session's cookie for an access token (POST /oauth/token/refresh, ADR-0090 §16) is no operation of
// the client API, which the fake API answers: these tests answer it themselves, as RFC 6749 words a token and its
// refusal, and hand every other request to a fake API.
const json = (status: number, body: unknown) => Response.json(body, { status });

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
  expect(requests[0]?.url).toBe(`${server}/oauth/token/refresh`);
  expect(requests[0]?.headers.get("X-My-Token-Refresh")).toBe("1");
});

test("with a session the client API gets the access token the cookie traded for", async () => {
  const api = createFakeApi();
  const authorizations: (string | null)[] = [];
  const data = await openSession({
    server,
    fetch: (request) => {
      if (new URL(request.url).pathname === "/oauth/token/refresh")
        return Promise.resolve(
          json(200, { access_token: "a1", token_type: "Bearer", expires_in: 3600, scope: "read" }),
        );
      authorizations.push(request.headers.get("Authorization"));
      return api.serve(request);
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
  const api = createFakeApi();
  const refusing = createFakeApi({ visitor: true });
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
    return (token === valid ? api : refusing).serve(request);
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

/**
 * A browser of two tabs signed in on a fake API, whose sign-in server trades the session's cookie for the token `a1`
 * until the session ends: the first tab's data, which read the reader's preferences, the other tab's token source, and
 * what the server was asked. The revocation (POST /oauth/revoke) is the fake's to answer.
 */
async function signedIn(options: Parameters<typeof createFakeApi>[0] = {}) {
  const api = createFakeApi(options);
  const browser = fakeBrowser();
  const asked: { path: string; method: string; type: string | null; form: Record<string, string> }[] = [];
  let answers = true;
  let ended = false;
  const seen: string[] = [];
  const fetch = async (request: Request): Promise<Response> => {
    const path = new URL(request.url).pathname;
    seen.push(`${request.method} ${path}`);
    if (path === "/oauth/token/refresh")
      return ended || api.revocations.length > 0
        ? json(400, { error: "invalid_grant" })
        : json(200, { access_token: "a1", token_type: "Bearer", expires_in: 3600 });
    if (path !== "/oauth/revoke") return api.serve(request);
    asked.push({
      path,
      method: request.method,
      // The media type, without the charset a browser may name after it.
      type: request.headers.get("Content-Type")?.split(";")[0] ?? null,
      form: Object.fromEntries(new URLSearchParams(await request.clone().text())),
    });
    if (!answers) throw new TypeError("Failed to fetch");
    return api.serve(request);
  };
  // A tab that only listens: what the tabs tell each other of the session, by the kind of each message.
  const heard: string[] = [];
  browser.join().listen((message) => {
    const kind = typeof message === "object" && message !== null && "kind" in message ? message.kind : undefined;
    if (kind === "ended" || kind === "signed_out") heard.push(kind);
  });
  const data = await openSession({ server, fetch, tabs: browser.join() });
  if (data === null) throw new Error("test: the browser has no session");
  const other = new SessionTokenSource(server, fetch, { tabs: browser.join() });
  expect(await other.getAccessToken()).toBe("a1");
  await data.queryClient.query(preferencesQuery(data));
  return {
    api,
    data,
    other,
    asked,
    heard,
    /** Everything the server was asked, the trades of the cookie too, each as `METHOD /path`. */
    seen,
    /** One more tab of the browser, with the app's data in it. */
    tab: async () => {
      const opened = await openSession({ server, fetch, tabs: browser.join() });
      if (opened === null) throw new Error("test: the browser has no session");
      return opened;
    },
    /** The server stops answering the revocation. */
    cut: () => {
      answers = false;
    },
    /** The server ends the session on its own: the cookie trades for no token from now on. */
    end: () => {
      ended = true;
    },
    /** How many answers the first tab's cache holds. */
    cached: () => data.queryClient.getQueryCache().getAll().length,
  };
}

/** What `settles` rejected with, or null when it resolved. */
const refusal = (settles: Promise<unknown>): Promise<unknown> =>
  settles.then(
    () => null,
    (error: unknown) => error,
  );

test("ending the session revokes its access token, as a form the token endpoint reads, and the server takes it", async () => {
  const { api, data, asked } = await signedIn();

  expect(await endSession(data, "my-web")).toBe("revoked");

  expect(asked).toEqual([
    {
      path: "/oauth/revoke",
      method: "POST",
      type: "application/x-www-form-urlencoded",
      form: { client_id: "my-web", token: "a1", token_type_hint: "access_token" },
    },
  ]);
  expect(api.revocations).toEqual([{ clientId: "my-web", token: "a1" }]);
});

test("once the server took the revocation, the other tabs have no session and the cache holds nothing", async () => {
  const { data, other, cached } = await signedIn();
  expect(cached()).toBeGreaterThan(0);

  await endSession(data, "my-web");

  expect(cached()).toBe(0);
  expect(await other.getAccessToken()).toBeNull();
  expect(await data.connection?.tokenSource?.getAccessToken()).toBeNull();
});

for (const [what, options, cuts] of [
  ["refuses the revocation", { revocation: "fails" }, false],
  ["does not answer", {}, true],
] as const)
  test(`a server that ${what} ends nothing: it rejects, nothing is cleaned, and the tabs and the cache are as they were`, async () => {
    const { api, data, other, asked, heard, tab, cut, cached } = await signedIn(options);
    if (cuts) cut();
    const before = cached();
    const did: string[] = [];
    onSignedOutElsewhere(await tab(), () => did.push("another tab heard a sign-out"));

    const failed = await refusal(
      endSession(data, "my-web", () => {
        did.push("cleaned");
        return Promise.resolve();
      }),
    );

    expect(failed).toBeInstanceOf(Error);
    expect(asked).toHaveLength(1);
    expect(api.revocations).toEqual([]);
    expect(did).toEqual([]);
    expect(heard).toEqual([]);
    expect(cached()).toBe(before);
    expect(await other.getAccessToken()).toBe("a1");
    expect(await data.connection?.tokenSource?.getAccessToken()).toBe("a1");
  });

test("a session the server ended already has no token to revoke: nothing is asked, it ends unrevoked, and the tab is cleaned and the cache emptied all the same", async () => {
  const { data, asked, end, cached } = await signedIn();
  end();
  // The server refuses the token the tab holds, and the cookie trades for no other.
  expect(await data.connection?.tokenSource?.refresh?.("a1")).toBeNull();
  let cleaned = 0;

  const ended = await endSession(data, "my-web", () => {
    cleaned += 1;
    return Promise.resolve();
  });

  expect(ended).toBe("unrevoked");
  expect(asked).toEqual([]);
  expect(cleaned).toBe(1);
  expect(cached()).toBe(0);
});

test("what the tab cleans is cleaned once the server took the revocation and before any other tab is told; the others then hear that the session ended, then that the reader signed out", async () => {
  const { data, asked, heard, tab } = await signedIn();
  const order: string[] = [];
  onSignedOutElsewhere(await tab(), () => order.push("another tab hears the sign-out"));

  await endSession(data, "my-web", async () => {
    order.push(`cleaning after ${String(asked.length)} revocation, the others told ${JSON.stringify(heard)}`);
    // What a tab removes takes its turns: nobody is told meanwhile.
    await new Promise((turn) => setTimeout(turn, 0));
    order.push(`cleaned, the others told ${JSON.stringify(heard)}`);
  });

  expect(order).toEqual([
    "cleaning after 1 revocation, the others told []",
    "cleaned, the others told []",
    "another tab hears the sign-out",
  ]);
  expect(heard).toEqual(["ended", "signed_out"]);
});

// A session the reader ended by another way than signing out here: they revoked the server's own web client, whose
// tokens are the session of every browser (ADR-0090 §5). The server has ended it; the tab only has to follow.
test("closing a session the server ended asks the server nothing: the tab forgets its token and empties the cache, then cleans what it is given to clean, and only then are the other tabs told, as after a sign-out", async () => {
  const { data, other, asked, heard, seen, tab, end, cached } = await signedIn();
  expect(cached()).toBeGreaterThan(0);
  const order: string[] = [];
  onSignedOutElsewhere(await tab(), () => order.push("another tab hears the sign-out"));
  // The server took the revocation of the web client: the cookie trades for no token from now on.
  end();
  const before = seen.length;

  await closeSession(data, async () => {
    order.push(`cleaning with ${String(cached())} answers cached, the others told ${JSON.stringify(heard)}`);
    // What a tab removes takes its turns: nobody is told meanwhile.
    await new Promise((turn) => setTimeout(turn, 0));
    order.push(`cleaned, the others told ${JSON.stringify(heard)}`);
  });

  expect(order).toEqual([
    "cleaning with 0 answers cached, the others told []",
    "cleaned, the others told []",
    "another tab hears the sign-out",
  ]);
  expect(heard).toEqual(["ended", "signed_out"]);
  expect(seen.slice(before)).toEqual([]);
  expect(asked).toEqual([]);
  expect(cached()).toBe(0);
  // Neither this tab nor another has a token to go on with.
  expect(await data.connection?.tokenSource?.getAccessToken()).toBeNull();
  expect(await other.getAccessToken()).toBeNull();
});

test("closing a session with nothing to clean empties the cache and tells the other tabs all the same", async () => {
  const { data, heard, seen, cached } = await signedIn();
  const before = seen.length;

  await closeSession(data);

  expect(cached()).toBe(0);
  expect(heard).toEqual(["ended", "signed_out"]);
  expect(seen.slice(before)).toEqual([]);
});

test("a tab hears of a sign-out in another tab: not of its own, not once it stopped listening, and not of a session that merely ended", async () => {
  const signedOut = await signedIn();
  const fired: string[] = [];
  onSignedOutElsewhere(signedOut.data, () => fired.push("the tab that signs out"));
  onSignedOutElsewhere(await signedOut.tab(), () => fired.push("another tab"));
  const stop = onSignedOutElsewhere(await signedOut.tab(), () => fired.push("a tab that stopped listening"));
  stop();

  await endSession(signedOut.data, "my-web");

  expect(fired).toEqual(["another tab"]);

  // A session the server ended: the tab that finds out tells the others that it ended, and nobody signed out.
  const ended = await signedIn();
  const tab = await ended.tab();
  onSignedOutElsewhere(tab, () => fired.push("a tab of a session that ended"));
  ended.end();
  expect(await ended.data.connection?.tokenSource?.refresh?.("a1")).toBeNull();

  expect(ended.heard).toEqual(["ended"]);
  expect(await tab.connection?.tokenSource?.getAccessToken()).toBeNull();
  expect(fired).toEqual(["another tab"]);
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
