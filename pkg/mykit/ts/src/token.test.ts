// SPDX-License-Identifier: MIT
import { describe, expect, test } from "bun:test";
import { createClient } from "./client.js";
import { codeChallenge, exchange, newCodeVerifier, OAuthError, type OAuthConfig } from "./oauth.js";
import { fakeFetch, json, problem, server } from "./test/fake.js";
import { RefreshingTokenSource, type Token } from "./token.js";

/**
 * A token endpoint that rotates the tokens on every refresh_token grant: the n-th grant answers a<n> and r<n>. A used
 * refresh token is refused, as the server revokes the family when it sees one again.
 */
function tokenEndpoint() {
  let n = 0;
  const used = new Set<string>();
  const grants: URLSearchParams[] = [];
  const f = fakeFetch(async (r) => {
    const form = new URLSearchParams(await r.text());
    grants.push(form);
    const refresh = form.get("refresh_token") ?? "";
    if (used.has(refresh)) return json(400, { error: "invalid_grant" });
    used.add(refresh);
    n++;
    return json(200, { access_token: `a${String(n)}`, token_type: "Bearer", expires_in: 3600, refresh_token: `r${String(n)}`, scope: "read" });
  });
  return { fetch: f.fetch, grants };
}

function oauth(fetch: OAuthConfig["fetch"]): OAuthConfig {
  return { server, clientId: "https://app.example/client.json", redirectUri: "https://app.example/cb", ...(fetch === undefined ? {} : { fetch }) };
}

describe("RefreshingTokenSource", () => {
  test("concurrent 401s refresh once, and the rotated refresh token reaches onTokens", async () => {
    const endpoint = tokenEndpoint();
    const saved: Token[] = [];
    const source = new RefreshingTokenSource(oauth(endpoint.fetch), { accessToken: "a0", refreshToken: "r0" }, (t) => {
      saved.push(t);
    });
    // The API refuses a0 and holds every request until all eight have met the 401, so that the refreshes overlap.
    const requests = 8;
    let refused = 0;
    let release: () => void = () => undefined;
    const allRefused = new Promise<void>((resolve) => {
      release = resolve;
    });
    const api = createClient({
      server,
      tokenSource: source,
      fetch: async (r) => {
        if (r.headers.get("Authorization") === "Bearer a0") {
          if (++refused === requests) release();
          await allRefused;
          return problem(401, "/api/v1/problems/invalid-token");
        }
        return json(200, { items: [] });
      },
    });
    const results = await Promise.all(Array.from({ length: requests }, () => api.GET("/timelines/home")));
    expect(results.every((r) => r.response.status === 200)).toBe(true);
    expect(endpoint.grants).toHaveLength(1);
    expect(endpoint.grants[0]?.get("grant_type")).toBe("refresh_token");
    expect(endpoint.grants[0]?.get("refresh_token")).toBe("r0");
    expect(endpoint.grants[0]?.get("client_id")).toBe("https://app.example/client.json");
    expect(saved.map((t) => t.refreshToken)).toEqual(["r1"]);
    expect(source.token?.accessToken).toBe("a1");
  });

  test("a refresh after another request renewed the token returns that token", async () => {
    const endpoint = tokenEndpoint();
    const source = new RefreshingTokenSource(oauth(endpoint.fetch), { accessToken: "a0", refreshToken: "r0" });
    expect(await source.refresh("a0")).toBe("a1");
    expect(await source.refresh("a0")).toBe("a1");
    expect(endpoint.grants).toHaveLength(1);
    expect(await source.refresh("a1")).toBe("a2");
    expect(endpoint.grants[1]?.get("refresh_token")).toBe("r1");
  });

  test("an expired token is renewed before the request", async () => {
    const endpoint = tokenEndpoint();
    let now = 1_000_000;
    const source = new RefreshingTokenSource(oauth(endpoint.fetch), { accessToken: "a0", refreshToken: "r0", expiresAt: now + 60_000 }, undefined, () => now);
    expect(await source.getAccessToken()).toBe("a0");
    now += 45_000; // within the 30-second skew of the expiry
    expect(await source.getAccessToken()).toBe("a1");
    expect(endpoint.grants).toHaveLength(1);
  });

  test("keeps the refresh token when the answer has none, and fails without one", async () => {
    const f = fakeFetch(() => json(200, { access_token: "a1", token_type: "bearer" }));
    const source = new RefreshingTokenSource(oauth(f.fetch), { accessToken: "a0", refreshToken: "r0" });
    expect(await source.refresh("a0")).toBe("a1");
    expect(source.token?.refreshToken).toBe("r0");

    const none = new RefreshingTokenSource(oauth(f.fetch), { accessToken: "a0" });
    expect(String(await none.refresh("a0").catch((e: unknown) => e))).toContain("no refresh token");
  });

  test("a failing onTokens fails the request but keeps the new tokens", async () => {
    const endpoint = tokenEndpoint();
    const source = new RefreshingTokenSource(oauth(endpoint.fetch), { accessToken: "a0", refreshToken: "r0" }, () => {
      throw new Error("disk full");
    });
    expect(String(await source.refresh("a0").catch((e: unknown) => e))).toContain("disk full");
    expect(source.token?.refreshToken).toBe("r1");
  });

  test("an error answer of the token endpoint is an OAuthError", async () => {
    const f = fakeFetch(() => json(400, { error: "invalid_grant", error_description: "revoked" }));
    const source = new RefreshingTokenSource(oauth(f.fetch), { accessToken: "a0", refreshToken: "r0" });
    const error = (await source.refresh("a0").catch((e: unknown) => e)) as OAuthError;
    expect(error).toBeInstanceOf(OAuthError);
    expect(error.status).toBe(400);
    expect(error.code).toBe("invalid_grant");
    expect(error.description).toBe("revoked");
  });
});

describe("PKCE and the code exchange", () => {
  test("the S256 challenge of RFC 7636 Appendix B", async () => {
    expect(await codeChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")).toBe("E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM");
  });

  test("a new verifier is 43 base64url characters", () => {
    const v = newCodeVerifier();
    expect(v).toMatch(/^[A-Za-z0-9_-]{43}$/);
    expect(newCodeVerifier()).not.toBe(v);
  });

  test("a public client sends its client_id in the form", async () => {
    const f = fakeFetch(() => json(200, { access_token: "a1", token_type: "Bearer", refresh_token: "r1", expires_in: 60 }));
    const before = Date.now();
    const token = await exchange(oauth(f.fetch), "code1", "verifier1");
    expect(token.accessToken).toBe("a1");
    expect(token.refreshToken).toBe("r1");
    expect(token.expiresAt).toBeGreaterThanOrEqual(before + 60_000);
    const [sent] = f.requests;
    expect(sent?.request.url).toBe(`${server}/oauth/token`);
    expect(sent?.request.headers.has("Authorization")).toBe(false);
    const form = new URLSearchParams(sent?.body);
    expect(Object.fromEntries(form)).toEqual({
      grant_type: "authorization_code",
      code: "code1",
      code_verifier: "verifier1",
      redirect_uri: "https://app.example/cb",
      client_id: "https://app.example/client.json",
    });
  });

  test("a confidential client authenticates with HTTP Basic", async () => {
    const f = fakeFetch(() => json(200, { access_token: "a1", token_type: "Bearer" }));
    await exchange({ server, clientId: "app 1", clientSecret: "s:é", fetch: f.fetch }, "code1", "verifier1");
    const [sent] = f.requests;
    const basic = sent?.request.headers.get("Authorization") ?? "";
    expect(basic.startsWith("Basic ")).toBe(true);
    expect(Buffer.from(basic.slice("Basic ".length), "base64").toString("utf8")).toBe("app+1:s%3A%C3%A9");
    expect(new URLSearchParams(sent?.body).has("client_id")).toBe(false);
  });

  test("an answer without a bearer token is invalid", async () => {
    const f = fakeFetch(() => json(200, { access_token: "a1", token_type: "mac" }));
    const error = (await exchange(oauth(f.fetch), "c", "v").catch((e: unknown) => e)) as OAuthError;
    expect(error.code).toBe("invalid_response");
  });
});
