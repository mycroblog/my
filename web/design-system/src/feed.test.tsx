// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { Entry } from "./entry";
import { Feed, FeedItem, FeedNotice, moveFeedFocus, onFeedFocus, onFeedKeyDown } from "./feed";

const feed = (props: { count: number; more: boolean; busy: boolean }) =>
  renderToStaticMarkup(
    <Feed label="Home" {...props} end={<FeedNotice>Loading</FeedNotice>}>
      {Array.from({ length: props.count }, (_, i) => (
        <FeedItem key={i} id={`p${i + 1}`} position={i + 1}>
          <Entry>{`post ${i + 1}`}</Entry>
        </FeedItem>
      ))}
    </Feed>,
  );

describe("the feed's markup", () => {
  test("with no entries it is no feed, busy while it loads, its notice after it", () => {
    const html = feed({ count: 0, more: true, busy: true });
    expect(html).toMatch(/<div aria-busy="true" class="[^"]*gap-feed[^"]*"><\/div>/);
    expect(html).not.toContain('role="feed"');
    expect(html).toMatch(/<\/div><p role="status" [^>]*>Loading<\/p>/);
    expect(feed({ count: 0, more: false, busy: false })).toContain('aria-busy="false"');
  });

  test("with entries it is a named feed", () => {
    expect(feed({ count: 1, more: true, busy: true })).toMatch(/<div role="feed" aria-label="Home" aria-busy="true"/);
  });

  test("each entry states its position, and the feed's size once nothing more follows", () => {
    const known = feed({ count: 2, more: false, busy: false });
    expect(known).toContain('aria-posinset="1" aria-setsize="2"');
    expect(known).toContain('aria-posinset="2" aria-setsize="2"');
    // While more may follow, the size is unknown.
    expect(feed({ count: 2, more: true, busy: false })).toContain('aria-posinset="2" aria-setsize="-1"');
  });

  test("each entry carries its key, by which a screen restores its position", () => {
    expect(feed({ count: 2, more: false, busy: false })).toContain('data-scroll-key="p2"');
  });

  test("the browser's anchoring is off, since the feed anchors itself", () => {
    expect(feed({ count: 1, more: false, busy: false })).toContain("[overflow-anchor:none]");
  });

  test("the new entries' status is there before it tells anything, and its button only while it does", () => {
    const empty = feed({ count: 1, more: false, busy: false });
    expect(empty).toMatch(/^<div [^>]*><div role="status" class="[^"]*sticky[^"]*"><\/div>/);
    const told = renderToStaticMarkup(
      <Feed label="Home" count={0} more={false} busy={false} newItems={{ label: "3 new posts", onPress: () => {} }}>
        {null}
      </Feed>,
    );
    expect(told).toMatch(/<div role="status" [^>]*><span [^>]*><button [^>]*>.*3 new posts.*<\/button><\/span><\/div>/);
    // It stands off what scrolls under it, and takes a full touch target's height.
    expect(told).toMatch(/<span class="[^"]*shadow-overlay[^"]*"><button [^>]*class="[^"]*h-icon-button-touch/);
  });

  test("an entry outside a feed states no position", () => {
    expect(renderToStaticMarkup(<Entry>x</Entry>)).not.toContain("aria-posinset");
  });

  test("an entry brought to the top keeps the feed's room above it", () => {
    expect(feed({ count: 1, more: false, busy: false })).toContain("scroll-mt-feed-scroll");
  });

  test("a failure is an alert, with its action after a middle dot", () => {
    const html = renderToStaticMarkup(
      <FeedNotice alert action={<button type="button">Try again</button>}>
        Failed
      </FeedNotice>,
    );
    expect(html).toMatch(
      /^<p role="alert" [^>]*>Failed <span [^>]*>· <button type="button">Try again<\/button><\/span><\/p>$/,
    );
  });
});

/** A small stand-in for the DOM of a feed: the parts of Element the feed's functions read. */
class Node {
  parentElement: Node | null = null;
  readonly children: Node[] = [];
  readonly scrolls: ScrollIntoViewOptions[] = [];
  focusedWith: FocusOptions | undefined;
  focusVisible = false;
  readonly kind: "feed" | "entry" | "field" | "button" | "other";
  readonly env: { reduced: boolean; focused?: Node; frames?: (() => void)[] };
  constructor(kind: Node["kind"], env: Node["env"]) {
    this.kind = kind;
    this.env = env;
  }
  add(...nodes: Node[]): this {
    for (const node of nodes) {
      node.parentElement = this;
      this.children.push(node);
    }
    return this;
  }
  get ownerDocument() {
    const { env } = this;
    const frames = env.frames;
    return {
      defaultView: {
        matchMedia: () => ({ matches: env.reduced }),
        // Frames only when the test draws them by hand.
        requestAnimationFrame: frames && ((f: () => void) => frames.push(f)),
      },
    };
  }
  matches(selector: string): boolean {
    if (selector === '[role="feed"]') return this.kind === "feed";
    if (selector === "[data-entry]") return this.kind === "entry";
    if (selector === ":focus-visible") return this.focusVisible;
    if (selector.startsWith("input")) return this.kind === "field";
    throw new Error(`the fake does not know ${selector}`);
  }
  closest(selector: string): Node | null {
    return this.matches(selector) ? this : (this.parentElement?.closest(selector) ?? null);
  }
  querySelectorAll(selector: string): Node[] {
    return this.children.flatMap((c) => [...(c.matches(selector) ? [c] : []), ...c.querySelectorAll(selector)]);
  }
  focus(options?: FocusOptions) {
    this.focusedWith = options;
    this.env.focused = this;
  }
  scrollIntoView(options: ScrollIntoViewOptions) {
    this.scrolls.push(options);
  }
}

function sample(reduced = false) {
  const env: Node["env"] = { reduced };
  const button = new Node("button", env);
  const field = new Node("field", env);
  const first = new Node("entry", env).add(button, field);
  const second = new Node("entry", env);
  const third = new Node("entry", env);
  const root = new Node("feed", env).add(first, second, third);
  return { env, root, first, second, third, button, field };
}

const el = (n: Node) => n as unknown as Element;

const key = (
  target: Element,
  k: string,
  mods: Partial<Record<"altKey" | "ctrlKey" | "metaKey" | "shiftKey", boolean>> = {},
) => {
  let prevented = false;
  onFeedKeyDown({
    key: k,
    altKey: false,
    ctrlKey: false,
    metaKey: false,
    shiftKey: false,
    ...mods,
    target,
    defaultPrevented: false,
    preventDefault: () => {
      prevented = true;
    },
  });
  return prevented;
};

describe("moving between entries", () => {
  test("Page Down and Page Up move the focus to the next and the previous entry, from inside an entry too", () => {
    const { env, first, second, button } = sample();
    expect(key(el(first), "PageDown")).toBe(true);
    expect(env.focused).toBe(second);
    expect(key(el(second), "PageUp")).toBe(true);
    expect(env.focused).toBe(first);
    expect(key(el(button), "PageDown")).toBe(true);
    expect(env.focused).toBe(second);
  });

  test("they stop at either end, and leave the key to the page there", () => {
    const { env, first, third } = sample();
    expect(key(el(first), "PageUp")).toBe(false);
    expect(key(el(third), "PageDown")).toBe(false);
    expect(env.focused).toBeUndefined();
  });

  test("they leave a field, a modifier and other keys alone", () => {
    const { env, first, field } = sample();
    expect(key(el(field), "PageDown")).toBe(false);
    expect(key(el(first), "PageDown", { ctrlKey: true })).toBe(false);
    expect(key(el(first), "PageDown", { shiftKey: true })).toBe(false);
    expect(key(el(first), "ArrowDown")).toBe(false);
    expect(key(el(first), "j")).toBe(false);
    expect(env.focused).toBeUndefined();
  });

  test("moveFeedFocus, which a shortcut binds, moves the same way and enters from the feed itself", () => {
    const { env, root, first, third } = sample();
    expect(moveFeedFocus(el(root), 1)).toBe(true);
    expect(env.focused).toBe(first);
    expect(moveFeedFocus(el(root), -1)).toBe(true);
    expect(env.focused).toBe(third);
    expect(moveFeedFocus(null, 1)).toBe(false);
    expect(moveFeedFocus(el(new Node("other", env)), 1)).toBe(false);
  });
});

describe("scrolling an entry to the top", () => {
  test("a move focuses the entry without the browser's own scroll, and glides it to the top", () => {
    const { first, second } = sample();
    moveFeedFocus(el(first), 1);
    expect(second.focusedWith).toEqual({ preventScroll: true });
    // block "start" keeps the entry's scroll margin and the scroller's scroll padding (a sticky header's).
    expect(second.scrolls).toEqual([{ block: "start", inline: "nearest", behavior: "smooth" }]);
  });

  test("with reduced motion it jumps", () => {
    const { first, second } = sample(true);
    moveFeedFocus(el(second), -1);
    expect(first.scrolls).toEqual([{ block: "start", inline: "nearest", behavior: "instant" }]);
  });

  test("while the key is held it jumps, and it moves once a frame", () => {
    const { env, first, second, third } = sample();
    const frames: (() => void)[] = [];
    env.frames = frames;
    expect(moveFeedFocus(el(first), 1, { repeat: true })).toBe(true);
    expect(second.scrolls).toEqual([{ block: "start", inline: "nearest", behavior: "instant" }]);
    // Another repeat in the same frame is taken, so the page does not scroll, but moves nothing.
    expect(moveFeedFocus(el(second), 1, { repeat: true })).toBe(true);
    expect(env.focused).toBe(second);
    for (const draw of frames.splice(0)) draw();
    expect(moveFeedFocus(el(second), 1, { repeat: true })).toBe(true);
    expect(env.focused).toBe(third);
    // A held Page Down jumps too.
    for (const draw of frames.splice(0)) draw();
    onFeedKeyDown({
      key: "PageUp",
      repeat: true,
      altKey: false,
      ctrlKey: false,
      metaKey: false,
      shiftKey: false,
      target: el(third),
      defaultPrevented: false,
      preventDefault: () => undefined,
    });
    expect(second.scrolls.at(-1)?.behavior).toBe("instant");
  });

  test("an entry the keyboard focuses scrolls to the top; one a click focuses stays put", () => {
    const { root, second, third, button } = sample();
    second.focusVisible = true;
    onFeedFocus({ target: el(second), currentTarget: el(root) });
    expect(second.scrolls).toHaveLength(1);
    onFeedFocus({ target: el(third), currentTarget: el(root) });
    expect(third.scrolls).toHaveLength(0);
    // Focus on a part inside an entry is the browser's to show.
    button.focusVisible = true;
    onFeedFocus({ target: el(button), currentTarget: el(root) });
    expect(button.scrolls).toHaveLength(0);
  });
});
