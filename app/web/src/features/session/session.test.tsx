// SPDX-License-Identifier: AGPL-3.0-only
// The whole sign-in round trip through the app (web-client/README.md T4): the sign-in page, the authorization
// redirect with PKCE, the callback, an API call whose token has to be refreshed, and the sign-out. MSW
// models the server's OAuth endpoints, with the refresh token cookie as a variable (the browser's cookie
// jar is not part of jsdom's fetch).
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test, vi } from "vitest";
import { apiBase, oauthError, oauthPaths, problem, tokenResponse } from "../../test/msw/http";
import { preferencesFixture, viewerFixture } from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { localeKey } from "../../lib/storage";
import { renderApp } from "../../test/render";

const origin = window.location.origin;

/** The S256 code challenge of a verifier, computed independently of the app (RFC 7636 §4.2). */
async function s256(verifier: string): Promise<string> {
  const digest = new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier)));
  return btoa(String.fromCharCode(...digest))
    .replaceAll("+", "-")
    .replaceAll("/", "_")
    .replace(/=+$/, "");
}

/** The server's OAuth endpoints and the verify_credentials operation, with what they received. */
function oauthServer() {
  const seen = {
    tokenForms: [] as Record<string, string>[],
    refreshes: 0,
    revoked: [] as Record<string, string>[],
    authorizations: [] as (string | null)[],
  };
  let challenge = "";
  /** The refresh token cookie; null when the browser has none. */
  let cookie: string | null = null;
  server.use(
    http.post(oauthPaths.token, async ({ request }) => {
      const form = Object.fromEntries(new URLSearchParams(await request.text()));
      seen.tokenForms.push(form);
      if (form.code !== "the-code" || (await s256(form.code_verifier ?? "")) !== challenge)
        return oauthError("invalid_grant");
      cookie = "refresh-1";
      return tokenResponse("access-1");
    }),
    http.post(oauthPaths.refresh, () => {
      if (cookie === null) return oauthError("invalid_grant");
      seen.refreshes++;
      cookie = `refresh-${seen.refreshes + 1}`;
      return tokenResponse(`access-${seen.refreshes + 1}`);
    }),
    http.post(oauthPaths.revoke, async ({ request }) => {
      seen.revoked.push(Object.fromEntries(new URLSearchParams(await request.text())));
      cookie = null;
      return new HttpResponse(null, { status: 200 });
    }),
    // access-1 has expired by the time the settings page asks; only the refreshed token works.
    http.get(`${apiBase}/accounts/verify_credentials`, ({ request }) => {
      const authorization = request.headers.get("Authorization");
      seen.authorizations.push(authorization);
      return authorization === "Bearer access-2"
        ? HttpResponse.json(viewerFixture)
        : problem(401, "/api/v1/problems/invalid-token", "Invalid token");
    }),
    // The shell's other requests take the refreshed token too, as every operation of the server does.
    http.get(`${apiBase}/preferences`, ({ request }) =>
      request.headers.get("Authorization") === "Bearer access-2"
        ? HttpResponse.json(preferencesFixture)
        : problem(401, "/api/v1/problems/invalid-token", "Invalid token"),
    ),
  );
  return {
    seen,
    /** The authorization screen: it remembers the challenge of the request and redirects back with a code. */
    authorize(url: URL) {
      challenge = url.searchParams.get("code_challenge") ?? "";
      return `/login/callback?${new URLSearchParams({ code: "the-code", state: url.searchParams.get("state") ?? "" }).toString()}`;
    },
  };
}

/** A Cache Storage with a runtime cache of media and the Service Worker's precache of the build. */
function stubCacheStorage() {
  const names = new Set(["my-media", "my-precache-v2-https://my.example/"]);
  const storage = {
    keys: vi.fn(() => Promise.resolve([...names])),
    delete: vi.fn((name: string) => Promise.resolve(names.delete(name))),
  };
  vi.stubGlobal("caches", storage);
  return names;
}

/** jsdom has no IndexedDB; a stand-in records whether the app opens a database. */
function stubIndexedDB() {
  const idb = { open: vi.fn(), deleteDatabase: vi.fn(), databases: vi.fn(() => Promise.resolve([])) };
  vi.stubGlobal("indexedDB", idb);
  return idb;
}

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

describe("signing in and out", () => {
  test("sign in with PKCE → an API call → a token refresh → sign out", async () => {
    const oauth = oauthServer();
    const cacheNames = stubCacheStorage();
    const idb = stubIndexedDB();

    // 1. The sign-in page sends the browser to the authorization screen.
    const view = await renderApp({ path: "/login?return_to=/settings" });
    expect(await screen.findByRole("heading", { level: 1, name: "Sign in" })).toBeInTheDocument();
    await view.user.click(screen.getByRole("button", { name: "Continue to sign in" }));
    await waitFor(() => {
      expect(view.leftTo).toHaveLength(1);
    });
    const authorizeURL = view.leftTo[0] ?? new URL(origin);
    expect(authorizeURL.origin + authorizeURL.pathname).toBe(`${origin}/oauth/authorize`);
    expect(Object.fromEntries(authorizeURL.searchParams)).toEqual({
      response_type: "code",
      client_id: "my-web",
      redirect_uri: `${origin}/login/callback`,
      state: expect.stringMatching(/^[\w-]{43}$/) as unknown,
      code_challenge: expect.stringMatching(/^[\w-]{43}$/) as unknown,
      code_challenge_method: "S256",
    });
    view.unmount();

    // 2. The server redirects back: a new page load, with only sessionStorage left from the first one.
    const { user, router, session, queryClient } = await renderApp({ path: oauth.authorize(authorizeURL) });

    // 3. The callback exchanges the code and opens the page the sign-in started from. Its first API call
    //    gets a 401 for the expired token, refreshes it and succeeds with the new one.
    expect(await screen.findByRole("heading", { level: 1, name: "Settings" })).toBeInTheDocument();
    expect(router.state.location.href).toBe("/settings");
    const main = screen.getByRole("main");
    expect(await within(main).findByText("Alice")).toBeInTheDocument();
    expect(within(main).getByText("@alice")).toBeInTheDocument();
    expect(oauth.seen.tokenForms).toEqual([
      expect.objectContaining({
        grant_type: "authorization_code",
        code: "the-code",
        client_id: "my-web",
        redirect_uri: `${origin}/login/callback`,
      }),
    ]);
    expect(oauth.seen.authorizations).toEqual(["Bearer access-1", "Bearer access-2"]);
    expect(oauth.seen.refreshes).toBe(1);

    // 4. Signing out revokes the token and forgets the user's data.
    const account = screen.getByRole("region", { name: "Account" });
    await user.click(within(account).getByRole("button", { name: "Sign out" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Sign in" })).toBeInTheDocument();
    expect(oauth.seen.revoked).toEqual([{ token: "access-2", token_type_hint: "access_token", client_id: "my-web" }]);
    expect(session.status).toBe("signed-out");
    // No query keeps the account (an observer that was still mounted may leave an empty, disabled one).
    // The navigation reads the public GET /instance again at once, as anyone may; it holds nothing of the user.
    const userQueries = queryClient
      .getQueryCache()
      .getAll()
      .filter((query) => query.queryKey[1] !== "/instance");
    expect(userQueries.map((query) => query.state.data)).not.toContainEqual(expect.anything());
    await waitFor(() => {
      expect([...cacheNames]).toEqual([]);
    });

    // No token was ever written where a script could read it later.
    // localStorage holds only the instance's public timelines, which the navigation keeps for an offline start, and
    // the locale the app was last displayed in, which the next start begins in.
    expect(Object.keys(localStorage).toSorted()).toEqual(["my:display:locale", "my:instance:timelines"]);
    expect(localeKey.read()).toBe("en");
    expect(sessionStorage).toHaveLength(0);
    expect(idb.open).not.toHaveBeenCalled();
    expect(document.cookie).toBe("");
  });

  test("the settings page sends to sign in when no session resumes", async () => {
    const { router } = await renderApp({ path: "/settings" });

    expect(await screen.findByRole("heading", { level: 1, name: "Sign in" })).toBeInTheDocument();
    expect(router.state.location.search).toEqual({ return_to: "/settings" });
  });
});

describe("a callback that is not for this tab's sign-in", () => {
  test("is refused without exchanging the code", async () => {
    const oauth = oauthServer();

    await renderApp({ path: "/login/callback?code=the-code&state=forged" });

    expect(await screen.findByRole("heading", { level: 1, name: "Could not sign in" })).toBeInTheDocument();
    expect(
      screen.getByText("This sign-in was not started in this tab, or it has already been completed. Start again."),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Sign in again" })).toHaveAttribute("href", "/login");
    expect(oauth.seen.tokenForms).toEqual([]);
  });

  test("reports a sign-in declined on the authorization screen", async () => {
    const oauth = oauthServer();
    const view = await renderApp({ path: "/login" });
    await view.user.click(await screen.findByRole("button", { name: "Continue to sign in" }));
    await waitFor(() => {
      expect(view.leftTo).toHaveLength(1);
    });
    const state = view.leftTo[0]?.searchParams.get("state") ?? "";
    view.unmount();

    await renderApp({ path: `/login/callback?error=access_denied&state=${state}`, locale: "ja" });

    expect(await screen.findByRole("heading", { level: 1, name: "ログインできませんでした" })).toBeInTheDocument();
    expect(screen.getByText("サーバーの画面でログインが取り消されました。")).toBeInTheDocument();
    expect(oauth.seen.tokenForms).toEqual([]);
  });
});
