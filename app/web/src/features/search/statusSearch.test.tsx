// SPDX-License-Identifier: AGPL-3.0-only
// The search of posts through the whole app (web-client/README.md T4): the real router and QueryClient, with MSW as the
// server (FR-309, ADR-0149): what a server without it shows, the results and their paging, and the failures.
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import { instanceFixture, type Schemas, statusPage, timelineStatusFixture } from "../../test/fixtures";
import { apiBase, problem, resumableSession } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

/** A post with its own ID and text. */
function post(n: number, overrides: Partial<Schemas["Status"]> = {}): Schemas["Status"] {
  return {
    ...timelineStatusFixture,
    id: `0192f0c4-7a8e-7000-8000-${String(n).padStart(12, "0")}`,
    content: `<p>Post ${String(n)}</p>`,
    ...overrides,
  };
}

/** A signed-in viewer on a server whose search covers `visibilities`. */
function searchOn(visibilities: string[] = ["public"]) {
  server.use(
    resumableSession(),
    http.get(`${apiBase}/instance`, () =>
      HttpResponse.json({ ...instanceFixture, full_text_search: { available: true, visibilities } }),
    ),
  );
}

/** The search answering by `respond`; returns what it was asked, as `q` and `cursor`. */
function searchAnswers(respond: (cursor: string | null) => Response) {
  const asked: { q: string | null; cursor: string | null }[] = [];
  server.use(
    http.get(`${apiBase}/statuses/search`, ({ request }) => {
      const params = new URL(request.url).searchParams;
      const cursor = params.get("cursor");
      asked.push({ q: params.get("q"), cursor });
      return respond(cursor);
    }),
  );
  return asked;
}

/** Opens the search of posts and returns its field. */
async function openPostSearch(path = "/search?type=posts") {
  const { user, router } = await renderApp({ path });
  const field = await screen.findByRole("searchbox", { name: "Words to find" });
  return { user, router, field };
}

describe("a server without the search of posts", () => {
  test("shows no tabs and the account search as it is, also from an address of the search of posts", async () => {
    server.use(resumableSession());
    const asked = searchAnswers(() => HttpResponse.json(statusPage([])));
    await renderApp({ path: "/search?type=posts&q=bread" });

    const field = await screen.findByRole("searchbox", { name: "Handle or URL" });
    expect(field).toHaveValue("");
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
    expect(screen.queryByRole("searchbox", { name: "Words to find" })).not.toBeInTheDocument();
    expect(asked).toEqual([]);
  });
});

describe("a signed-out visitor", () => {
  test("has no search of posts on a server that has one", async () => {
    server.use(
      http.get(`${apiBase}/instance`, () =>
        HttpResponse.json({ ...instanceFixture, full_text_search: { available: true, visibilities: ["public"] } }),
      ),
    );
    const asked = searchAnswers(() => HttpResponse.json(statusPage([])));
    await renderApp({ path: "/search?type=posts&q=bread" });

    await screen.findByRole("searchbox", { name: "Handle or URL" });
    expect(screen.queryByRole("tab", { name: "Posts" })).not.toBeInTheDocument();
    expect(asked).toEqual([]);
  });
});

describe("the search of posts", () => {
  test("keeps the account search as the first tab, and the tab in the address", async () => {
    searchOn();
    const { user, router } = await renderApp({ path: "/search" });

    expect(await screen.findByRole("searchbox", { name: "Handle or URL" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Accounts" })).toHaveAttribute("aria-selected", "true");

    await user.click(screen.getByRole("tab", { name: "Posts" }));

    expect(await screen.findByRole("searchbox", { name: "Words to find" })).toBeInTheDocument();
    expect(router.state.location.search).toEqual({ type: "posts" });
  });

  test("searches only when sent, with the keyboard, and shows the posts as a timeline does", async () => {
    searchOn();
    const asked = searchAnswers(() =>
      HttpResponse.json(
        statusPage([post(1), post(2, { spoiler_text: "About bread", content: "<p>The hidden text</p>" })]),
      ),
    );
    const { user, field, router } = await openPostSearch();

    await user.type(field, "  sourdough bread ");
    expect(asked).toEqual([]);
    await user.keyboard("{Enter}");

    const feed = await screen.findByRole("feed", { name: "Posts" });
    expect(asked).toEqual([{ q: "sourdough bread", cursor: null }]);
    expect(await within(feed).findByText("Post 1")).toBeInTheDocument();
    // A content warning hides its post as in every timeline, behind the button that shows it.
    expect(within(feed).getByText("About bread")).toBeInTheDocument();
    expect(within(feed).getByRole("button", { name: /Show/ })).toBeInTheDocument();
    expect(router.state.location.search).toEqual({ type: "posts", q: "sourdough bread" });
    expect(field).toHaveFocus();
  });

  test("does not search during an IME composition, and searches once it has ended", async () => {
    searchOn();
    const asked = searchAnswers(() => HttpResponse.json(statusPage([post(1)])));
    const { user, field } = await openPostSearch();
    await user.click(field);

    // oxlint-disable testing-library/prefer-user-event -- user-event cannot compose or send isComposing.
    fireEvent.compositionStart(field);
    fireEvent.change(field, { target: { value: "ぱん" } });
    // Chrome and Firefox mark the Enter that commits a conversion with isComposing.
    expect(fireEvent.keyDown(field, { key: "Enter", isComposing: true })).toBe(false);
    // A submit that gets through while the IME composes sends nothing either.
    fireEvent.submit(screen.getByRole("search"));
    fireEvent.change(field, { target: { value: "パン" } });
    fireEvent.compositionEnd(field);
    // Safari sends that Enter after compositionend, with keyCode 229.
    expect(fireEvent.keyDown(field, { key: "Enter", keyCode: 229 })).toBe(false);
    // oxlint-enable testing-library/prefer-user-event
    expect(field).toHaveValue("パン");
    expect(asked).toEqual([]);

    await user.keyboard("{Enter}");
    await screen.findByRole("feed", { name: "Posts" });
    expect(asked).toEqual([{ q: "パン", cursor: null }]);
  });

  test("sends nothing for text of nothing but spaces", async () => {
    searchOn();
    const asked = searchAnswers(() => HttpResponse.json(statusPage([])));
    const { user, field } = await openPostSearch();

    await user.type(field, "   {Enter}");

    expect(await screen.findByText("Enter the words to find.")).toBeInTheDocument();
    expect(asked).toEqual([]);
  });

  test("searches the address of the page once, and shows it in the field", async () => {
    searchOn();
    const asked = searchAnswers(() => HttpResponse.json(statusPage([post(1)])));
    const { field } = await openPostSearch("/search?type=posts&q=bread");

    expect(field).toHaveValue("bread");
    expect(await screen.findByRole("feed", { name: "Posts" })).toBeInTheDocument();
    expect(asked).toEqual([{ q: "bread", cursor: null }]);
  });

  test("goes on through short and empty pages until no cursor is left, and only then says nothing was found", async () => {
    searchOn(["public", "local"]);
    const asked = searchAnswers((cursor) =>
      HttpResponse.json(cursor === null ? statusPage([], "page-2") : statusPage([])),
    );
    const { user } = await openPostSearch("/search?type=posts&q=bread");

    // An empty page with a cursor is not the end: the feed goes on (by itself in a browser, or by its button).
    await user.click(await screen.findByRole("button", { name: "Load more posts" }));

    expect(await screen.findByRole("heading", { level: 3, name: "No posts found" })).toBeInTheDocument();
    expect(asked.map(({ cursor }) => cursor)).toEqual([null, "page-2"]);
    expect(
      screen.getByText(
        'This server\'s search finds public posts and posts with the visibility "This server", only from accounts that allow it. Authors can opt out in their profile settings.',
      ),
    ).toBeInTheDocument();
  });

  test.each([
    { visibilities: ["public"], text: /finds public posts, only from accounts that allow it/ },
    { visibilities: ["local"], text: /finds posts with the visibility "This server", only from accounts/ },
  ])("says which posts can be found when the search covers $visibilities", async ({ visibilities, text }) => {
    searchOn(visibilities);
    searchAnswers(() => HttpResponse.json(statusPage([])));
    await openPostSearch("/search?type=posts&q=bread");

    expect(await screen.findByText(text)).toBeInTheDocument();
  });

  test("offers another try when the search engine fails, and keeps the text", async () => {
    searchOn();
    let fails = true;
    searchAnswers(() =>
      fails
        ? problem(503, "/api/v1/problems/search-failed", "The search is not working right now.")
        : HttpResponse.json(statusPage([post(1)])),
    );
    const { user, field } = await openPostSearch("/search?type=posts&q=bread");

    expect(await screen.findByText("The search is not working right now.")).toBeInTheDocument();
    expect(field).toHaveValue("bread");
    fails = false;
    await user.click(screen.getByRole("button", { name: "Try again" }));

    expect(await screen.findByText("Post 1")).toBeInTheDocument();
  });

  test("offers no other try over the rate limit, which only spends the limit", async () => {
    searchOn();
    const asked = searchAnswers(() =>
      problem(429, "/api/v1/problems/rate-limited", "Too many requests", { headers: { "Retry-After": "120" } }),
    );
    await openPostSearch("/search?type=posts&q=bread");

    expect(await screen.findByRole("heading", { level: 3, name: "Too many searches" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Try again" })).not.toBeInTheDocument();
    expect(asked).toHaveLength(1);
  });

  test("keeps the posts when a later page fails, and goes on after another try", async () => {
    searchOn();
    let fails = true;
    searchAnswers((cursor) => {
      if (cursor === null) return HttpResponse.json(statusPage([post(1)], "page-2"));
      return fails
        ? problem(503, "/api/v1/problems/search-failed", "Failed")
        : HttpResponse.json(statusPage([post(2)]));
    });
    const { user } = await openPostSearch("/search?type=posts&q=bread");

    await user.click(await screen.findByRole("button", { name: "Load more posts" }));
    expect(await screen.findByRole("button", { name: "Try again" })).toBeInTheDocument();
    expect(screen.getByText("Post 1")).toBeInTheDocument();

    fails = false;
    await user.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("Post 2")).toBeInTheDocument();
  });

  test("stops offering the search when the server says it has no search engine", async () => {
    let instanceReads = 0;
    server.use(
      resumableSession(),
      http.get(`${apiBase}/instance`, () => {
        instanceReads += 1;
        return HttpResponse.json({
          ...instanceFixture,
          full_text_search:
            instanceReads === 1
              ? { available: true, visibilities: ["public"] }
              : { available: false, visibilities: [] },
        });
      }),
    );
    searchAnswers(() => problem(404, "/api/v1/problems/search-unavailable", "This server cannot search posts."));
    await openPostSearch("/search?type=posts&q=bread");

    expect(await screen.findByRole("searchbox", { name: "Handle or URL" })).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
    });
  });
});
