// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { openSession, SessionTokenSource } from "./session";
import { json, page } from "./test/fake";

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
    () => now,
  );
  expect(await Promise.all([source.getAccessToken(), source.getAccessToken()])).toEqual(["a1", "a1"]);
  expect(await source.getAccessToken()).toBe("a1");
  now = 60_000;
  expect(await source.getAccessToken()).toBe("a2");
  expect(await source.refresh("a2")).toBe("a3");
  expect(await source.refresh("a2")).toBe("a3");
  expect(trades).toBe(3);
});
