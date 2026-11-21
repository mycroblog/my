// SPDX-License-Identifier: AGPL-3.0-only

// A timeline's column, from what a timeline hook gives it: its posts as the entries of a named feed, and what it says
// and offers while the timeline loads, fails, is empty, or has more. What a feed is to assistive technology and to the
// keyboard is the design system's (feed.dom.test.tsx).

import { describe, expect, onTestFinished, test } from "bun:test";
import { act, cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { Column, type ColumnTimeline } from "./column";
import { inLocale, lena, status as post } from "./test/post";

const status = (id: string) =>
  post({ id, account: { ...lena, display_name: `Author of ${id}` }, content: `<p>Post ${id}</p>` });

/** The column of a timeline with `over`, alone in the document, and what it asked of the timeline, in order. */
function column(over: Partial<ColumnTimeline> = {}, locale = "en", empty?: ReactNode) {
  cleanup();
  const asked: string[] = [];
  inLocale(
    <Column
      label="Home"
      statuses={[]}
      status="success"
      error={null}
      hasNextPage={false}
      isFetchingNextPage={false}
      failedNextPage={false}
      fetchNextPage={() => asked.push("next")}
      refetch={() => asked.push("again")}
      {...over}
      empty={empty}
    />,
    locale,
  );
  return asked;
}

const feed = (busy?: boolean) =>
  screen.getByRole("feed", busy === undefined ? { name: "Home" } : { name: "Home", busy });

/** The notice of the column, by its role: what it says without its button's words, and its buttons. */
function notice(role: "status" | "alert") {
  const found = screen.queryAllByRole(role).filter((element) => element.textContent !== "");
  const [one] = found;
  if (one === undefined || found.length > 1) return undefined;
  const buttons = within(one).queryAllByRole("button");
  const said = buttons.reduce((text, button) => text.replace(button.textContent, ""), one.textContent).trim();
  return { said, buttons: buttons.map((button) => button.textContent) };
}

describe("the feed", () => {
  test("its posts are the articles of a named feed, each with its position, in order", () => {
    column({ statuses: [status("a"), status("b")] });

    const entries = within(feed(false)).getAllByRole("article");
    expect(entries.map((entry) => entry.getAttribute("aria-posinset"))).toEqual(["1", "2"]);
    expect(entries.map((entry) => entry.getAttribute("aria-setsize"))).toEqual(["2", "2"]);
    expect(within(feed()).getByRole("article", { name: "Author of a" }) === entries[0]).toBe(true);
    expect(within(feed()).getByRole("article", { name: "Author of b" }) === entries[1]).toBe(true);
  });

  test("while more may follow, its size is unknown", () => {
    column({ statuses: [status("a")], hasNextPage: true });

    expect(within(feed()).getByRole("article").getAttribute("aria-setsize")).toBe("-1");
  });

  test("what the reader asks of a post reaches the screen", async () => {
    const user = userEvent.setup();
    const actions: unknown[] = [];
    cleanup();
    inLocale(
      <Column
        label="Home"
        statuses={[status("a"), status("b")]}
        status="success"
        error={null}
        hasNextPage={false}
        isFetchingNextPage={false}
        failedNextPage={false}
        fetchNextPage={() => undefined}
        refetch={() => undefined}
        onAction={(action) => actions.push(action)}
        postProps={(shown) => ({ context: <p>{`Context of ${shown.id}`}</p> })}
      />,
    );
    const second = within(screen.getByRole("article", { name: "Author of b" }));

    expect(second.getByText("Context of b")).not.toBeNull();
    await user.click(second.getByRole("button", { name: /^Favourite/ }));

    expect(actions).toEqual([{ type: "favourite", id: "b" }]);
  });
});

describe("its states", () => {
  test("loading the first posts, it says so, and is no feed before it has an entry", () => {
    column({ status: "pending" });

    expect(notice("status")).toEqual({ said: "Loading posts…", buttons: [] });
    expect(screen.queryAllByRole("feed")).toHaveLength(0);
  });

  test("when the first posts failed, it alerts with a retry, which fetches the timeline again", async () => {
    const user = userEvent.setup();
    const asked = column({ status: "error", error: new Error("x") });

    expect(notice("alert")).toEqual({ said: "The posts could not be loaded.", buttons: ["Try again"] });
    await user.click(screen.getByRole("button", { name: "Try again" }));

    expect(asked).toEqual(["again"]);
  });

  test("with no posts, it shows the feature's own empty state when given one", () => {
    column({}, "en", <p>Follow someone to fill your home.</p>);

    expect(screen.getByText("Follow someone to fill your home.")).not.toBeNull();
    expect(screen.queryAllByText("No posts yet.")).toHaveLength(0);
  });

  test("with no posts, it says there are none", () => {
    column();

    expect(notice("status")).toEqual({ said: "No posts yet.", buttons: [] });
  });

  test("loading more, it is busy and says so under its posts", () => {
    column({ statuses: [status("a")], hasNextPage: true, isFetchingNextPage: true });

    expect(feed(true)).not.toBeNull();
    expect(notice("status")).toEqual({ said: "Loading more posts…", buttons: [] });
    const said = screen.getByText("Loading more posts…");
    expect(screen.getByRole("article").compareDocumentPosition(said) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  });

  // As TanStack Query gives a next page that failed: the query's status is error, with the pages fetched before.
  test("when more failed, it alerts with a retry under the posts, which fetches the next page", async () => {
    const user = userEvent.setup();
    const failed = { status: "error", error: new Error("x"), failedNextPage: true } as const;
    const asked = column({ ...failed, statuses: [status("a")], hasNextPage: true });

    expect(notice("alert")).toEqual({ said: "More posts could not be loaded.", buttons: ["Try again"] });
    expect(screen.getByRole("article")).not.toBeNull();
    await user.click(screen.getByRole("button", { name: "Try again" }));

    expect(asked).toEqual(["next"]);
  });

  test("while the retry of more loads, it says that more load", () => {
    const failed = { status: "error", error: new Error("x"), failedNextPage: true } as const;
    column({ ...failed, statuses: [status("a")], hasNextPage: true, isFetchingNextPage: true });

    expect(notice("status")).toEqual({ said: "Loading more posts…", buttons: [] });
    expect(notice("alert")).toBeUndefined();
  });

  test("with posts and nothing to say, it shows no notice", () => {
    column({ statuses: [status("a")] });

    expect(notice("status")).toBeUndefined();
    expect(notice("alert")).toBeUndefined();
  });

  test("its words are in the reader's language", () => {
    column({ status: "pending" }, "ja");

    expect(notice("status")).toEqual({ said: "投稿を読み込んでいます…", buttons: [] });
  });
});

describe("the next page", () => {
  /** The browser's IntersectionObserver, held by the test: `reach` says that everything watched came into view. */
  function watching() {
    const watchers: { callback: IntersectionObserverCallback; watched: Element[]; self: IntersectionObserver }[] = [];
    const real = globalThis.IntersectionObserver;
    class Held implements IntersectionObserver {
      readonly root = null;
      readonly rootMargin = "";
      readonly scrollMargin = "";
      readonly thresholds = [];
      readonly #watcher: (typeof watchers)[number];
      constructor(callback: IntersectionObserverCallback) {
        this.#watcher = { callback, watched: [], self: this };
        watchers.push(this.#watcher);
      }
      observe(target: Element) {
        this.#watcher.watched.push(target);
      }
      unobserve(target: Element) {
        this.#watcher.watched = this.#watcher.watched.filter((element) => element !== target);
      }
      disconnect() {
        this.#watcher.watched = [];
      }
      takeRecords() {
        return [];
      }
    }
    globalThis.IntersectionObserver = Held;
    onTestFinished(() => {
      globalThis.IntersectionObserver = real;
    });
    return {
      reach: () => {
        act(() => {
          for (const { callback, watched, self } of watchers)
            if (watched.length > 0)
              callback(
                watched.map(
                  (target) => ({ isIntersecting: true, intersectionRatio: 1, target }) as IntersectionObserverEntry,
                ),
                self,
              );
        });
      },
    };
  }

  test("near its end with more to come, it asks for the next page", () => {
    const { reach } = watching();
    const asked = column({ statuses: [status("a")], hasNextPage: true });

    reach();

    expect(asked).toEqual(["next"]);
  });

  test("with no more to come, or when more failed, it asks for no more by itself", () => {
    const { reach } = watching();
    const last = column({ statuses: [status("a")] });
    reach();
    expect(last).toEqual([]);

    const failed = column({
      status: "error",
      error: new Error("x"),
      failedNextPage: true,
      statuses: [status("a")],
      hasNextPage: true,
    });
    reach();
    expect(failed).toEqual([]);
  });
});
