// SPDX-License-Identifier: AGPL-3.0-only
// The page that the follow and reply buttons of other servers open (the Activity Intents of WebFinger, FEP-3b86,
// web-client/screens/intent.md) through the whole app (web-client/README.md T4): the real router and QueryClient, with
// MSW as the server. A signed-out visitor signs in first; nothing is looked up until the button is pressed; the found
// account's profile opens, or the found post with a reply started.
import type { RouterHistory } from "@tanstack/react-router";
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import { remoteAccountFixture, remoteStatusFixture } from "../../test/fixtures";
import { apiBase, problem, resumableSession } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { expectSignInStarted } from "../../test/signIn";

const carol = { ...remoteAccountFixture, followers_count: 3, following_count: 4 };
const post = remoteStatusFixture;

/** A server that finds Carol by her actor's URL and her post by its id, and answers other lookups with 404. */
function intentServer() {
  const lookups: string[] = [];
  server.use(
    http.get(`${apiBase}/accounts/lookup`, ({ request }) => {
      const params = new URL(request.url).searchParams;
      lookups.push(`account ${params.get("acct") ?? ""} resolve=${params.get("resolve") ?? ""}`);
      return params.get("acct") === carol.uri ? HttpResponse.json(carol) : problem(404, "about:blank", "Not Found");
    }),
    http.get(`${apiBase}/statuses/lookup`, ({ request }) => {
      const uri = new URL(request.url).searchParams.get("uri") ?? "";
      lookups.push(`status ${uri}`);
      return uri === post.uri ? HttpResponse.json(post) : problem(404, "about:blank", "Not Found");
    }),
    http.get(`${apiBase}/statuses/:id/context`, () => HttpResponse.json({ ancestors: [], descendants: [] })),
  );
  return { lookups };
}

function intentPath(params: Record<string, string>): string {
  return `/intent?${new URLSearchParams(params).toString()}`;
}

describe("the page of the Activity Intents", () => {
  test("sends a signed-out visitor to sign in first, and back afterwards", async () => {
    const path = intentPath({ follow: carol.uri });
    const { leftTo } = await renderApp({ path });

    await expectSignInStarted(leftTo, path);
  });

  test("shows the account's address, and opens its profile to follow once the button is pressed", async () => {
    server.use(resumableSession());
    const { lookups } = intentServer();
    const { user, router } = await renderApp({ path: intentPath({ follow: carol.uri }) });

    expect(await screen.findByRole("heading", { level: 1, name: "Open from another server" })).toBeInTheDocument();
    const main = within(screen.getByRole("main"));
    expect(main.getByText(carol.uri)).toBeInTheDocument();
    // Opening the page looks nothing up (FEP-3b86 §6.1).
    expect(lookups).toEqual([]);

    await user.click(main.getByRole("button", { name: "Show the account" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Carol" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/@carol@remote.example");
    expect(lookups).toEqual([`account ${carol.uri} resolve=true`]);
    // The intent's page is replaced: going back leaves for the page that sent the visitor.
    expect((router.history as RouterHistory).length).toBe(1);
  });

  test("opens the post with a reply to it started", async () => {
    server.use(resumableSession());
    const { lookups } = intentServer();
    const { user, router } = await renderApp({ path: intentPath({ reply: post.uri }) });

    await user.click(await screen.findByRole("button", { name: "Reply to the post" }));

    const sheet = await screen.findByRole("dialog", { name: "New post" });
    expect(within(sheet).getByText(/Replying to/)).toBeInTheDocument();
    expect(router.state.location.pathname).toBe(`/@${post.account.acct}/status/${post.id}`);
    expect(lookups).toEqual([`status ${post.uri}`]);
    await waitFor(() => {
      expect(within(sheet).getByRole("textbox", { name: /./ })).toHaveFocus();
    });
  });

  test("says when the address leads to nothing, next to the button", async () => {
    server.use(resumableSession());
    intentServer();
    const { user } = await renderApp({ path: intentPath({ reply: "https://remote.example/users/carol/statuses/2" }) });

    const button = await screen.findByRole("button", { name: "Reply to the post" });
    await user.click(button);

    expect(await screen.findByRole("alert")).toHaveTextContent("No post found at this address, or you cannot see it.");
    expect(
      within(screen.getByRole("main")).getByText("No post found at this address, or you cannot see it."),
    ).toBeInTheDocument();
    expect(button).toHaveFocus();
  });

  test("says when the server limits the lookups", async () => {
    server.use(
      resumableSession(),
      http.get(`${apiBase}/accounts/lookup`, () =>
        problem(429, "/api/v1/problems/rate-limited", "Too Many Requests", { headers: { "Retry-After": "120" } }),
      ),
    );
    const { user } = await renderApp({ path: intentPath({ follow: carol.uri }) });

    await user.click(await screen.findByRole("button", { name: "Show the account" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Too many lookups. Try again in a few minutes.");
  });

  test("says that a link without an account or a post names nothing", async () => {
    server.use(resumableSession());
    await renderApp({ path: "/intent" });

    expect(await screen.findByText("This link names no account or post to open.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Show the account" })).not.toBeInTheDocument();
    expect(within(screen.getByRole("main")).getByRole("link", { name: /home/i })).toHaveAttribute("href", "/home");
  });
});
