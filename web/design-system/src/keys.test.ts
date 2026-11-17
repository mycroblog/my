// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { ariaKeys, display, isTextField, keymapProblems, Shortcuts, spelled, type KeyPress, type Place } from "./keys";

const keymap = {
  "shortcuts.help": { keys: ["?"] },
  "compose.open": { keys: ["n"] },
  "compose.send": { keys: ["Mod+Enter"], inFields: true },
  "post.reply": { keys: ["r"] },
  "post.favourite": { keys: ["f"] },
  "quote.reply": { keys: ["r"] },
  "timeline.next": { keys: ["j", "ArrowDown"] },
};

const press = (key: string, more: Partial<KeyPress> = {}): KeyPress => ({
  key,
  ctrlKey: false,
  metaKey: false,
  altKey: false,
  shiftKey: false,
  ...more,
});

type El = { name: string };
const post: El = { name: "post" };
const quote: El = { name: "quote" };
const button: El = { name: "button" };
const body: El = { name: "body" };
const at = (path: El[], more: Partial<Place<El>> = {}): Place<El> => ({
  path,
  inField: false,
  overlaid: false,
  ...more,
});

/** The keymap's shortcuts, with a log of what ran. */
function setup(mac = false) {
  const s = new Shortcuts<El>(keymap, mac);
  const ran: string[] = [];
  const declare = (
    id: string,
    scope: "screen" | "focus",
    owner: El | null = null,
    more: { group?: string; groupLabel?: string } = {},
  ) =>
    s.declare({
      id,
      label: id,
      scope,
      owner: () => owner,
      run: () => {
        ran.push(`${id}@${owner?.name ?? "screen"}`);
      },
      ...more,
    });
  const fire = (e: KeyPress, place: Place<El>) => s.press(e, place);
  return { s, ran, declare, fire };
}

describe("scopes", () => {
  test("a screen action acts anywhere; a focus action only while focus is inside its element", () => {
    const { ran, declare, fire } = setup();
    declare("compose.open", "screen");
    declare("post.favourite", "focus", post);
    fire(press("n"), at([body]));
    fire(press("f"), at([body]));
    fire(press("f"), at([button, post, body]));
    expect(ran).toEqual(["compose.open@screen", "post.favourite@post"]);
  });

  test("the focused element's action comes before its ancestor's, and both before the screen's", () => {
    const { ran, declare, fire } = setup();
    declare("post.reply", "focus", post);
    declare("quote.reply", "focus", quote);
    fire(press("r"), at([button, quote, post, body]));
    fire(press("r"), at([post, body]));
    expect(ran).toEqual(["quote.reply@quote", "post.reply@post"]);
  });

  test("under a layer over the screen, the screen's actions wait", () => {
    const { ran, declare, fire } = setup();
    declare("compose.open", "screen");
    fire(press("n"), at([button], { overlaid: true }));
    expect(ran).toEqual([]);
  });

  test("an action taken back no longer acts, and a key no action has does nothing", () => {
    const { ran, declare, fire } = setup();
    const undo = declare("compose.open", "screen");
    undo();
    fire(press("n"), at([body]));
    fire(press("x"), at([body]));
    expect(ran).toEqual([]);
  });

  test("an action declared twice acts once, by its latest declaration", () => {
    const { ran, declare, fire } = setup();
    declare("compose.open", "screen");
    declare("compose.open", "screen");
    fire(press("n"), at([body]));
    expect(ran).toEqual(["compose.open@screen"]);
  });
});

describe("conflicts", () => {
  test("two actions on one key in one scope are refused", () => {
    const { declare } = setup();
    declare("post.reply", "focus", post);
    expect(() => declare("quote.reply", "focus", post)).toThrow(/post\.reply.*share r/);
  });

  test("two actions on one key on different elements, or in different scopes, are no conflict", () => {
    const { declare } = setup();
    declare("post.reply", "focus", post);
    expect(() => declare("quote.reply", "focus", quote)).not.toThrow();
    expect(() => declare("quote.reply", "screen")).not.toThrow();
  });

  test("a chord the browser keeps for itself is refused, with a message", () => {
    expect(keymapProblems({ "compose.open": { keys: ["Mod+N"] } })).toEqual([
      "compose.open: the browser keeps ⌘N for itself and never passes it to the page; choose another key",
    ]);
    expect(keymapProblems({ "tab.close": { keys: ["Ctrl+w"] } })[0]).toContain("Ctrl+w");
    expect(keymapProblems({ "tab.next": { keys: ["Ctrl+Tab"] } })).toHaveLength(1);
    expect(() => new Shortcuts({ "compose.open": { keys: ["Meta+Shift+T"] } }, false)).toThrow(/keeps/);
  });

  test("a chord that does not parse is refused", () => {
    expect(keymapProblems({ a: { keys: ["Hyper+x"] } })[0]).toContain("Hyper is no modifier");
  });

  test("the default keys pass", () => {
    expect(keymapProblems(keymap)).toEqual([]);
  });
});

describe("chords", () => {
  test("a letter matches in either case without Shift; a printable key whatever Shift made it", () => {
    const { ran, declare, fire } = setup();
    declare("compose.open", "screen");
    declare("shortcuts.help", "screen");
    fire(press("N"), at([body]));
    fire(press("n", { shiftKey: true }), at([body]));
    fire(press("n", { ctrlKey: true }), at([body]));
    fire(press("?", { shiftKey: true }), at([body]));
    expect(ran).toEqual(["compose.open@screen", "shortcuts.help@screen"]);
  });

  test("Mod is ⌘ on macOS and Ctrl elsewhere", () => {
    for (const mac of [true, false]) {
      const { ran, declare, fire } = setup(mac);
      declare("compose.send", "screen");
      fire(press("Enter", { metaKey: true }), at([body]));
      fire(press("Enter", { ctrlKey: true }), at([body]));
      expect(ran).toHaveLength(1);
    }
  });
});

describe("input", () => {
  test("no action acts while an input method composes", () => {
    const { ran, declare, fire } = setup();
    declare("compose.open", "screen");
    fire(press("n", { isComposing: true }), at([body]));
    fire(press("Process", { keyCode: 229 }), at([body]));
    expect(ran).toEqual([]);
  });

  test("in a text field only the keys the keymap allows there act", () => {
    const { ran, declare, fire } = setup();
    declare("compose.open", "screen");
    declare("compose.send", "screen");
    fire(press("n"), at([body], { inField: true }));
    fire(press("Enter", { ctrlKey: true }), at([body], { inField: true }));
    expect(ran).toEqual(["compose.send@screen"]);
  });

  test("a text field is what typing goes into", () => {
    const el = (tagName: string, more: { type?: string; isContentEditable?: boolean; role?: string } = {}) => ({
      tagName,
      ...more,
      getAttribute: (name: string) => (name === "role" ? (more.role ?? null) : null),
    });
    expect(isTextField(el("INPUT"))).toBe(true);
    expect(isTextField(el("INPUT", { type: "search" }))).toBe(true);
    expect(isTextField(el("TEXTAREA"))).toBe(true);
    expect(isTextField(el("SELECT"))).toBe(true);
    expect(isTextField(el("DIV", { isContentEditable: true }))).toBe(true);
    expect(isTextField(el("DIV", { role: "textbox" }))).toBe(true);
    expect(isTextField(el("INPUT", { type: "checkbox" }))).toBe(false);
    expect(isTextField(el("BUTTON"))).toBe(false);
    expect(isTextField(el("ARTICLE"))).toBe(false);
  });
});

describe("names", () => {
  test("aria-keyshortcuts names each chord, Mod for the platform", () => {
    expect(ariaKeys(["j", "ArrowDown"], false)).toBe("j ArrowDown");
    expect(ariaKeys(["Mod+Enter"], true)).toBe("Meta+Enter");
    expect(ariaKeys(["Mod+Shift+ "], false)).toBe("Control+Shift+Space");
    const { s } = setup();
    expect(s.aria("timeline.next")).toBe("j ArrowDown");
    expect(s.aria("unknown")).toBeUndefined();
  });

  test("the help names the keys as the reader's platform does", () => {
    expect(display("Mod+Shift+Enter", true)).toEqual(["⇧", "⌘", "Enter"]);
    expect(spelled("Mod+Enter", true)).toBe("⌘Enter");
    expect(spelled("Mod+Enter", false)).toBe("Ctrl+Enter");
    expect(spelled("ArrowDown", false)).toBe("↓");
  });
});

test("a press is taken by an action that acted, and goes on past one that did nothing", () => {
  const s = new Shortcuts<El>(keymap, false);
  let moved = true;
  s.declare({ id: "timeline.next", label: "Next", scope: "focus", owner: () => body, run: () => moved });
  expect(s.press(press("j"), at([post, body]))).toBe(true);
  moved = false;
  expect(s.press(press("ArrowDown"), at([post, body]))).toBe(false);
  expect(s.press(press("x"), at([post, body]))).toBe(false);
});

describe("the list", () => {
  const other: El = { name: "other post" };
  const feed: El = { name: "feed" };
  /** A screen with two posts in a feed: each post declares its actions, the feed the next post. */
  function screen() {
    const t = setup();
    t.declare("shortcuts.help", "screen");
    t.declare("compose.open", "screen");
    t.declare("unbound", "screen");
    for (const el of [post, other]) {
      t.declare("post.favourite", "focus", el, { groupLabel: "When a post is focused" });
      t.declare("post.reply", "focus", el, { groupLabel: "When a post is focused" });
    }
    t.declare("timeline.next", "focus", feed, { groupLabel: "When the timeline is focused" });
    t.declare("quote.reply", "focus", quote, { group: "post" });
    return t.s;
  }
  const ids = (l: { id: string }[]) => l.map((a) => a.id);
  const outline = (groups: { id: string; label: string; current: boolean; actions: { id: string }[] }[]) =>
    groups.map((g) => [g.id, g.label, g.current, ids(g.actions)]);

  test("the screen's actions, then a group per namespace, each action once in the keymap's order", () => {
    const { screen: actions, groups } = screen().list([body]);
    expect(ids(actions)).toEqual(["shortcuts.help", "compose.open"]);
    expect(outline(groups)).toEqual([
      ["post", "When a post is focused", false, ["post.reply", "post.favourite", "quote.reply"]],
      ["timeline", "When the timeline is focused", false, ["timeline.next"]],
    ]);
  });

  test("the groups focus is in come first, the nearest first, marked current", () => {
    expect(outline(screen().list([feed, body]).groups).map((g) => [g[0], g[2]])).toEqual([
      ["timeline", true],
      ["post", false],
    ]);
    expect(outline(screen().list([button, post, feed, body]).groups).map((g) => [g[0], g[2]])).toEqual([
      ["post", true],
      ["timeline", true],
    ]);
  });

  test("a group with no declaration on the screen is left out", () => {
    const t = setup();
    t.declare("compose.open", "screen");
    expect(t.s.list([body]).groups).toEqual([]);
  });
});
