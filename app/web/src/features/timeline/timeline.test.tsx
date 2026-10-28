// SPDX-License-Identifier: AGPL-3.0-only
// The timelines through the whole app (web-client/README.md T4): the real router and QueryClient, MSW for the
// client API. Time is fixed so that relative times are known.
import { isInaccessible, screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import type { Status } from "./api/timelines";
import { apiBase, oauthPaths, problem, tokenResponse } from "../../test/msw/http";
import {
  imageFixture,
  longDomainAccountFixture,
  longNameAccountFixture,
  rtlNameAccountFixture,
  statusPage,
  timelineStatusFixture,
} from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { autoplayKey } from "../../lib/storage";
import { newTabName } from "../../test/links";
import { renderApp } from "../../test/render";

/** A status like the fixture, with its own ID and text. */
function status(n: number, overrides: Partial<Status> = {}): Status {
  return {
    ...timelineStatusFixture,
    id: `0192f0c4-7a8e-7000-8000-${String(n).padStart(12, "0")}`,
    content: `<p>Post ${String(n)}</p>`,
    ...overrides,
  };
}

beforeEach(() => {
  // Only Date is faked: timers, MSW and user-event run as usual.
  vi.useFakeTimers({ toFake: ["Date"], now: new Date("2026-10-16T12:00:00Z") });
  sessionStorage.clear();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("the local timeline", () => {
  test("shows the posts as the articles of a feed named by the page heading", async () => {
    await renderApp({ path: "/local" });

    const feed = await screen.findByRole("feed", { name: "Local timeline" });
    const article = within(feed).getByRole("article", { name: "Bob @bob 3 hr. ago Public" });
    expect(article).toHaveAccessibleDescription("Hello from the local timeline!");
    expect(article).toHaveAttribute("aria-posinset", "1");
    // The last page: the feed knows its size and offers nothing more.
    expect(article).toHaveAttribute("aria-setsize", "1");
    expect(feed).toHaveAttribute("aria-busy", "false");
    expect(screen.queryByRole("button", { name: "Load more posts" })).not.toBeInTheDocument();
    expect(within(article).getByText("3 hr. ago")).toHaveAttribute("datetime", "2026-10-16T09:00:00Z");
  });

  test("loads the older pages by their cursor when asked", async () => {
    const cursors: (string | null)[] = [];
    server.use(
      http.get(`${apiBase}/timelines/local`, ({ request }) => {
        const cursor = new URL(request.url).searchParams.get("cursor");
        cursors.push(cursor);
        return HttpResponse.json(
          cursor === null ? statusPage([status(1), status(2)], "page-2") : statusPage([status(3)]),
        );
      }),
    );
    const { user } = await renderApp({ path: "/local" });

    const feed = await screen.findByRole("feed", { name: "Local timeline" });
    expect(within(feed).getAllByRole("article")).toHaveLength(2);
    // More to come: the size is unknown.
    expect(within(feed).getAllByRole("article")[0]).toHaveAttribute("aria-setsize", "-1");

    await user.click(screen.getByRole("button", { name: "Load more posts" }));

    expect(await within(feed).findByRole("article", { description: "Post 3" })).toHaveAttribute("aria-posinset", "3");
    expect(cursors).toEqual([null, "page-2"]);
    expect(
      within(feed)
        .getAllByRole("article")
        .map((a) => a.getAttribute("aria-setsize")),
    ).toEqual(["3", "3", "3"]);
    expect(screen.queryByRole("button", { name: "Load more posts" })).not.toBeInTheDocument();
  });

  test("says so when there are no posts", async () => {
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([]))));
    await renderApp({ path: "/local" });

    expect(await screen.findByRole("heading", { level: 2, name: "No posts yet" })).toBeInTheDocument();
    expect(screen.queryByRole("feed")).not.toBeInTheDocument();
  });

  test("shows the page's error when the first page fails, and loads it again on request", async () => {
    let fail = true;
    server.use(
      http.get(`${apiBase}/timelines/local`, () =>
        fail ? problem(503, "about:blank", "Service Unavailable") : HttpResponse.json(statusPage([status(1)])),
      ),
    );
    const { user } = await renderApp({ path: "/local" });

    expect(await screen.findByRole("heading", { level: 2, name: "Could not load this page" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Local timeline" })).toBeInTheDocument();
    fail = false;
    await user.click(screen.getByRole("button", { name: "Try again" }));
    // The router reruns the loader after its pending delay: about a second in jsdom.
    expect(await screen.findByRole("feed", { name: "Local timeline" })).toBeInTheDocument();
  });

  test("keeps the posts when an older page fails, announces it and tries again on request", async () => {
    let failures = 1;
    server.use(
      http.get(`${apiBase}/timelines/local`, ({ request }) => {
        if (new URL(request.url).searchParams.get("cursor") === null)
          return HttpResponse.json(statusPage([status(1)], "page-2"));
        if (failures-- > 0) return problem(503, "about:blank", "Service Unavailable");
        return HttpResponse.json(statusPage([status(2)]));
      }),
    );
    const { user } = await renderApp({ path: "/local" });
    const feed = await screen.findByRole("feed", { name: "Local timeline" });

    await user.click(screen.getByRole("button", { name: "Load more posts" }));

    expect(
      await within(screen.getByRole("alert")).findByText("Could not load more posts: Service Unavailable"),
    ).toBeInTheDocument();
    expect(within(feed).getAllByRole("article")).toHaveLength(1);
    // Shown under the posts as the region's error: what happened, why, and the way to try again (G16).
    expect(screen.getByText("Could not load more posts")).toBeVisible();
    expect(screen.getByText("Service Unavailable")).toBeVisible();
    // The feed stops loading by itself; only the retry button asks again.
    expect(screen.queryByRole("button", { name: "Load more posts" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Try again" }));

    expect(await within(feed).findByRole("article", { description: "Post 2" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Try again" })).not.toBeInTheDocument();
  });
});

describe("a status card", () => {
  test("shows a boost as the boosted post under the booster's name", async () => {
    const original = status(1, { created_at: "2026-10-16T11:55:00Z" });
    const boost = status(2, {
      account: {
        ...timelineStatusFixture.account,
        id: "0192f0c4-7a8e-7000-8000-000000000003",
        username: "carol",
        acct: "carol@remote.example",
        display_name: "",
      },
      content: "",
      created_at: "2026-10-16T11:59:30Z",
      boost: original,
    });
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([boost]))));
    await renderApp({ path: "/local" });

    // A booster without a display name is named by the username; the time is the post's.
    const article = await screen.findByRole("article", { name: "carol boosted Bob @bob 5 min. ago Public" });
    expect(article).toHaveAccessibleDescription("Post 1");
  });

  test("shows the visibility of every post under its time, at the end of the post's name", async () => {
    const posts = [
      status(1, { visibility: "local" }),
      status(2, { visibility: "private" }),
      status(3, { visibility: "direct" }),
    ];
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage(posts))));
    await renderApp({ path: "/local" });

    const feed = await screen.findByRole("feed", { name: "Local timeline" });
    for (const [post, name] of [
      ["Post 1", "This server"],
      ["Post 2", "Followers"],
      ["Post 3", "Mentioned people"],
    ] as const) {
      const article = within(feed).getByRole("article", { description: post });
      expect(article).toHaveAccessibleName(new RegExp(`${name}$`));
      expect(within(article).getByText(name)).toBeVisible();
    }
  });

  test("names the author's link by the whole name and handle, however long, in the name's own direction", async () => {
    const posts = [longNameAccountFixture, rtlNameAccountFixture, longDomainAccountFixture].map((account, n) =>
      status(n + 1, { account }),
    );
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage(posts))));
    await renderApp({ path: "/local" });

    const feed = await screen.findByRole("feed");
    // The card cuts the name and the handle to its width (e2e/post-header.spec.ts checks the lines); the
    // text stays whole, so the link says all of it.
    for (const account of [longNameAccountFixture, rtlNameAccountFixture, longDomainAccountFixture]) {
      const link = within(feed).getByRole("link", { name: `${account.display_name} @${account.acct}` });
      expect(within(link).getByText(account.display_name)).toHaveAttribute("dir", "auto");
      expect(within(link).getByText(`@${account.acct}`)).toHaveAttribute("dir", "ltr");
    }
  });

  test("shows the author's avatar, and the first letter of the name without one", async () => {
    const pictured = status(1, {
      account: { ...timelineStatusFixture.account, avatar: imageFixture("https://my.example/media/avatar/small.webp") },
    });
    const plain = status(2);
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([pictured, plain]))));
    await renderApp({ path: "/local" });

    const [withAvatar, withoutAvatar] = await screen.findAllByRole("article");
    if (withAvatar === undefined || withoutAvatar === undefined) throw new Error("want two articles");
    // The avatar is decorative (alt=""), and its link is hidden: the name is next to it.
    expect(within(withAvatar).getByRole("presentation", { hidden: true })).toHaveAttribute(
      "src",
      "https://my.example/media/avatar/small.webp",
    );
    expect(within(withoutAvatar).queryByRole("presentation", { hidden: true })).not.toBeInTheDocument();
    expect(within(withoutAvatar).getByText("B")).toBeInTheDocument();
  });

  test("links the avatar for the pointer only: the name is the one tab stop to the profile", async () => {
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([status(1)]))));
    const { user } = await renderApp({ path: "/local" });

    const article = await screen.findByRole("article");
    // Screen readers meet the profile once, on the name, and then the time.
    const links = within(article).getAllByRole("link");
    expect(links).toHaveLength(2);
    expect(links[0]).toHaveAccessibleName("Bob @bob");
    expect(links[1]).toHaveAccessibleName("3 hr. ago");
    // The avatar's link, to the same profile, is out of the tab order inside an element hidden from them.
    const name = within(article).getByRole("link", { name: "Bob @bob" });
    const [avatar] = within(article).getAllByRole("link", { hidden: true });
    if (avatar === undefined) throw new Error("want the avatar's link");
    expect(isInaccessible(avatar)).toBe(true);
    expect(avatar).toHaveAttribute("href", name.getAttribute("href"));
    expect(avatar).toHaveAttribute("tabindex", "-1");

    // The keyboard goes from the name straight to the time, and back from the name past the avatar.
    name.focus();
    await user.tab();
    expect(within(article).getByRole("link", { name: "3 hr. ago" })).toHaveFocus();
    await user.tab({ shift: true });
    expect(name).toHaveFocus();
    await user.tab({ shift: true });
    expect(avatar).not.toHaveFocus();
  });

  test("hides a post behind its content warning, and its sensitive media behind their own toggle", async () => {
    const warned = status(1, {
      spoiler_text: "Cake",
      sensitive: true,
      created_at: "2026-10-01T09:00:00Z",
      media_attachments: [
        {
          id: "0192f0c4-7a8e-7000-8000-0000000000m1",
          type: "image",
          state: "ready",
          meta: { focus: { x: 0, y: 0 } },
          variants: {
            small: {
              url: "https://my.example/media/thumb.webp",
              content_type: "image/webp",
              width: 640,
              height: 360,
              size: 100,
            },
            medium: {
              url: "https://my.example/media/medium.webp",
              content_type: "image/webp",
              width: 1600,
              height: 900,
              size: 500,
            },
            large: {
              url: "https://my.example/media/full.jpg",
              content_type: "image/jpeg",
              width: 1600,
              height: 900,
              size: 1000,
            },
          },
          url: "https://my.example/media/full.jpg",
          preview_url: "https://my.example/media/thumb.webp",
          blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
        },
      ],
    });
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([warned]))));
    const { user } = await renderApp({ path: "/local" });

    // Two weeks ago: the date instead of a relative time.
    const article = await screen.findByRole("article", { name: "Bob @bob Oct 1, 2026 Public" });
    expect(within(article).getByText("Cake")).toBeVisible();
    expect(within(article).getByText("Post 1")).not.toBeVisible();

    await user.click(within(article).getByRole("button", { name: "Show content", expanded: false }));

    expect(within(article).getByText("Post 1")).toBeVisible();
    const reveal = within(article).getByRole("button", { name: "Show sensitive media", pressed: false });
    expect(within(article).queryByRole("img")).not.toBeInTheDocument();
    // The blur of the picture (its blurhash) covers it while it is concealed, hidden from screen readers.
    // eslint-disable-next-line testing-library/no-node-access -- the decorative canvas has no role.
    expect(article.querySelector("canvas")).toHaveAttribute("aria-hidden", "true");
    await user.click(reveal);
    // No description: the picture is named so, and not skipped.
    expect(within(article).getByRole("img", { name: "No description" })).toHaveAttribute(
      "src",
      "https://my.example/media/thumb.webp",
    );
  });

  test("shows the card of a post's link as one link, behind the content warning with the body", async () => {
    const linked = status(1, {
      spoiler_text: "News",
      sensitive: true,
      card: {
        url: "https://news.example/articles/sunrise",
        title: "The sun rose again",
        description: "Observers report a sunrise.",
        site_name: "Example News",
        language: "en",
        author_name: "Alice Example",
        published_at: "2026-10-02T09:00:00Z",
        image: {
          url: "https://my.example/media/proxy/card/small?sig=x",
          width: 1200,
          height: 630,
          description: "A sunrise",
        },
      },
    });
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([linked]))));
    const { user } = await renderApp({ path: "/local" });

    const article = await screen.findByRole("article", { name: /^Bob @bob/ });
    expect(within(article).queryByRole("link", { name: "The sun rose again" })).not.toBeInTheDocument();

    await user.click(within(article).getByRole("button", { name: "Show content", expanded: false }));

    const link = within(article).getByRole("link", { name: newTabName("The sun rose again") });
    expect(link).toHaveAttribute("href", "https://news.example/articles/sunrise");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer nofollow");
    expect(within(article).getByText("Observers report a sunrise.")).toHaveAttribute("lang", "en");
    expect(within(article).getByText("By Alice Example · Oct 2, 2026")).toBeVisible();
    // The post is sensitive: the card's image waits for its own toggle.
    expect(within(article).queryByRole("img", { name: "A sunrise" })).not.toBeInTheDocument();
    await user.click(within(article).getByRole("button", { name: "Show sensitive media", pressed: false }));
    expect(within(article).getByRole("img", { name: "A sunrise" })).toHaveAttribute(
      "src",
      "https://my.example/media/proxy/card/small?sig=x",
    );
  });

  test("moves an animated picture only without reduced motion, and not at all with autoplay off", async () => {
    const variant = (name: string, width: number) => ({
      url: `https://my.example/media/${name}`,
      width,
      height: width / 2,
    });
    const dancing = status(1, {
      media_attachments: [
        {
          id: "0192f0c4-7a8e-7000-8000-0000000000m2",
          type: "image",
          state: "ready",
          description: "A dance",
          meta: { focus: { x: 0, y: 0 } },
          variants: {
            small: variant("s.webp", 640),
            medium: variant("m.webp", 1920),
            large: variant("l.jpg", 3840),
            animated: {
              small: variant("s-a.webp", 640),
              medium: variant("m-a.webp", 1920),
              large: variant("l-a.gif", 3840),
            },
          },
          url: "https://my.example/media/l-a.gif",
          preview_url: "https://my.example/media/s.webp",
        },
      ],
    });
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([dancing]))));
    const { unmount } = await renderApp({ path: "/local" });
    const picture = await screen.findByRole("img", { name: "A dance" });
    expect(picture).toHaveAttribute(
      "srcset",
      "https://my.example/media/s.webp 640w, https://my.example/media/m.webp 1920w, https://my.example/media/l.jpg 3840w",
    );
    // eslint-disable-next-line testing-library/no-node-access -- <picture> and <source> have no role.
    expect(picture.parentElement?.querySelector("source")).toHaveAttribute(
      "media",
      "(prefers-reduced-motion: no-preference)",
    );
    unmount();

    autoplayKey.write(false);
    await renderApp({ path: "/local" });
    const still = await screen.findByRole("img", { name: "A dance" });
    // eslint-disable-next-line testing-library/no-node-access -- <picture> and <source> have no role.
    expect(still.parentElement?.querySelector("source")).toBeNull();
    expect(still).toHaveAttribute("src", "https://my.example/media/s.webp");
  });

  test("opens a picture at its original size in the viewer, and steps to the next one", async () => {
    const pictures = status(1, {
      media_attachments: [
        {
          id: "0192f0c4-7a8e-7000-8000-0000000000m3",
          type: "image",
          state: "ready",
          description: "A cat",
          meta: { focus: { x: 0, y: 0 }, width: 4000, height: 3000 },
          variants: {
            small: { url: "https://my.example/media/cat-s.webp", width: 640, height: 480 },
            medium: { url: "https://my.example/media/cat-m.webp", width: 1920, height: 1440 },
            large: { url: "https://my.example/media/cat-l.jpg", width: 3840, height: 2880 },
          },
          url: "https://my.example/media/cat-l.jpg",
          preview_url: "https://my.example/media/cat-s.webp",
        },
        {
          // A remote file: no renditions, and its original through the media proxy.
          id: "0192f0c4-7a8e-7000-8000-0000000000m4",
          type: "image",
          state: "ready",
          meta: { focus: { x: 0, y: 0 }, width: 800, height: 600 },
          url: "https://my.example/media/proxy/dog/original?sig=x",
          preview_url: "https://my.example/media/proxy/dog/small?sig=x",
          remote_url: "https://remote.example/files/dog.png",
        },
      ],
    });
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([pictures]))));
    const { user } = await renderApp({ path: "/local" });

    const article = await screen.findByRole("article", { name: /^Bob @bob/ });
    const cat = within(article).getByRole("button", { name: "A cat" });
    await user.click(cat);
    const dialog = await screen.findByRole("dialog", { name: "Image 1 of 2" });
    expect(within(dialog).getByRole("img", { name: "A cat" })).toHaveAttribute(
      "src",
      "https://my.example/media/cat-l.jpg",
    );

    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("dialog", { name: "Image 2 of 2" })).toBeInTheDocument();
    expect(within(dialog).getByRole("img", { name: "No description" })).toHaveAttribute(
      "src",
      "https://my.example/media/proxy/dog/original?sig=x",
    );

    await user.click(within(dialog).getByRole("button", { name: "Close" }));
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(cat).toHaveFocus();
    });

    // By keyboard alone: Enter on the focused picture opens it, Esc closes it.
    await user.keyboard("{Enter}");
    expect(await screen.findByRole("dialog", { name: "Image 1 of 2" })).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
  });

  test("plays videos and audio files, local and remote, only when asked to", async () => {
    const variant = (name: string, width: number) => ({
      url: `https://my.example/media/${name}`,
      width,
      height: (width * 9) / 16,
    });
    const playing = status(1, {
      media_attachments: [
        {
          id: "0192f0c4-7a8e-7000-8000-0000000000m3",
          type: "video",
          state: "ready",
          description: "A cat jumps",
          meta: { focus: { x: 0, y: 0 }, width: 1920, height: 1080, duration: 12.5 },
          variants: { small: variant("s.webp", 640), medium: variant("m.webp", 1920), large: variant("l.jpg", 1920) },
          url: "https://my.example/media/large-animated.mp4",
          preview_url: "https://my.example/media/s.webp",
        },
        {
          id: "0192f0c4-7a8e-7000-8000-0000000000m4",
          type: "audio",
          state: "ready",
          meta: { focus: { x: 0, y: 0 }, duration: 61 },
          url: "https://my.example/media/large-animated.ogg",
        },
        {
          // A remote video: served through the media proxy, without a poster.
          id: "0192f0c4-7a8e-7000-8000-0000000000m5",
          type: "video",
          state: "ready",
          description: "A remote clip",
          meta: { focus: { x: 0, y: 0 } },
          url: "https://my.example/proxy/media/m5/full",
          remote_url: "https://remote.example/files/clip.mp4",
        },
      ],
    });
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([playing]))));
    await renderApp({ path: "/local" });
    const article = await screen.findByRole("article", { description: "Post 1" });
    // eslint-disable-next-line testing-library/no-node-access -- <video> and <audio> have no ARIA role.
    const players = Array.from(article.querySelectorAll("video, audio"));
    expect(
      players.map((p) => [
        p.tagName,
        p.getAttribute("src"),
        p.getAttribute("poster"),
        p.hasAttribute("controls"),
        p.hasAttribute("autoplay"),
      ]),
    ).toEqual([
      ["VIDEO", "https://my.example/media/large-animated.mp4", "https://my.example/media/l.jpg", true, false],
      ["AUDIO", "https://my.example/media/large-animated.ogg", null, false, false],
      ["VIDEO", "https://my.example/proxy/media/m5/full", null, true, false],
    ]);
    expect(players[0]).toHaveAccessibleName("A cat jumps");
    // An audio file is played by the kit's player, named by the description, not by the browser's controls.
    expect(within(article).getByRole("group", { name: "No description" })).toBeInTheDocument();
    expect(players[2]).toHaveAccessibleName("A remote clip");
  });

  test("formats the relative time in the page's locale", async () => {
    await renderApp({ path: "/local", locale: "ja" });

    const feed = await screen.findByRole("feed", { name: "ローカルタイムライン" });
    expect(within(feed).getByRole("article", { name: /3 時間前 公開$/ })).toBeInTheDocument();
  });
});

describe("the hashtag timeline", () => {
  test("is titled after the hashtag and asks for its posts", async () => {
    const hashtags: string[] = [];
    server.use(
      http.get(`${apiBase}/timelines/tag/:hashtag`, ({ params }) => {
        hashtags.push(String(params["hashtag"]));
        return HttpResponse.json(statusPage([]));
      }),
    );
    await renderApp({ path: "/tags/cats" });

    expect(await screen.findByRole("heading", { level: 1, name: "#cats" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 2, name: "No posts with #cats" })).toBeInTheDocument();
    expect(document.title).toBe("#cats – my example");
    expect(hashtags).toEqual(["cats"]);
  });
});

describe("the home timeline", () => {
  test("sends to sign in without a session, and back here after it", async () => {
    const { router } = await renderApp({ path: "/home" });

    expect(await screen.findByRole("heading", { level: 1, name: "Sign in" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/login");
    expect(router.state.location.search).toEqual({ return_to: "/home" });
  });

  test("shows the signed-in user's timeline", async () => {
    server.use(http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")));
    await renderApp({ path: "/home" });

    const feed = await screen.findByRole("feed", { name: "Home" });
    expect(within(feed).getByRole("article", { description: "Hello from the local timeline!" })).toBeInTheDocument();
  });

  test("says how to fill an empty timeline", async () => {
    server.use(
      http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")),
      http.get(`${apiBase}/timelines/home`, () => HttpResponse.json(statusPage([]))),
    );
    await renderApp({ path: "/home" });

    expect(await screen.findByRole("heading", { level: 2, name: "Your home timeline is empty" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Browse the local timeline" })).toHaveAttribute("href", "/local");
  });
});

describe("the bookmarks", () => {
  test("send to sign in without a session, and back here after it", async () => {
    const { router } = await renderApp({ path: "/bookmarks" });

    await screen.findByRole("heading", { level: 1 });
    expect(router.state.location.pathname).toBe("/login");
    expect(router.state.location.search).toEqual({ return_to: "/bookmarks" });
  });

  test("list the bookmarked posts as a feed, opened from the navigation", async () => {
    server.use(http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")));
    const { user } = await renderApp({ path: "/home" });
    await screen.findByRole("feed", { name: "Home" });
    const navigation = screen.getByRole("navigation", { name: "Main" });

    await user.click(within(navigation).getByRole("link", { name: "Bookmarks" }));

    const feed = await screen.findByRole("feed", { name: "Bookmarks" });
    const article = within(feed).getByRole("article", { description: "Hello from the local timeline!" });
    expect(within(article).getByRole("button", { name: "Bookmark", pressed: true })).toBeInTheDocument();
    expect(within(navigation).getByRole("link", { name: "Bookmarks" })).toHaveAttribute("aria-current", "page");
  });

  test("load the older pages by their cursor", async () => {
    const cursors: (string | null)[] = [];
    server.use(
      http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")),
      http.get(`${apiBase}/bookmarks`, ({ request }) => {
        const cursor = new URL(request.url).searchParams.get("cursor");
        cursors.push(cursor);
        return HttpResponse.json(
          cursor === null
            ? statusPage([status(1, { bookmarked: true })], "page-2")
            : statusPage([status(2, { bookmarked: true })]),
        );
      }),
    );
    const { user } = await renderApp({ path: "/bookmarks" });
    const feed = await screen.findByRole("feed", { name: "Bookmarks" });

    await user.click(screen.getByRole("button", { name: "Load more posts" }));

    expect(await within(feed).findByRole("article", { description: "Post 2" })).toBeInTheDocument();
    expect(cursors).toEqual([null, "page-2"]);
  });

  test("say what fills an empty list", async () => {
    server.use(
      http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")),
      http.get(`${apiBase}/bookmarks`, () => HttpResponse.json(statusPage([]))),
    );
    await renderApp({ path: "/bookmarks" });

    expect(await screen.findByRole("heading", { level: 2, name: "No bookmarks yet" })).toBeInTheDocument();
    expect(screen.getByText("Posts you bookmark appear here. Only you can see your bookmarks.")).toBeInTheDocument();
  });

  test("gain a post bookmarked elsewhere, and lose one whose bookmark is removed once opened again", async () => {
    const bookmarked = new Map<string, Status>();
    const local = status(1);
    server.use(
      http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")),
      http.get(`${apiBase}/timelines/local`, () =>
        HttpResponse.json(statusPage([{ ...local, bookmarked: bookmarked.has(local.id) }])),
      ),
      http.get(`${apiBase}/bookmarks`, () => HttpResponse.json(statusPage([...bookmarked.values()]))),
      http.post(`${apiBase}/statuses/:id/bookmark`, () => {
        const answer = { ...local, bookmarked: true };
        bookmarked.set(local.id, answer);
        return HttpResponse.json(answer);
      }),
      http.delete(`${apiBase}/statuses/:id/bookmark`, () => {
        bookmarked.delete(local.id);
        return HttpResponse.json({ ...local, bookmarked: false });
      }),
    );
    const { user } = await renderApp({ path: "/bookmarks" });
    expect(await screen.findByRole("heading", { level: 2, name: "No bookmarks yet" })).toBeInTheDocument();
    const navigation = screen.getByRole("navigation", { name: "Main" });

    await user.click(within(navigation).getByRole("link", { name: "Local" }));
    const timeline = await screen.findByRole("feed", { name: "Local timeline" });
    await user.click(within(timeline).getByRole("button", { name: "Bookmark", pressed: false }));
    expect(await within(politeAnnouncer()).findByText("Bookmarked")).toBeInTheDocument();
    await user.click(within(navigation).getByRole("link", { name: "Bookmarks" }));

    const feed = await screen.findByRole("feed", { name: "Bookmarks" });
    const toggle = within(within(feed).getByRole("article", { description: "Post 1" })).getByRole("button", {
      name: "Bookmark",
      pressed: true,
    });

    await user.click(toggle);

    // The list being read keeps the post, shown unbookmarked, so that the press can be undone.
    expect(await within(politeAnnouncer()).findByText("Bookmark removed")).toBeInTheDocument();
    expect(toggle).toHaveAttribute("aria-pressed", "false");
    expect(within(feed).getByRole("article", { description: "Post 1" })).toBeInTheDocument();

    await user.click(within(navigation).getByRole("link", { name: "Local" }));
    await screen.findByRole("feed", { name: "Local timeline" });
    await user.click(within(navigation).getByRole("link", { name: "Bookmarks" }));

    expect(await screen.findByRole("heading", { level: 2, name: "No bookmarks yet" })).toBeInTheDocument();
  });
});

/** The announcer's polite live region; the app's offline bar is another status region, empty while online. */
function politeAnnouncer(): HTMLElement {
  const region = screen.getAllByRole("status").find((element) => element.hasAttribute("data-live-announcer"));
  if (region === undefined) throw new Error("the announcer is not rendered");
  return region;
}
