// SPDX-License-Identifier: AGPL-3.0-only
// Pinned posts through the whole app (web-client/README.md T4, FR-403): the real router and QueryClient, MSW for the
// client API. The viewer pins and unpins their own posts from a post's menu, and a profile's posts tab starts
// with the account's pinned posts.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { apiBase, resumableSession } from "../../test/msw/http";
import { type Schemas, statusFixture, statusPage, viewerFixture, viewerStatusFixture } from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import type { Status } from "./api/status";

const first: Status = viewerStatusFixture({ id: "0192f0c4-7a8e-7000-8000-0000000000f1", content: "<p>First post</p>" });
const second: Status = viewerStatusFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000000f2",
  content: "<p>Second post</p>",
});

/**
 * The viewer's profile, with the posts in `posts` of which `pinned` are pinned (newest pin first); the pins and
 * unpins change them, and a pin beyond `limit` is refused as the server does. Returns the requests it received.
 */
function viewerPosts({ posts, pinned = [], limit = 5 }: { posts: Status[]; pinned?: string[]; limit?: number }) {
  let pins = [...pinned];
  const requests: string[] = [];
  const withPin = (status: Status): Status => ({ ...status, pinned: pins.includes(status.id) });
  server.use(
    resumableSession(),
    http.get(`${apiBase}/accounts/:id/statuses`, ({ params, request }) => {
      if (params["id"] !== viewerFixture.id) return HttpResponse.json(statusPage([]));
      const isPinned = new URL(request.url).searchParams.get("pinned") === "true";
      requests.push(isPinned ? "GET pinned" : "GET posts");
      const items = isPinned ? pins.flatMap((id) => posts.filter((post) => post.id === id)) : posts;
      return HttpResponse.json(statusPage(items.map(withPin)));
    }),
    http.post(`${apiBase}/statuses/:id/pin`, ({ params }) => {
      const id = String(params["id"]);
      requests.push(`POST pin ${id}`);
      if (!pins.includes(id) && pins.length >= limit) {
        const body: Schemas["Problem"] = {
          type: "about:blank",
          title: "Unprocessable Content",
          status: 422,
          detail: "You can pin up to 5 posts.",
        };
        return HttpResponse.json(body, { status: 422, headers: { "Content-Type": "application/problem+json" } });
      }
      if (!pins.includes(id)) pins = [id, ...pins];
      const post = posts.find((p) => p.id === id);
      return HttpResponse.json(withPin(post ?? first));
    }),
    http.delete(`${apiBase}/statuses/:id/pin`, ({ params }) => {
      const id = String(params["id"]);
      requests.push(`DELETE pin ${id}`);
      pins = pins.filter((p) => p !== id);
      const post = posts.find((p) => p.id === id);
      return HttpResponse.json(withPin(post ?? first));
    }),
  );
  return { requests };
}

/** Finds a visible toast by its text (the announcer's live regions say it too). */
async function findToast(text: string) {
  return within(await screen.findByRole("region", { name: "Messages" })).findByText(text);
}

/** The feed of the profile's posts tab. */
async function findPostsFeed() {
  return screen.findByRole("feed", { name: "Posts" });
}

beforeEach(() => {
  sessionStorage.clear();
});

describe("a profile's pinned posts", () => {
  test("come first in the posts tab, each named as pinned, before the same posts in the timeline", async () => {
    viewerPosts({ posts: [first, second], pinned: [second.id] });
    await renderApp({ path: "/@alice" });
    const feed = await findPostsFeed();

    const articles = within(feed).getAllByRole("article");
    expect(articles).toHaveLength(3);
    expect(articles[0]).toHaveAccessibleName(/^Pinned post/);
    expect(articles[0]).toHaveAccessibleDescription("Second post");
    expect(articles[1]).not.toHaveAccessibleName(/^Pinned post/);
    expect(articles[1]).toHaveAccessibleDescription("First post");
    expect(articles[2]).toHaveAccessibleDescription("Second post");
  });

  test("are not in the other tabs", async () => {
    const { requests } = viewerPosts({ posts: [first, second], pinned: [second.id] });
    await renderApp({ path: "/@alice?tab=replies" });
    const feed = await screen.findByRole("feed", { name: "Posts and replies" });

    expect(within(feed).getAllByRole("article")).toHaveLength(2);
    expect(within(feed).queryByRole("article", { name: /^Pinned post/ })).not.toBeInTheDocument();
    expect(requests).not.toContain("GET pinned");
  });
});

describe("pinning a post", () => {
  test("pins the viewer's post from its menu, shows it at the top of the profile, and unpins it the same way", async () => {
    const { requests } = viewerPosts({ posts: [first, second] });
    const { user } = await renderApp({ path: "/@alice" });
    const feed = await findPostsFeed();
    expect(within(feed).getAllByRole("article")).toHaveLength(2);

    await user.click(
      await within(within(feed).getByRole("article", { description: "First post" })).findByRole("button", {
        name: "More actions",
      }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Pin to profile" }));

    expect(await findToast("Post pinned to your profile")).toBeInTheDocument();
    expect(requests).toContain(`POST pin ${first.id}`);
    // The pinned posts were read again before the toast, so the post is at the top already.
    const pinned = within(feed).getByRole("article", { name: /^Pinned post/ });
    expect(pinned).toHaveAccessibleDescription("First post");

    await user.click(within(pinned).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Unpin from profile" }));

    expect(await findToast("Post unpinned from your profile")).toBeInTheDocument();
    expect(requests).toContain(`DELETE pin ${first.id}`);
    expect(within(feed).queryByRole("article", { name: /^Pinned post/ })).not.toBeInTheDocument();
    expect(within(feed).getAllByRole("article")).toHaveLength(2);
  });

  test("says why when the server refuses a pin beyond the limit, and changes nothing", async () => {
    viewerPosts({ posts: [first, second], pinned: [second.id], limit: 1 });
    const { user } = await renderApp({ path: "/@alice" });
    const feed = await findPostsFeed();

    const firstPost = within(feed).getByRole("article", { description: "First post" });
    await user.click(await within(firstPost).findByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Pin to profile" }));

    expect(await findToast("Could not pin the post: You can pin up to 5 posts.")).toBeInTheDocument();
    expect(within(feed).getAllByRole("article", { name: /^Pinned post/ })).toHaveLength(1);
    await user.click(within(firstPost).getByRole("button", { name: "More actions" }));
    expect(await screen.findByRole("menuitem", { name: "Pin to profile" })).toBeInTheDocument();
  });

  test("is offered for the viewer's own posts that are not direct only", async () => {
    const direct: Status = viewerStatusFixture({
      id: "0192f0c4-7a8e-7000-8000-0000000000f3",
      content: "<p>Direct post</p>",
      visibility: "direct",
    });
    const others: Status = { ...statusFixture("0192f0c4-7a8e-7000-8000-0000000000f4"), content: "<p>Bob's post</p>" };
    viewerPosts({ posts: [first] });
    server.use(http.get(`${apiBase}/timelines/home`, () => HttpResponse.json(statusPage([first, direct, others]))));
    const { user } = await renderApp({ path: "/home" });
    const feed = await screen.findByRole("feed", { name: "Home" });

    await user.click(
      await within(within(feed).getByRole("article", { description: "First post" })).findByRole("button", {
        name: "More actions",
      }),
    );
    expect(await screen.findByRole("menuitem", { name: "Pin to profile" })).toBeInTheDocument();
    await user.keyboard("{Escape}");

    for (const description of ["Direct post", "Bob's post"]) {
      await user.click(
        within(within(feed).getByRole("article", { description })).getByRole("button", { name: "More actions" }),
      );
      expect(await screen.findByRole("menuitem", { name: "Copy link" })).toBeInTheDocument();
      expect(screen.queryByRole("menuitem", { name: "Pin to profile" })).not.toBeInTheDocument();
      await user.keyboard("{Escape}");
    }
  });
});
