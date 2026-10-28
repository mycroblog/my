// SPDX-License-Identifier: AGPL-3.0-only
// The account search and the follow of an account on another server through the whole app (web-client/README.md
// T4): the real router and QueryClient, with MSW as the server (FR-305, FR-301). A signed-in viewer's search
// asks the server to resolve the account on its own server; the found profile opens, and a follow of a
// remote account stays requested until that server accepts it.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import {
  instanceFixture,
  relationshipFixture,
  remoteAccountFixture,
  type Schemas,
  statusPage,
  viewerFixture,
} from "../../test/fixtures";
import { apiBase, problem, resumableSession } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

const carol = { ...remoteAccountFixture, followers_count: 3, following_count: 4 };

/**
 * A server that resolves Carol from her handle or her profile's URL, and answers other searches with
 * `otherwise`. Returns the searches it received, as `acct` and whether it was asked to resolve.
 */
function searchServer(otherwise: () => Response = () => problem(404, "about:blank", "Not Found")) {
  const searches: { acct: string; resolve: boolean }[] = [];
  server.use(
    http.get(`${apiBase}/accounts/lookup`, ({ request }) => {
      const params = new URL(request.url).searchParams;
      const acct = params.get("acct") ?? "";
      // The profile opens from the cache: only the search reaches the server.
      searches.push({ acct, resolve: params.get("resolve") === "true" });
      return [carol.acct, `@${carol.acct}`, carol.url].includes(acct) ? HttpResponse.json(carol) : otherwise();
    }),
  );
  return { searches };
}

/** Opens the search page and returns the field and the app. */
async function openSearch(path = "/search") {
  const { user, router } = await renderApp({ path });
  const field = await screen.findByRole("searchbox", { name: "Handle or URL" });
  return { user, router, field };
}

describe("the search page", () => {
  test("is a search landmark named by the page heading, linked from the navigation", async () => {
    server.use(resumableSession());
    await openSearch();

    expect(screen.getByRole("heading", { level: 1, name: "Search" })).toBeInTheDocument();
    expect(screen.getByRole("search", { name: "Search" })).toContainElement(screen.getByRole("searchbox"));
    const nav = screen.getByRole("navigation", { name: "Main" });
    expect(within(nav).getByRole("link", { name: "Search" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("searchbox")).toHaveAccessibleDescription(/on this server or another one/);
  });

  test("resolves a handle on its server and opens its profile, with the keyboard only", async () => {
    server.use(resumableSession());
    const { searches } = searchServer();
    const { user, field, router } = await openSearch();

    await user.click(field);
    await user.keyboard("@carol@remote.example{Enter}");

    const heading = await screen.findByRole("heading", { level: 1, name: "Carol" });
    expect(router.state.location.pathname).toBe("/@carol@remote.example");
    // The layout moves the focus to the new page's heading (web-client/README.md Y5).
    await waitFor(() => {
      expect(heading).toHaveFocus();
    });
    expect(searches).toEqual([{ acct: "@carol@remote.example", resolve: true }]);
    expect(within(screen.getByRole("main")).getAllByText("@carol@remote.example")[0]).toBeInTheDocument();
  });

  test("resolves the URL of a profile, trimmed", async () => {
    server.use(resumableSession());
    const { searches } = searchServer();
    const { user, field } = await openSearch();

    await user.type(field, `  ${carol.url} `);
    await user.click(screen.getByRole("button", { name: "Search" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Carol" })).toBeInTheDocument();
    expect(searches).toEqual([{ acct: carol.url, resolve: true }]);
  });

  test("says when no account is found, until the text changes", async () => {
    server.use(resumableSession());
    searchServer();
    const { user, field } = await openSearch();

    await user.type(field, "@nobody@remote.example{Enter}");

    const message = "No account found. Check the handle or the address.";
    expect(await screen.findByRole("alert")).toHaveTextContent(message);
    expect(field).toHaveAttribute("aria-invalid", "true");
    expect(field).toHaveAccessibleDescription(new RegExp(message.replaceAll(".", "\\.")));
    expect(field).toHaveFocus();

    await user.type(field, "x");
    expect(field).not.toHaveAttribute("aria-invalid");
  });

  test("says when the server limits the searches", async () => {
    server.use(resumableSession());
    searchServer(() =>
      problem(429, "/api/v1/problems/rate-limited", "Too Many Requests", { headers: { "Retry-After": "120" } }),
    );
    const { user, field } = await openSearch();

    await user.type(field, "@dave@elsewhere.example{Enter}");

    expect(await screen.findByRole("alert")).toHaveTextContent("Too many searches. Try again in a few minutes.");
    expect(field).toHaveAttribute("aria-invalid", "true");
  });

  test("shows the server's reason for another failure", async () => {
    server.use(resumableSession());
    searchServer(() => problem(500, "about:blank", "Internal Server Error"));
    const { user, field } = await openSearch();

    await user.type(field, "@dave@elsewhere.example{Enter}");

    expect(await screen.findByRole("alert")).toHaveTextContent("Could not search for accounts: Internal Server Error");
  });

  test("starts with the search in the address", async () => {
    server.use(resumableSession());
    await openSearch("/search?q=%40carol%40remote.example");

    expect(screen.getByRole("searchbox")).toHaveValue("@carol@remote.example");
  });

  test("only finds the accounts this server knows while signed out, and offers to sign in", async () => {
    const { searches } = searchServer();
    const { user, field } = await openSearch();

    expect(field).toHaveAccessibleDescription(/Only accounts this server already knows are found\./);
    expect(screen.getByRole("link", { name: "Sign in to find accounts on other servers" })).toHaveAttribute(
      "href",
      "/login?return_to=%2Fsearch",
    );

    await user.type(field, "@dave@elsewhere.example{Enter}");
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "This server does not know that account. Sign in to look it up on its own server.",
    );
    expect(searches).toEqual([{ acct: "@dave@elsewhere.example", resolve: false }]);
  });
});

describe("an account on another server", () => {
  /** Carol's profile, whose server has not accepted the viewer's follow yet; returns the changes sent. */
  function carolServer(relationship: Parameters<typeof relationshipFixture>[1] = {}) {
    const changes: string[] = [];
    server.use(
      http.get(`${apiBase}/accounts/lookup`, () => HttpResponse.json(carol)),
      http.get(`${apiBase}/accounts/relationships`, () =>
        HttpResponse.json([relationshipFixture(carol.id, relationship)]),
      ),
      http.post(`${apiBase}/accounts/:id/:action`, ({ params }) => {
        const action = String(params["action"]);
        changes.push(action);
        // Her server answers the Follow later: until then the follow is a request.
        return HttpResponse.json(relationshipFixture(carol.id, action === "follow" ? { requested: true } : {}));
      }),
    );
    return { changes };
  }

  test("is followed with a request that waits for her server to accept it", async () => {
    server.use(resumableSession());
    const { changes } = carolServer();
    const { user } = await renderApp({ path: `/@${carol.acct}` });

    await user.click(await screen.findByRole("button", { name: "Follow" }));

    const requested = await screen.findByRole("button", { name: "Requested" });
    expect(requested).toHaveAccessibleDescription("Waiting for ⁨remote.example⁩ to accept your follow request.");
    expect(changes).toEqual(["follow"]);
    expect(await screen.findByText(/^Follow request sent to ⁨Carol⁩\.$/)).toBeInTheDocument();
  });

  test("withdraws the pending request after asking", async () => {
    server.use(resumableSession());
    const { changes } = carolServer({ requested: true });
    const { user } = await renderApp({ path: `/@${carol.acct}` });

    await user.click(await screen.findByRole("button", { name: "Requested" }));
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Withdraw request" }));

    expect(await screen.findByRole("button", { name: "Follow" })).toBeInTheDocument();
    expect(screen.queryByText(/Waiting for/)).not.toBeInTheDocument();
    expect(changes).toEqual(["unfollow"]);
  });

  test("unfollows once her server accepted, with no pending note", async () => {
    server.use(resumableSession());
    const { changes } = carolServer({ following: true });
    const { user } = await renderApp({ path: `/@${carol.acct}` });

    const following = await screen.findByRole("button", { name: "Following" });
    expect(screen.queryByText(/Waiting for/)).not.toBeInTheDocument();
    await user.click(following);
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Unfollow" }));

    expect(await screen.findByRole("button", { name: "Follow" })).toBeInTheDocument();
    expect(changes).toEqual(["unfollow"]);
  });

  test("is listed with her full handle among the accounts the viewer follows", async () => {
    server.use(resumableSession());
    server.use(
      http.get(`${apiBase}/accounts/${viewerFixture.id}/following`, () => HttpResponse.json({ accounts: [carol] })),
      http.get(`${apiBase}/accounts/${viewerFixture.id}/statuses`, () => HttpResponse.json(statusPage([]))),
    );
    await renderApp({ path: "/@alice/following" });

    await screen.findByRole("heading", { level: 1, name: "Accounts ⁨Alice⁩ follows" });
    const link = within(screen.getByRole("main")).getByRole("link", { name: "Carol @carol@remote.example" });
    expect(link).toHaveAttribute("href", "/@carol@remote.example");
  });
});

/** Serves GET /instance with the federation direction (ADR-0076): the modes in turn, one for each read, then the last. */
function federationServer(...modes: Schemas["InstanceFederation"]["mode"][]) {
  let reads = 0;
  server.use(
    http.get(`${apiBase}/instance`, () => {
      const mode = modes[Math.min(reads, modes.length - 1)] ?? "both";
      reads++;
      return HttpResponse.json({ ...instanceFixture, federation: { mode } });
    }),
  );
}

describe("while the instance takes nothing in from other servers", () => {
  test("the search finds only the accounts this server knows", async () => {
    server.use(resumableSession());
    federationServer("outbound");
    const { searches } = searchServer();
    const { user, field } = await openSearch();

    await waitFor(() => {
      expect(field).toHaveAccessibleDescription(/does not look up accounts of other servers now/);
    });
    await user.type(field, "@dave@elsewhere.example{Enter}");

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "This server does not know that account, and it does not look up accounts of other servers now.",
    );
    expect(searches).toEqual([{ acct: "@dave@elsewhere.example", resolve: false }]);
  });

  test("the search does not offer signing in to find accounts on other servers", async () => {
    federationServer("off");
    const { field } = await openSearch();

    await waitFor(() => {
      expect(field).toHaveAccessibleDescription(/does not look up accounts of other servers now/);
    });
    expect(screen.queryByRole("link", { name: "Sign in to find accounts on other servers" })).not.toBeInTheDocument();
  });

  test("a refusal for the federation direction stops the app offering what it refuses", async () => {
    server.use(resumableSession());
    // The operators turned federation off after the page read GET /instance.
    federationServer("both", "off");
    const { searches } = searchServer(() =>
      problem(
        403,
        "/api/v1/problems/federation-restricted",
        "This server does not federate in the direction that this needs.",
      ),
    );
    const { user, field } = await openSearch();

    await user.type(field, "@dave@elsewhere.example{Enter}");

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "This server does not know that account, and it does not look up accounts of other servers now.",
    );
    await waitFor(() => {
      expect(field).toHaveAccessibleDescription(/does not look up accounts of other servers now/);
    });
    await user.type(field, "x{Enter}");
    await waitFor(() => {
      expect(searches).toEqual([
        { acct: "@dave@elsewhere.example", resolve: true },
        { acct: "@dave@elsewhere.examplex", resolve: false },
      ]);
    });
  });

  test("a remote account's profile offers no new follow", async () => {
    server.use(resumableSession());
    federationServer("outbound");
    server.use(
      http.get(`${apiBase}/accounts/lookup`, () => HttpResponse.json(carol)),
      http.get(`${apiBase}/accounts/relationships`, () => HttpResponse.json([relationshipFixture(carol.id)])),
    );
    await renderApp({ path: `/@${carol.acct}` });

    expect(await screen.findByText("This server does not follow accounts of other servers now.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Follow" })).not.toBeInTheDocument();
    // Mute, block and report stay.
    expect(screen.getByRole("button", { name: "More actions for ⁨Carol⁩" })).toBeInTheDocument();
  });

  test("a remote account's profile still undoes an existing follow", async () => {
    server.use(resumableSession());
    federationServer("off");
    server.use(
      http.get(`${apiBase}/accounts/lookup`, () => HttpResponse.json(carol)),
      http.get(`${apiBase}/accounts/relationships`, () =>
        HttpResponse.json([relationshipFixture(carol.id, { following: true })]),
      ),
    );
    await renderApp({ path: `/@${carol.acct}` });

    expect(await screen.findByRole("button", { name: "Following" })).toBeInTheDocument();
    expect(screen.queryByText("This server does not follow accounts of other servers now.")).not.toBeInTheDocument();
  });
});
