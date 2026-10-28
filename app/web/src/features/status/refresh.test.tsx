// SPDX-License-Identifier: AGPL-3.0-only
// The actions on a post are in the lists that no page shows when they are made (web-client/README.md S1, ADR-0045):
// the viewer's profile opened afterwards shows the change from its first render. The server answers every list
// slowly, as over a real network, so that a list shown from a stale cache would be seen.
import { screen, within } from "@testing-library/react";
import { delay, http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import {
  type Schemas,
  statusPage,
  timelineStatusFixture,
  viewerFixture,
  viewerStatusFixture,
} from "../../test/fixtures";
import { apiBase, resumableSession } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

/** How long the server takes to answer a list, in ms. */
const latency = 300;

/** A server whose list of the viewer's posts is `posts` (newest first), answered slowly. */
function viewerPostsServer(posts: Schemas["Status"][]) {
  server.use(
    resumableSession(),
    http.get(`${apiBase}/accounts/:id/statuses`, async ({ request }) => {
      await delay(latency);
      // No pinned posts: the posts tab lists each post once.
      return HttpResponse.json(statusPage(new URL(request.url).searchParams.get("pinned") === "true" ? [] : posts));
    }),
  );
}

function navigation() {
  return screen.getByRole("navigation", { name: "Main" });
}

async function openProfile(user: Awaited<ReturnType<typeof renderApp>>["user"]) {
  await user.click(within(navigation()).getByRole("link", { name: "Profile" }));
  expect(
    await screen.findByRole("heading", { level: 1, name: viewerFixture.display_name }, { timeout: latency * 5 }),
  ).toBeInTheDocument();
  return screen.getByRole("feed", { name: "Posts" });
}

describe("a boost", () => {
  test("is on the viewer's profile at once when it opens again after boosting from the local timeline", async () => {
    const posts: Schemas["Status"][] = [];
    viewerPostsServer(posts);
    server.use(
      http.post(`${apiBase}/statuses/:id/boost`, () => {
        const boosted = { ...timelineStatusFixture, boosted: true, boosts_count: 1 };
        posts.unshift(viewerStatusFixture({ id: "0192f0c4-7a8e-7000-8000-0000000000b1", content: "", boost: boosted }));
        return HttpResponse.json(posts[0]);
      }),
    );
    const { user, router } = await renderApp({ path: "/profile" });
    expect(await screen.findByText("No posts yet", {}, { timeout: latency * 5 })).toBeInTheDocument();

    await router.navigate({ to: "/local" });
    const article = await within(await screen.findByRole("feed", { name: "Local timeline" })).findByRole("article", {
      description: "Hello from the local timeline!",
    });
    await user.click(within(article).getByRole("button", { name: "Boost", pressed: false }));
    expect(await screen.findByText("Boosted")).toBeInTheDocument();

    // The profile is marked stale, so its route fetches it again before it shows (web-client/README.md S7).
    const feed = await openProfile(user);
    // The profile shows the boost from its first render, not the list from before it.
    expect(within(feed).getByRole("article", { description: "Hello from the local timeline!" })).toBeInTheDocument();
  });
});

describe("a deleted post", () => {
  test("is gone from the viewer's profile at once when it opens again after deleting it on its page", async () => {
    const own = viewerStatusFixture({ content: "<p>My post</p>" });
    const posts = [own];
    viewerPostsServer(posts);
    server.use(
      http.get(`${apiBase}/statuses/:id`, () => HttpResponse.json(own)),
      http.delete(`${apiBase}/statuses/:id`, () => {
        posts.splice(0, posts.length);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user, router } = await renderApp({ path: "/profile" });
    expect(
      within(await screen.findByRole("feed", { name: "Posts" }, { timeout: latency * 5 })).getByRole("article", {
        description: "My post",
      }),
    ).toBeInTheDocument();

    await router.navigate({ to: "/@{$acct}/status/$id", params: { acct: viewerFixture.acct, id: own.id } });
    const article = await screen.findByRole("article", { description: "My post" });
    await user.click(await within(article).findByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }));
    await user.click(
      within(await screen.findByRole("alertdialog", { name: "Delete this post?" })).getByRole("button", {
        name: "Delete",
      }),
    );
    expect(await screen.findByText("Post deleted")).toBeInTheDocument();

    await router.navigate({ to: "/profile" });
    expect(await screen.findByRole("heading", { level: 1, name: viewerFixture.display_name })).toBeInTheDocument();
    // The profile no longer shows the post from its first render.
    expect(screen.getByText("No posts yet")).toBeInTheDocument();
  });
});
