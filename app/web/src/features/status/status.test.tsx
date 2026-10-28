// SPDX-License-Identifier: AGPL-3.0-only
// A post's page and the actions on posts through the whole app (web-client/README.md T4): the real router and
// QueryClient, MSW for the client API. Time is fixed so that relative times are known.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { findLoadingRegion } from "../../test/loading";
import { apiBase, oauthPaths, problem, tokenResponse } from "../../test/msw/http";
import {
  remoteStatusFixture,
  statusFixture,
  statusPage,
  timelineStatusFixture,
  viewerFixture,
  viewerStatusFixture,
} from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import type { Status } from "./api/status";
import { newTabName } from "../../test/links";

const postId = "0192f0c4-7a8e-7000-8000-0000000000c1";
const ancestor: Status = { ...statusFixture("0192f0c4-7a8e-7000-8000-0000000000c0"), content: "<p>The question</p>" };
const post: Status = {
  ...statusFixture(postId),
  content: "<p>The post</p>",
  in_reply_to_id: ancestor.id,
  favourites_count: 2,
  boosts_count: 1,
  replies_count: 1,
};
const reply: Status = {
  ...viewerStatusFixture({
    id: "0192f0c4-7a8e-7000-8000-0000000000c2",
    content: "<p>The answer</p>",
    created_at: "2026-10-16T11:00:00Z",
  }),
  in_reply_to_id: postId,
};

/** Resumes a session with the token the default handlers accept. */
/** The announcer's polite live region; the app's offline bar is another status region, empty while online. */
function politeAnnouncements(): HTMLElement {
  const region = screen.getAllByRole("status").find((element) => element.hasAttribute("data-live-announcer"));
  if (region === undefined) throw new Error("the announcer is not rendered");
  return region;
}

function signedIn() {
  server.use(http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")));
}

/** Answers GET /statuses/{id} with `statuses` by ID, and the thread of `post`. */
function serveThread(...statuses: Status[]) {
  server.use(
    http.get(`${apiBase}/statuses/:id/context`, ({ params }) =>
      params["id"] === postId
        ? HttpResponse.json({ ancestors: [ancestor], descendants: [reply] })
        : HttpResponse.json({ ancestors: [], descendants: [] }),
    ),
    http.get(`${apiBase}/statuses/:id`, ({ params }) => {
      const found = [post, ancestor, reply, ...statuses].find((s) => s.id === params["id"]);
      return found === undefined ? problem(404, "/api/v1/problems/not-found", "Not Found") : HttpResponse.json(found);
    }),
  );
}

/** A request the test answers when it chooses, to look at the page while the request is on its way. */
function deferred() {
  let release: (response: Response) => void = () => undefined;
  const response = new Promise<Response>((resolve) => {
    release = resolve;
  });
  return {
    response,
    release: (answer: Response) => {
      release(answer);
    },
  };
}

/** Finds a visible toast by its text (the announcer's live regions say it too). */
async function findToast(text: string) {
  return within(await screen.findByRole("region", { name: "Messages" })).findByText(text);
}

/** The article of the post the page is about. */
async function findFocusedPost() {
  return screen.findByRole("article", { description: "The post" });
}

beforeEach(() => {
  // Only Date is faked: timers, MSW and user-event run as usual.
  vi.useFakeTimers({ toFake: ["Date"], now: new Date("2026-10-16T12:00:00Z") });
  sessionStorage.clear();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("a post's page", () => {
  test("shows the post between the posts it replies to and its replies, each under a heading", async () => {
    serveThread();
    await renderApp({ path: `/@bob/status/${postId}` });

    expect(await screen.findByRole("heading", { level: 1, name: "Post" })).toBeInTheDocument();
    const article = await findFocusedPost();
    // Named apart from the other posts of the thread; the relative time as everywhere, and no link to
    // the page it is on; the full date and the counts in words under the body.
    expect(article).toHaveAccessibleName("Opened post Bob @bob 3 hr. ago");
    expect(within(article).queryByRole("link", { name: /ago/ })).not.toBeInTheDocument();
    expect(within(article).getByText(/October 16, 2026/)).toHaveAttribute("datetime", post.created_at);
    expect(within(article).getByText("1 boost")).toBeInTheDocument();
    expect(within(article).getByText("2 favourites")).toBeInTheDocument();

    const earlier = await screen.findByRole("region", { name: "Earlier in the thread" });
    expect(within(earlier).getByRole("article", { description: "The question" })).toBeInTheDocument();
    const replies = screen.getByRole("region", { name: "Replies" });
    expect(await within(replies).findByRole("article", { description: "The answer" })).toBeInTheDocument();
    // The thread reads top to bottom: question, post, answer.
    expect(screen.getAllByRole("article").map((a) => a.textContent)).toEqual([
      expect.stringContaining("The question"),
      expect.stringContaining("The post"),
      expect.stringContaining("The answer"),
    ]);
    expect(document.title).toBe("Post – my example");
  });

  test("is where the time of a post in a timeline leads, and scrolls the post into view below its thread", async () => {
    serveThread();
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([post]))));
    const { user, router } = await renderApp({ path: "/local" });

    const feed = await screen.findByRole("feed", { name: "Local timeline" });
    const time = within(feed).getByRole("link", { name: "3 hr. ago" });
    expect(time).toHaveAttribute("href", `/@bob/status/${postId}`);
    const scrolled = vi.spyOn(Element.prototype, "scrollIntoView");
    await user.click(time);

    const heading = await screen.findByRole("heading", { level: 1, name: "Post" });
    expect(router.state.location.pathname).toBe(`/@bob/status/${postId}`);
    expect(await screen.findByRole("region", { name: "Earlier in the thread" })).toBeInTheDocument();
    // The post comes into view once the posts above it are there; focus stays on the page's heading.
    const article = await findFocusedPost();
    expect(scrolled).toHaveBeenCalled();
    expect(scrolled.mock.contexts.every((element) => element === article)).toBe(true);
    expect(heading).toHaveFocus();
  });

  // The server links them under https://{host}/ (ADR-0018), each with target="_blank", while the app runs at
  // http://localhost:3000, as on the local plain-HTTP setup; the link to another server opens in a new tab.
  const rel = 'rel="nofollow noopener noreferrer" target="_blank"';
  test.each([
    ["#cats", "/tags/cats"],
    ["@alice", "/@alice"],
    [newTabName("@carol"), "/local"],
  ])("opens the link %s of its body at %s, whatever the link's scheme", async (name, path) => {
    const linked: Status = {
      ...post,
      content: `<p><a href="https://localhost:3000/@alice" ${rel}>@alice</a> <a href="https://localhost:3000/tags/cats" ${rel}>#cats</a> <a href="https://remote.example/@carol" ${rel}>@carol</a></p>`,
    };
    serveThread(linked);
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([linked]))));
    const { user, router } = await renderApp({ path: "/local" });

    const feed = await screen.findByRole("feed", { name: "Local timeline" });
    const link = within(feed).getByRole("link", { name });
    if (typeof name === "string") {
      expect(link).not.toHaveAttribute("target");
      await user.click(link);
    } else {
      expect(link).toHaveAttribute("target", "_blank");
    }
    await waitFor(() => {
      expect(router.state.location.pathname).toBe(path);
    });
  });

  test("does not scroll a post that replies to nothing", async () => {
    const scrolled = vi.spyOn(Element.prototype, "scrollIntoView");
    await renderApp({ path: `/@bob/status/${postId}` });

    expect(await screen.findByText("No replies yet.")).toBeInTheDocument();
    expect(scrolled).not.toHaveBeenCalled();
  });

  test("does not mark the viewer's profile in the navigation on a page of the viewer's own post", async () => {
    signedIn();
    const own: Status = viewerStatusFixture({ id: "0192f0c4-7a8e-7000-8000-0000000000e2", content: "<p>Mine</p>" });
    serveThread(own);
    await renderApp({ path: `/@alice/status/${own.id}` });

    const article = await screen.findByRole("article", { description: "Mine" });
    // The actions show once the viewer is known.
    expect(await within(article).findByRole("button", { name: "More actions" })).toBeInTheDocument();
    const nav = screen.getByRole("navigation", { name: "Main" });
    expect(await within(nav).findByRole("link", { name: "Profile" })).not.toHaveAttribute("aria-current");
  });

  test("keeps the post on screen when its thread fails, and loads the thread again on request", async () => {
    let fail = true;
    serveThread();
    server.use(
      http.get(`${apiBase}/statuses/:id/context`, () =>
        fail
          ? problem(503, "about:blank", "Service Unavailable")
          : HttpResponse.json({ ancestors: [], descendants: [reply] }),
      ),
    );
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });

    expect(await screen.findByText("Could not load the replies: Service Unavailable")).toBeInTheDocument();
    expect(await findFocusedPost()).toBeInTheDocument();
    fail = false;
    await user.click(screen.getByRole("button", { name: "Try again" }));

    expect(await screen.findByRole("article", { description: "The answer" })).toBeInTheDocument();
  });

  test("shows the post at once and a skeleton in the busy replies section while the thread loads", async () => {
    const context = deferred();
    serveThread();
    server.use(http.get(`${apiBase}/statuses/:id/context`, () => context.response));
    await renderApp({ path: `/@bob/status/${postId}` });

    expect(await findFocusedPost()).toBeInTheDocument();
    const replies = screen.getByRole("region", { name: "Replies" });
    expect(await findLoadingRegion("Loading the replies")).toBe(replies);
    expect(replies).toHaveAttribute("aria-busy", "true");

    context.release(HttpResponse.json({ ancestors: [], descendants: [reply] }));
    expect(await within(replies).findByRole("article", { description: "The answer" })).toBeInTheDocument();
    expect(replies).toHaveAttribute("aria-busy", "false");
    expect(screen.queryByText("Loading the replies")).not.toBeInTheDocument();
  });

  test("says so when there are no replies", async () => {
    await renderApp({ path: `/@bob/status/${postId}` });

    expect(await screen.findByText("No replies yet.")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Earlier in the thread" })).not.toBeInTheDocument();
  });

  test("says that a post that does not exist, or that the viewer may not see, is not found", async () => {
    serveThread();
    await renderApp({ path: "/@bob/status/0192f0c4-7a8e-7000-8000-0000000000ff" });

    expect(await screen.findByRole("heading", { level: 1, name: "Post not found" })).toBeInTheDocument();
    expect(screen.getByText("This post does not exist, or you cannot see it.")).toBeInTheDocument();
  });

  test("offers no actions while signed out", async () => {
    serveThread();
    await renderApp({ path: `/@bob/status/${postId}` });

    const article = await findFocusedPost();
    expect(within(article).queryByRole("button", { name: "Favourite" })).not.toBeInTheDocument();
    expect(within(article).queryByRole("button", { name: "Reply" })).not.toBeInTheDocument();
  });
});

describe("the actions on a post", () => {
  test("favourite at once, announce it when the server agrees, and take it back the same way", async () => {
    signedIn();
    serveThread();
    const pending = deferred();
    const requests: string[] = [];
    server.use(
      http.post(`${apiBase}/statuses/:id/favourite`, async ({ request }) => {
        requests.push(`${request.method} ${new URL(request.url).pathname}`);
        return pending.response;
      }),
      http.delete(`${apiBase}/statuses/:id/favourite`, ({ request }) => {
        requests.push(`${request.method} ${new URL(request.url).pathname}`);
        return HttpResponse.json({ ...post, favourited: false, favourites_count: 2 });
      }),
    );
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    const article = await findFocusedPost();
    const favourite = await within(article).findByRole("button", { name: "Favourite", pressed: false });
    expect(favourite).toHaveAccessibleDescription("2");

    await user.click(favourite);

    // Optimistic: pressed with the count moved before the server answers.
    expect(favourite).toHaveAttribute("aria-pressed", "true");
    expect(favourite).toHaveAccessibleDescription("3");
    pending.release(HttpResponse.json({ ...post, favourited: true, favourites_count: 5 }));
    expect(await within(politeAnnouncements()).findByText("Added to favourites")).toBeInTheDocument();
    // The server's count, which includes the others' favourites.
    expect(favourite).toHaveAccessibleDescription("5");

    await user.click(favourite);

    expect(favourite).toHaveAttribute("aria-pressed", "false");
    expect(await within(politeAnnouncements()).findByText("Removed from favourites")).toBeInTheDocument();
    expect(requests).toEqual([
      `POST ${apiBase}/statuses/${postId}/favourite`,
      `DELETE ${apiBase}/statuses/${postId}/favourite`,
    ]);
  });

  test("set a failed boost back, and say why", async () => {
    signedIn();
    serveThread();
    server.use(http.post(`${apiBase}/statuses/:id/boost`, () => problem(503, "about:blank", "Service Unavailable")));
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    const boost = await within(await findFocusedPost()).findByRole("button", { name: "Boost", pressed: false });

    await user.click(boost);

    expect(await findToast("Could not save the change: Service Unavailable")).toBeInTheDocument();
    expect(
      within(screen.getByRole("alert")).getByText("Could not save the change: Service Unavailable"),
    ).toBeInTheDocument();
    expect(boost).toHaveAttribute("aria-pressed", "false");
    expect(boost).toHaveAccessibleDescription("1");
  });

  test("send presses one after another, so that the last one wins, and set back only the one that failed", async () => {
    signedIn();
    serveThread();
    const first = deferred();
    const sent: string[] = [];
    server.use(
      http.post(`${apiBase}/statuses/:id/bookmark`, () => {
        sent.push("on");
        return first.response;
      }),
      http.delete(`${apiBase}/statuses/:id/bookmark`, () => {
        sent.push("off");
        return problem(503, "about:blank", "Service Unavailable");
      }),
    );
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    const bookmark = await within(await findFocusedPost()).findByRole("button", { name: "Bookmark", pressed: false });

    await user.click(bookmark);
    await user.click(bookmark);

    // The second waits for the first.
    expect(bookmark).toHaveAttribute("aria-pressed", "false");
    expect(sent).toEqual(["on"]);
    first.release(HttpResponse.json({ ...post, bookmarked: true }));

    expect(await findToast("Could not save the change: Service Unavailable")).toBeInTheDocument();
    expect(sent).toEqual(["on", "off"]);
    // The removal failed, so the bookmark the server kept shows.
    expect(bookmark).toHaveAttribute("aria-pressed", "true");
  });

  test("show on every copy of the post: the boost of it in the same timeline, and its page", async () => {
    signedIn();
    serveThread();
    const booster = {
      ...viewerFixture,
      id: "0192f0c4-7a8e-7000-8000-000000000003",
      username: "carol",
      acct: "carol",
      display_name: "Carol",
    };
    const boostOfPost: Status = {
      ...timelineStatusFixture,
      id: "0192f0c4-7a8e-7000-8000-0000000000d1",
      account: booster,
      content: "",
      boost: post,
    };
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([boostOfPost, post]))));
    const { user } = await renderApp({ path: "/local" });
    const feed = await screen.findByRole("feed", { name: "Local timeline" });
    const [boosted, original] = await within(feed).findAllByRole("article", { description: "The post" });
    if (boosted === undefined || original === undefined) throw new Error("two copies expected");

    await user.click(await within(original).findByRole("button", { name: "Favourite", pressed: false }));

    expect(within(boosted).getByRole("button", { name: "Favourite" })).toHaveAttribute("aria-pressed", "true");
    expect(await within(politeAnnouncements()).findByText("Added to favourites")).toBeInTheDocument();

    // The page's own answer comes from the server, which now knows the favourite.
    server.use(
      http.get(`${apiBase}/statuses/:id`, () => HttpResponse.json({ ...post, favourited: true, favourites_count: 3 })),
    );
    await user.click(within(original).getByRole("link", { name: "3 hr. ago" }));

    expect(
      await within(await findFocusedPost()).findByRole("button", { name: "Favourite", pressed: true }),
    ).toBeInTheDocument();
  });

  test("copy the post's address", async () => {
    signedIn();
    serveThread();
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });

    await user.click(await within(await findFocusedPost()).findByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Copy link" }));

    expect(await findToast("Link copied")).toBeInTheDocument();
    // user-event's clipboard stands in for the browser's.
    expect(await navigator.clipboard.readText()).toBe(post.url);
  });

  test("reply: the form names the post, in the sheet", async () => {
    signedIn();
    serveThread();
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });

    const replyButton = await within(await findFocusedPost()).findByRole("button", { name: "Reply" });
    await user.click(replyButton);

    const sheet = await screen.findByRole("dialog", { name: "New post" });
    expect(within(sheet).getByText("Replying to Bob")).toBeInTheDocument();
  });
});

describe("deleting a post", () => {
  const own: Status = {
    ...viewerStatusFixture({ id: "0192f0c4-7a8e-7000-8000-0000000000e1", content: "<p>My post</p>" }),
    in_reply_to_id: postId,
  };

  test("is offered for the viewer's own posts only", async () => {
    signedIn();
    serveThread();
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });

    await user.click(await within(await findFocusedPost()).findByRole("button", { name: "More actions" }));

    expect(await screen.findByRole("menuitem", { name: "Copy link" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Delete" })).not.toBeInTheDocument();
  });

  test("asks first, then deletes the post and leaves its page", async () => {
    signedIn();
    serveThread(own);
    const deleted: string[] = [];
    server.use(
      http.delete(`${apiBase}/statuses/:id`, ({ params }) => {
        deleted.push(String(params["id"]));
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user, router } = await renderApp({ path: `/@alice/status/${own.id}` });
    const article = await screen.findByRole("article", { description: "My post" });

    await user.click(await within(article).findByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }));

    const dialog = await screen.findByRole("alertdialog", { name: "Delete this post?" });
    // The least destructive choice has the focus.
    expect(within(dialog).getByRole("button", { name: "Cancel" })).toHaveFocus();
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await findToast("Post deleted")).toBeInTheDocument();
    expect(deleted).toEqual([own.id]);
    // Opened directly, the page has nowhere to go back to: it leaves for the home timeline.
    expect(await screen.findByRole("heading", { level: 1, name: "Home" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/home");
  });

  test("keeps the post and the dialog when the server refuses", async () => {
    signedIn();
    serveThread(own);
    server.use(http.delete(`${apiBase}/statuses/:id`, () => problem(503, "about:blank", "Service Unavailable")));
    const { user } = await renderApp({ path: `/@alice/status/${own.id}` });

    await user.click(
      await within(await screen.findByRole("article", { description: "My post" })).findByRole("button", {
        name: "More actions",
      }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Delete this post?" });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await findToast("Could not delete the post: Service Unavailable")).toBeInTheDocument();
    expect(dialog).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(screen.getByRole("article", { description: "My post" })).toBeInTheDocument();
  });

  test("drops it from the timeline", async () => {
    signedIn();
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([own, post]))));
    const { user } = await renderApp({ path: "/local" });
    const feed = await screen.findByRole("feed", { name: "Local timeline" });
    const article = within(feed).getByRole("article", { description: "My post" });

    await user.click(await within(article).findByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }));
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Delete" }));

    expect(await findToast("Post deleted")).toBeInTheDocument();
    expect(within(feed).getAllByRole("article")).toHaveLength(1);
    expect(within(feed).getByRole("article", { description: "The post" })).toBeInTheDocument();
  });
});

describe("a post of another server", () => {
  const remote = remoteStatusFixture;

  test("shows the author's full handle, its content warning, its proxied pictures, and a link to its page there", async () => {
    signedIn();
    serveThread(remote);
    const { user } = await renderApp({ path: `/@${remote.account.acct}/status/${remote.id}` });

    const article = await screen.findByRole("article", { name: /^Opened post Carol @carol@remote\.example / });
    // The handle is cut on the domain's side when it does not fit; the link is named by all of it.
    expect(within(article).getByRole("link", { name: "Carol @carol@remote.example" })).toBeInTheDocument();
    expect(within(article).getByText("@carol@remote.example")).toHaveAttribute("dir", "ltr");
    // The link shows the host and is named by where it leads; the actions stay next to it.
    const original = within(article).getByRole("link", {
      name: "Open the original post on remote.example (opens in a new tab)",
    });
    expect(original).toHaveAttribute("href", "https://remote.example/@carol/1");
    expect(original).toHaveTextContent("remote.example");
    expect(await within(article).findByRole("button", { name: "Reply" })).toBeInTheDocument();

    expect(within(article).getByText("Cats")).toBeVisible();
    expect(within(article).getByText("Hello from another server!")).not.toBeVisible();
    await user.click(within(article).getByRole("button", { name: "Show content", expanded: false }));

    expect(within(article).getByText("Hello from another server!")).toBeVisible();
    // Remote files have no renditions here: the card loads the media proxy's preview_url.
    expect(within(article).getByRole("img", { name: "A cat on a sofa" })).toHaveAttribute(
      "src",
      "https://my.example/media/proxy/0192f0c4-7a8e-7000-8000-0000000003d1",
    );
    expect(within(article).getByRole("img", { name: "No description" })).toHaveAttribute(
      "src",
      "https://my.example/media/proxy/0192f0c4-7a8e-7000-8000-0000000003d2",
    );
    // Nothing is drawn for a picture without a description (web-client/README.md Y9); to a screen reader the name says so.
    expect(within(article).queryByText("No description", { selector: "span" })).not.toBeInTheDocument();
  });

  test("draws the custom emoji of its author's name, its content warning and its text, read as their shortcodes", async () => {
    signedIn();
    const blobcat = {
      shortcode: "blobcat",
      url: "https://my.example/media/proxy/e1/full",
      static_url: "https://my.example/media/proxy/e1/full",
    };
    const withEmoji: Status = {
      ...remote,
      account: { ...remote.account, display_name: "Carol :blobcat:", emojis: [blobcat] },
      spoiler_text: "Cats :blobcat:",
      content: "<p>Hello :blobcat: and :unknown:</p>",
      emojis: [blobcat],
    };
    serveThread(withEmoji);
    const { user } = await renderApp({ path: `/@${remote.account.acct}/status/${remote.id}` });

    const article = await screen.findByRole("article", {
      name: /^Opened post Carol :blobcat: @carol@remote\.example /,
    });
    expect(within(article).getByRole("link", { name: "Carol :blobcat: @carol@remote.example" })).toBeInTheDocument();
    await user.click(within(article).getByRole("button", { name: "Show content", expanded: false }));
    const emoji = within(article).getAllByRole("img", { name: ":blobcat:" });
    // The name, the content warning, and the text; the unknown shortcode stays text.
    expect(emoji).toHaveLength(3);
    for (const image of emoji) expect(image).toHaveAttribute("src", blobcat.static_url);
    expect(within(article).getByText(/and :unknown:/)).toBeVisible();
  });

  test("is the only kind of post with a link to another server", async () => {
    serveThread();
    await renderApp({ path: `/@bob/status/${postId}` });

    const article = await findFocusedPost();
    expect(within(article).queryByRole("link", { name: /^Open the original/ })).not.toBeInTheDocument();
  });

  test("links to its ActivityPub ID when it has no page", async () => {
    const withoutPage: Status = { ...remote };
    delete withoutPage.url;
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([withoutPage]))));
    await renderApp({ path: "/local" });

    const article = await screen.findByRole("article", { name: "Carol @carol@remote.example 3 hr. ago" });
    expect(
      within(article).getByRole("link", { name: "Open the original post on remote.example (opens in a new tab)" }),
    ).toHaveAttribute("href", remote.uri);
  });
});
