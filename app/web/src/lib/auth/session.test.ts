// SPDX-License-Identifier: AGPL-3.0-only
// The session across tabs (ADR-0030): two Session objects with one lock manager and the real
// BroadcastChannel act as two tabs of the origin. The refresh endpoint is modelled like the server's:
// it rotates the refresh token, and a refresh token used by two requests at once revokes the family.
import { delay, http, HttpResponse } from "msw";
import { waitFor } from "@testing-library/react";
import { afterEach, describe, expect, test } from "vitest";
import { oauthError, oauthPaths, tokenResponse } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { createLockManager } from "../../test/locks";
import { Session } from "./session";

const apiOrigin = window.location.origin;

/** The refresh endpoint with strict rotation and reuse detection, and what it saw. */
function refreshEndpoint() {
  const seen = { requests: 0, maxConcurrent: 0, revoked: false };
  let inFlight = 0;
  server.use(
    http.post(oauthPaths.refresh, async ({ request }) => {
      if (request.headers.get("X-My-Token-Refresh") !== "1") return oauthError("invalid_request", 403);
      seen.requests++;
      inFlight++;
      seen.maxConcurrent = Math.max(seen.maxConcurrent, inFlight);
      // Two requests with the same cookie at once: the second is a reuse, and the family is revoked.
      if (inFlight > 1) seen.revoked = true;
      await delay(20);
      inFlight--;
      return seen.revoked ? oauthError("invalid_grant") : tokenResponse(`access-${seen.requests}`);
    }),
  );
  return seen;
}

const sessions: Session[] = [];

/** A tab of the origin: a session on the shared lock manager and the real BroadcastChannel. */
function openTab(locks = sharedLocks) {
  const session = new Session({ apiOrigin, locks });
  sessions.push(session);
  return session;
}

let sharedLocks = createLockManager();

afterEach(() => {
  for (const session of sessions.splice(0)) session.dispose();
  sharedLocks = createLockManager();
});

describe("refreshing in two tabs", () => {
  test("never sends the refresh token cookie twice at once, so the server does not revoke the family", async () => {
    const seen = refreshEndpoint();
    const [a, b] = [openTab(), openTab()];

    const [tokenA, tokenB] = await Promise.all([a.refresh(null), b.refresh(null)]);

    expect(seen.maxConcurrent).toBe(1);
    expect(seen.revoked).toBe(false);
    expect(tokenA).not.toBeNull();
    expect(tokenB).not.toBeNull();
    // Whichever refreshed last, its token reaches the other tab.
    await waitFor(() => {
      expect(a.accessToken).toBe(b.accessToken);
    });
    expect([a.status, b.status]).toEqual(["signed-in", "signed-in"]);
  });

  test("a tab uses the token that another tab's refresh shared instead of refreshing again", async () => {
    const seen = refreshEndpoint();
    const [a, b] = [openTab(), openTab()];
    await a.refresh(null);
    await waitFor(() => {
      expect(b.accessToken).toBe("access-1");
    });

    // b's old token was rejected; the one a shared is newer, so b needs no request of its own.
    expect(await b.refresh("expired-token")).toBe("access-1");
    expect(seen.requests).toBe(1);
  });

  test("concurrent refreshes in one tab share one request", async () => {
    const seen = refreshEndpoint();
    const a = openTab();

    const tokens = await Promise.all([a.refresh("old"), a.refresh("old"), a.refresh("old")]);

    expect(new Set(tokens)).toEqual(new Set(["access-1"]));
    expect(seen.requests).toBe(1);
  });

  test("without a shared lock the same refresh would revoke the family (what the lock prevents)", async () => {
    const seen = refreshEndpoint();
    const [a, b] = [openTab(createLockManager()), openTab(createLockManager())];

    await Promise.all([a.refresh(null), b.refresh(null)]);

    expect(seen.revoked).toBe(true);
  });
});

describe("the end of a session", () => {
  test("a refused refresh signs out every tab that had the session", async () => {
    refreshEndpoint();
    const [a, b] = [openTab(), openTab()];
    await a.refresh(null);
    await waitFor(() => {
      expect(b.status).toBe("signed-in");
    });

    server.use(http.post(oauthPaths.refresh, () => oauthError("invalid_grant")));
    expect(await a.refresh("access-1")).toBeNull();

    expect(a.status).toBe("signed-out");
    await waitFor(() => {
      expect(b.status).toBe("signed-out");
    });
    expect(b.accessToken).toBeNull();
  });

  test("signing out revokes the access token and signs out the other tabs", async () => {
    refreshEndpoint();
    const revoked: URLSearchParams[] = [];
    server.use(
      http.post(oauthPaths.revoke, async ({ request }) => {
        revoked.push(new URLSearchParams(await request.text()));
        return new HttpResponse(null, { status: 200 });
      }),
    );
    const [a, b] = [openTab(), openTab()];
    await a.refresh(null);
    await waitFor(() => {
      expect(b.status).toBe("signed-in");
    });
    const changes: string[] = [];
    b.onUserChange((status) => changes.push(status));

    await a.signOut();

    expect(revoked.map((form) => Object.fromEntries(form))).toEqual([
      { token: "access-1", token_type_hint: "access_token", client_id: "my-web" },
    ]);
    expect(a.accessToken).toBeNull();
    await waitFor(() => {
      expect(b.status).toBe("signed-out");
    });
    expect(b.accessToken).toBeNull();
    expect(changes).toEqual(["signed-out"]);
  });

  test("signing out lets its listeners send a request with the token before revoking it", async () => {
    refreshEndpoint();
    const order: string[] = [];
    server.use(
      http.post(oauthPaths.revoke, () => {
        order.push("revoke");
        return new HttpResponse(null, { status: 200 });
      }),
    );
    const a = openTab();
    await a.refresh(null);
    a.onBeforeSignOut(async () => {
      await delay(10);
      order.push(`leave with ${a.accessToken ?? "nothing"}`);
    });

    await a.signOut();

    expect(order).toEqual(["leave with access-1", "revoke"]);
  });

  test("a sign-out that cannot reach the server still signs out here and says so", async () => {
    refreshEndpoint();
    server.use(http.post(oauthPaths.revoke, () => HttpResponse.error()));
    const a = openTab();
    await a.refresh(null);

    await expect(a.signOut()).rejects.toThrow("the server did not revoke the session");

    expect(a.status).toBe("signed-out");
    expect(a.accessToken).toBeNull();
  });
});

describe("resuming while the server cannot be reached", () => {
  test("acts signed out, and resumes on the next restore once the server answers", async () => {
    server.use(http.post(oauthPaths.refresh, () => HttpResponse.error()));
    const a = openTab();
    const changes: string[] = [];
    a.onUserChange((status) => changes.push(status));

    await a.restore();
    expect(a.status).toBe("signed-out");
    expect(a.unreachable).toBe(true);
    // Still unreachable: nothing changes.
    await a.restore();
    expect(a.unreachable).toBe(true);
    expect(changes).toEqual([]);

    refreshEndpoint();
    await a.restore();
    expect(a.status).toBe("signed-in");
    expect(a.unreachable).toBe(false);
    // The tab may have shown data of no session meanwhile: the user changed.
    expect(changes).toEqual(["signed-in"]);
  });

  test("a session the server no longer has ends the user's data that the tab kept meanwhile", async () => {
    server.use(http.post(oauthPaths.refresh, () => HttpResponse.error()));
    const a = openTab();
    const changes: string[] = [];
    a.onUserChange((status) => changes.push(status));
    await a.restore();
    let notified = 0;
    a.subscribe(() => notified++);

    server.use(http.post(oauthPaths.refresh, () => oauthError("invalid_grant")));
    await a.restore();

    expect(a.status).toBe("signed-out");
    expect(a.unreachable).toBe(false);
    expect(notified).toBe(1);
    expect(changes).toEqual(["signed-out"]);
    // Resumed and answered: no more tries.
    await a.restore();
    expect(changes).toEqual(["signed-out"]);
  });

  test("a start that the server answers without a session is not unreachable", async () => {
    const a = openTab();
    await a.restore();
    expect(a.status).toBe("signed-out");
    expect(a.unreachable).toBe(false);
  });
});
