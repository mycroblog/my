// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { Button, TextButton } from "./button";
import { Entry } from "./entry";
import { IconButton } from "./icon-button";
import {
  Feed,
  FeedItem,
  FeedNotice,
  moveFeedFocus,
  showNewest,
  useFeedPosition,
  type FeedPosition,
  type FeedProps,
} from "./feed";

/** What an entry was asked to scroll with, by the entry's position in its feed. */
type Scroll = { entry: string | null; options: ScrollIntoViewOptions | boolean | undefined };

// The simulation lays nothing out, so nothing scrolls: the tests read what a feed asks of the browser instead.
const scrolls: Scroll[] = [];
const scrollIntoView = Object.getOwnPropertyDescriptor(Element.prototype, "scrollIntoView");
const matchMedia = window.matchMedia.bind(window);

/** The next frame: a feed moves at most once a frame. */
const frame = () =>
  act(
    () =>
      new Promise<void>((resolve) => {
        requestAnimationFrame(() => {
          resolve();
        });
      }),
  );

beforeEach(async () => {
  scrolls.length = 0;
  Object.defineProperty(Element.prototype, "scrollIntoView", {
    configurable: true,
    writable: true,
    value(this: Element, options?: ScrollIntoViewOptions | boolean) {
      scrolls.push({ entry: this.getAttribute("aria-posinset"), options });
    },
  });
  // A move of the test before is a frame behind.
  await frame();
});

afterEach(() => {
  if (scrollIntoView) Object.defineProperty(Element.prototype, "scrollIntoView", scrollIntoView);
  else Reflect.deleteProperty(Element.prototype, "scrollIntoView");
  window.matchMedia = matchMedia;
});

/** The reader asked for reduced motion. */
function reduceMotion() {
  window.matchMedia = (query: string) => {
    const list = matchMedia(query);
    Object.defineProperty(list, "matches", { value: query.includes("prefers-reduced-motion") });
    return list;
  };
}

/** A timeline of `count` posts, each with a reply button, and a button of the page on either side of it. */
function timeline({
  count,
  inside,
  ...props
}: Partial<FeedProps> & { count: number; inside?: (position: number) => ReactNode }) {
  return render(
    <>
      <Button>Before</Button>
      <Feed label="Home" count={count} more={false} busy={false} {...props}>
        {Array.from({ length: count }, (_, i) => (
          <FeedItem key={i} id={`p${String(i + 1)}`} position={i + 1}>
            <Entry labelledBy={`name-${String(i + 1)}`}>
              <span id={`name-${String(i + 1)}`}>{`Post ${String(i + 1)}`}</span>
              <Button>{`Reply to post ${String(i + 1)}`}</Button>
              {inside?.(i + 1)}
            </Entry>
          </FeedItem>
        ))}
      </Feed>
      <Button>After</Button>
    </>,
  );
}

const entry = (position: number) => screen.getByRole("article", { name: `Post ${String(position)}` });

/** Moves the focus as a shortcut does, and says whether the feed took the press. */
function move(from: Element | null, step: 1 | -1, options?: { repeat: boolean }) {
  let took = false;
  act(() => {
    took = moveFeedFocus(from, step, options);
  });
  return took;
}

/** The keys the page saw go down, and whether the feed had taken each from it. */
function keysOfThePage() {
  const keys: [key: string, taken: boolean][] = [];
  const listener = (event: KeyboardEvent) => keys.push([event.key, event.defaultPrevented]);
  document.addEventListener("keydown", listener);
  return {
    keys,
    stop: () => {
      document.removeEventListener("keydown", listener);
    },
  };
}

describe("a feed", () => {
  test("with entries it is a feed named by its label, busy while entries are added", () => {
    const { rerender } = render(
      <Feed label="Home" count={1} more busy>
        <FeedItem id="p1" position={1}>
          <Entry>Post 1</Entry>
        </FeedItem>
      </Feed>,
    );
    expect(screen.getByRole("feed", { name: "Home", busy: true })).not.toBeNull();

    rerender(
      <Feed label="Home" count={1} more busy={false}>
        <FeedItem id="p1" position={1}>
          <Entry>Post 1</Entry>
        </FeedItem>
      </Feed>,
    );
    expect(screen.getByRole("feed", { name: "Home", busy: false })).not.toBeNull();
  });

  test("with no entries it is no feed, busy while it loads, and its notice follows it as a status", () => {
    const { rerender } = render(
      <Feed label="Home" count={0} more busy end={<FeedNotice>Loading…</FeedNotice>}>
        {null}
      </Feed>,
    );
    expect(screen.queryByRole("feed")).toBeNull();
    // What holds no entry has no role to find it by: its state is on whatever says it is busy.
    expect(document.querySelectorAll('[aria-busy="true"]')).toHaveLength(1);
    expect(screen.getAllByRole("status").map((status) => status.textContent)).toContain("Loading…");

    rerender(
      <Feed label="Home" count={0} more={false} busy={false} end={<FeedNotice>No posts yet.</FeedNotice>}>
        {null}
      </Feed>,
    );
    expect(document.querySelectorAll('[aria-busy="true"]')).toHaveLength(0);
    expect(screen.getAllByRole("status").map((status) => status.textContent)).toContain("No posts yet.");
  });

  test("each entry states its position, and the feed's size once nothing more follows", () => {
    const position = (article: HTMLElement) => [
      article.getAttribute("aria-posinset"),
      article.getAttribute("aria-setsize"),
    ];
    const { unmount } = timeline({ count: 2, more: false });
    expect(screen.getAllByRole("article").map(position)).toEqual([
      ["1", "2"],
      ["2", "2"],
    ]);
    unmount();

    // While more may follow, the size is unknown.
    timeline({ count: 2, more: true });
    expect(screen.getAllByRole("article").map(position)).toEqual([
      ["1", "-1"],
      ["2", "-1"],
    ]);
  });

  test("what is inside an item learns its position, the feed's size and its key, and nothing outside a feed", () => {
    const seen: (FeedPosition | undefined)[] = [];
    const Probe = () => {
      seen.push(useFeedPosition());
      return null;
    };
    render(
      <>
        <Feed label="Home" count={2} more={false} busy={false}>
          <FeedItem id="post-b" position={2}>
            <Probe />
          </FeedItem>
        </Feed>
        <Probe />
      </>,
    );

    expect(seen).toEqual([{ posinset: 2, setsize: 2, key: "post-b" }, undefined]);
  });

  test("the entries are stops of Tab, each before what is inside it", async () => {
    const user = userEvent.setup();
    timeline({ count: 2 });

    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Before" }));
    await user.tab();
    expect(document.activeElement).toBe(entry(1));
    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Reply to post 1" }));
    await user.tab();
    expect(document.activeElement).toBe(entry(2));
  });

  test("the new entries' status is there before it tells anything, and its button only while it does", async () => {
    const user = userEvent.setup();
    let presses = 0;
    const { rerender } = render(
      <Feed label="Home" count={0} more={false} busy={false}>
        {null}
      </Feed>,
    );
    const status = screen.getByRole("status");
    expect(status.textContent).toBe("");

    rerender(
      <Feed
        label="Home"
        count={0}
        more={false}
        busy={false}
        newItems={{ label: "3 new posts", onPress: () => presses++ }}
      >
        {null}
      </Feed>,
    );
    // The same region tells it, so that assistive technology announces it.
    expect(screen.getByRole("status")).toBe(status);
    await user.click(within(status).getByRole("button", { name: "3 new posts" }));
    expect(presses).toBe(1);
  });

  test("the place chosen again shows its newest as a press of the new posts' button does", () => {
    let presses = 0;
    timeline({ count: 2, newItems: { label: "3 new posts", onPress: () => presses++ } });
    act(() => {
      showNewest(document);
    });
    expect(presses).toBe(1);
  });

  test("it tells the key of the entry the reader clicks, however deep inside it", async () => {
    const user = userEvent.setup();
    const clicked: string[] = [];
    timeline({
      count: 2,
      onEntryClick: (key) => clicked.push(key),
      inside: (position) => (
        <p>
          The words of <em>{`post ${String(position)}`}</em>
        </p>
      ),
    });

    await user.click(screen.getByText("Post 2"));
    await user.click(screen.getByText("post 1"));
    await user.click(screen.getByRole("button", { name: "After" }));

    expect(clicked).toEqual(["p2", "p1"]);
  });

  test("it does not tell of a press that a control inside an entry takes", async () => {
    const user = userEvent.setup();
    const clicked: string[] = [];
    const pressed: string[] = [];
    timeline({
      count: 2,
      onEntryClick: (key) => clicked.push(key),
      inside: (position) => (
        <>
          <Button onPress={() => pressed.push(`open ${String(position)}`)}>{`Open post ${String(position)}`}</Button>
          <IconButton
            icon="favourite"
            label={`Favourite post ${String(position)}`}
            onPress={() => pressed.push(`favourite ${String(position)}`)}
          />
        </>
      ),
    });

    await user.click(screen.getByText("Post 1"));
    expect(clicked).toEqual(["p1"]);
    await user.click(screen.getByRole("button", { name: "Open post 1" }));
    await user.click(screen.getByRole("button", { name: "Favourite post 2" }));
    await user.keyboard("{Enter}");

    // The press is the control's alone.
    expect(pressed).toEqual(["open 1", "favourite 2", "favourite 2"]);
    expect(clicked).toEqual(["p1"]);
  });

  test("it tells when the reader comes into it and leaves it, by the focus or a selection", async () => {
    const user = userEvent.setup();
    const inside: boolean[] = [];
    timeline({ count: 2, onReaderChange: (reader) => inside.push(reader) });

    await user.tab();
    expect(inside).toEqual([]);
    await user.tab();
    await user.tab();
    expect(inside).toEqual([true]);
    await user.click(screen.getByRole("button", { name: "After" }));
    expect(inside).toEqual([true, false]);

    act(() => {
      document.getSelection()?.selectAllChildren(screen.getByText("Post 1"));
      document.dispatchEvent(new Event("selectionchange"));
    });
    expect(inside).toEqual([true, false, true]);
  });
});

describe("a feed's notice", () => {
  test("it is a status, or an alert when it tells a failure", () => {
    render(
      <>
        <FeedNotice>Loading…</FeedNotice>
        <FeedNotice alert>Could not load the timeline.</FeedNotice>
      </>,
    );

    expect(screen.getByRole("status").textContent).toBe("Loading…");
    expect(screen.getByRole("alert").textContent).toBe("Could not load the timeline.");
  });

  test("its action follows words after a middle dot, and a full sentence after its stop alone", async () => {
    const user = userEvent.setup();
    let tries = 0;
    render(
      <>
        <FeedNotice alert action={<TextButton onPress={() => tries++}>Try again</TextButton>}>
          Failed
        </FeedNotice>
        <FeedNotice start action={<TextButton>Sign in</TextButton>}>
          Sign in to see this timeline.
        </FeedNotice>
      </>,
    );

    expect(screen.getByRole("alert").textContent).toBe("Failed · Try again");
    expect(screen.getByRole("status").textContent).toBe("Sign in to see this timeline. Sign in");
    await user.click(within(screen.getByRole("alert")).getByRole("button", { name: "Try again" }));
    expect(tries).toBe(1);
  });
});

describe("moving between entries", () => {
  test("Page Down and Page Up move the focus to the next and the previous entry, from inside an entry too", async () => {
    const user = userEvent.setup();
    const page = keysOfThePage();
    timeline({ count: 3 });
    await user.tab();
    await user.tab();
    expect(document.activeElement).toBe(entry(1));

    await user.keyboard("{PageDown}");
    expect(document.activeElement).toBe(entry(2));
    await user.keyboard("{PageUp}");
    expect(document.activeElement).toBe(entry(1));
    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Reply to post 1" }));
    await user.keyboard("{PageDown}");
    expect(document.activeElement).toBe(entry(2));

    page.stop();
    // The feed took each of them, so the page does not scroll by a view as well.
    expect(page.keys.filter(([key]) => key.startsWith("Page"))).toEqual([
      ["PageDown", true],
      ["PageUp", true],
      ["PageDown", true],
    ]);
  });

  test("they stop at either end, and leave the key to the page there", async () => {
    const user = userEvent.setup();
    const page = keysOfThePage();
    timeline({ count: 2 });
    await user.tab();
    await user.tab();

    await user.keyboard("{PageUp}");
    expect(document.activeElement).toBe(entry(1));
    await user.keyboard("{PageDown}");
    expect(document.activeElement).toBe(entry(2));
    await user.keyboard("{PageDown}");
    expect(document.activeElement).toBe(entry(2));

    page.stop();
    expect(page.keys.filter(([key]) => key.startsWith("Page"))).toEqual([
      ["PageUp", false],
      ["PageDown", true],
      ["PageDown", false],
    ]);
  });

  test("outside the feed they are the page's", async () => {
    const user = userEvent.setup();
    const page = keysOfThePage();
    timeline({ count: 2 });
    const before = screen.getByRole("button", { name: "Before" });
    const after = screen.getByRole("button", { name: "After" });

    await user.tab();
    expect(document.activeElement).toBe(before);
    await user.keyboard("{PageDown}");
    expect(document.activeElement).toBe(before);
    await user.click(after);
    await user.keyboard("{PageUp}");
    expect(document.activeElement).toBe(after);

    page.stop();
    expect(page.keys.filter(([, taken]) => taken)).toEqual([]);
  });

  test("they leave a field, a modifier and other keys alone", async () => {
    const user = userEvent.setup();
    const page = keysOfThePage();
    timeline({
      count: 2,
      inside: (position) => (
        <>
          <input aria-label={`Alternative text ${String(position)}`} />
          <textarea aria-label={`Reply ${String(position)}`} />
        </>
      ),
    });
    await user.tab();
    await user.tab();

    await user.keyboard("{Control>}{PageDown}{/Control}");
    await user.keyboard("{Shift>}{PageDown}{/Shift}");
    await user.keyboard("{Alt>}{PageDown}{/Alt}");
    await user.keyboard("{Meta>}{PageDown}{/Meta}");
    await user.keyboard("{Control>}{PageUp}{/Control}");
    await user.keyboard("{Shift>}{PageUp}{/Shift}");
    await user.keyboard("{ArrowDown}{ArrowUp}j");
    expect(document.activeElement).toBe(entry(1));
    await user.click(screen.getByRole("textbox", { name: "Alternative text 1" }));
    await user.keyboard("{PageDown}");
    expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Alternative text 1" }));
    await user.click(screen.getByRole("textbox", { name: "Reply 1" }));
    await user.keyboard("{PageDown}");
    expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Reply 1" }));

    page.stop();
    expect(page.keys.filter(([, taken]) => taken)).toEqual([]);
  });

  test("moveFeedFocus, which a shortcut binds, moves the same way and says whether it took the press", async () => {
    timeline({ count: 3 });
    expect(move(entry(1), 1)).toBe(true);
    expect(document.activeElement).toBe(entry(2));
    await frame();
    expect(move(screen.getByRole("button", { name: "Reply to post 2" }), -1)).toBe(true);
    expect(document.activeElement).toBe(entry(1));
    await frame();
    // Not past either end, nor outside a feed.
    expect(move(entry(1), -1)).toBe(false);
    expect(move(entry(3), 1)).toBe(false);
    expect(move(screen.getByRole("button", { name: "After" }), 1)).toBe(false);
    expect(move(null, 1)).toBe(false);
    expect(document.activeElement).toBe(entry(1));
  });

  test("moveFeedFocus from the feed itself enters at the first entry, or at the last going back", async () => {
    timeline({ count: 3 });
    const feed = screen.getByRole("feed", { name: "Home" });

    expect(move(feed, 1)).toBe(true);
    expect(document.activeElement).toBe(entry(1));
    await frame();
    expect(move(feed, -1)).toBe(true);
    expect(document.activeElement).toBe(entry(3));
  });

  test("it moves once a frame: another press in the same frame is taken and moves nothing", async () => {
    timeline({ count: 3 });
    const held = (from: Element | null) => move(from, 1, { repeat: true });

    expect(held(entry(1))).toBe(true);
    expect(document.activeElement).toBe(entry(2));
    expect(held(entry(2))).toBe(true);
    expect(document.activeElement).toBe(entry(2));
    await frame();
    expect(held(entry(2))).toBe(true);
    expect(document.activeElement).toBe(entry(3));
  });
});

describe("bringing an entry to the top", () => {
  /** What the entry at `position` was last asked to scroll with. */
  const asked = (position: number) => scrolls.findLast((scroll) => scroll.entry === String(position))?.options;

  test("a move asks the browser to glide the entry to the top of the view", async () => {
    const user = userEvent.setup();
    timeline({ count: 2 });
    await user.tab();
    await user.tab();
    scrolls.length = 0;

    await user.keyboard("{PageDown}");

    // block "start" keeps the entry's scroll margin and the scroller's scroll padding (a sticky header's).
    expect(asked(2)).toEqual({ block: "start", inline: "nearest", behavior: "smooth" });
    expect(scrolls.every((scroll) => scroll.entry === "2")).toBe(true);
  });

  // Whether the keyboard or a click gave the focus is the browser's to say: here `:focus-visible` is true of whatever
  // has the focus, so that an entry a click focused stays put is for a test in a browser.
  test("an entry Tab comes to is asked to glide to the top, and a part inside an entry is the browser's to show", async () => {
    const user = userEvent.setup();
    timeline({ count: 2 });
    await user.tab();
    expect(scrolls).toEqual([]);

    await user.tab();
    expect(document.activeElement).toBe(entry(1));
    expect(asked(1)).toEqual({ block: "start", inline: "nearest", behavior: "smooth" });
    expect(scrolls.every((scroll) => scroll.entry === "1")).toBe(true);

    scrolls.length = 0;
    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Reply to post 1" }));
    expect(scrolls).toEqual([]);
  });

  test("with reduced motion it asks for a jump", () => {
    reduceMotion();
    timeline({ count: 2 });

    move(entry(2), -1);

    expect(asked(1)).toEqual({ block: "start", inline: "nearest", behavior: "instant" });
    expect(scrolls.every((scroll) => typeof scroll.options === "object" && scroll.options.behavior === "instant")).toBe(
      true,
    );
  });

  test("while the key is held it asks for a jump, since each glide would cancel the one before", () => {
    timeline({ count: 2 });

    move(entry(1), 1, { repeat: true });

    expect(asked(2)).toEqual({ block: "start", inline: "nearest", behavior: "instant" });
  });

  test("a held Page Down asks for a jump too", async () => {
    const user = userEvent.setup();
    timeline({ count: 3 });
    await user.tab();
    await user.tab();

    await user.keyboard("{PageDown>2/}");

    expect(asked(3)).toEqual({ block: "start", inline: "nearest", behavior: "instant" });
  });
});
