// SPDX-License-Identifier: AGPL-3.0-only

// The keyboard shortcuts on a page: which press runs which declared action, what the controls say of their keys, and
// the help that lists them. Which action a press finds among the declared ones is keys.test.ts's.

import { expect, test } from "bun:test";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef, type ReactNode } from "react";
import { I18nProvider } from "react-aria-components";
import { IconButton } from "./icon-button";
import type { ListedGroup } from "./keys";
import { MenuButton, MenuGroup, MenuItem } from "./menu";
import { ActionScope, ShortcutList, ShortcutProvider, useAction } from "./shortcuts";

const keymap = {
  "shortcuts.help": { keys: ["?"] },
  "post.reply": { keys: ["r"] },
  "compose.open": { keys: ["n", "Mod+Enter"] },
  "compose.send": { keys: ["Mod+s"], inFields: true },
  "timeline.next": { keys: ["j", "ArrowDown"] },
};

/** What ran, in order: the action, and for a post's the post. */
type Ran = string[];

function Reply({ post, ran }: { post: string; ran: Ran }) {
  const keys = useAction("post.reply", () => ran.push(`reply ${post}`), {
    scope: "focus",
    label: "Reply",
    groupLabel: "When a post is focused",
  });
  return <IconButton icon="reply" label={`Reply to ${post}`} {...keys} />;
}

/** A post that takes the focus, whose reply acts on it. */
function Post({ name, ran }: { name: string; ran: Ran }) {
  const ref = useRef<HTMLElement>(null);
  return (
    <article ref={ref} tabIndex={0} aria-label={name}>
      <ActionScope target={ref}>
        <Reply post={name} ran={ran} />
      </ActionScope>
    </article>
  );
}

/** The screen's menu button, whose keys open a new post, told whether the key is held. */
function More({ ran }: { ran: Ran }) {
  const keys = useAction("compose.open", ({ repeat }) => ran.push(repeat ? "compose, held" : "compose"), {
    scope: "screen",
    label: "New post",
  });
  return (
    <MenuButton icon="more" label="More" {...keys}>
      <MenuGroup>
        <MenuItem id="link" icon="link">
          Copy link
        </MenuItem>
      </MenuGroup>
    </MenuButton>
  );
}

/** The screen's actions with no control: send, which acts in a text field too, and next, which has nowhere to go. */
function Others({ ran }: { ran: Ran }) {
  useAction("compose.send", () => ran.push("send"), { scope: "screen", label: "Post" });
  useAction(
    "timeline.next",
    () => {
      ran.push("next, nowhere");
      return false;
    },
    { scope: "screen", label: "Go to next post" },
  );
  return null;
}

/** A screen of two posts, a menu button and a text field, and what ran on it. */
function screenOf({ mac = false, provider = true }: { mac?: boolean; provider?: boolean } = {}) {
  const ran: Ran = [];
  const parts: ReactNode = (
    <>
      <Post name="First post" ran={ran} />
      <Post name="Second post" ran={ran} />
      <More ran={ran} />
      <Others ran={ran} />
      <input aria-label="Search" />
    </>
  );
  render(
    provider ? (
      <ShortcutProvider keymap={keymap} mac={mac}>
        {parts}
      </ShortcutProvider>
    ) : (
      parts
    ),
  );
  return ran;
}

const shortcutsOf = (name: string) => screen.getByRole("button", { name }).getAttribute("aria-keyshortcuts");

test("a declared action puts its keys on its control, Mod named for the platform", () => {
  screenOf();

  expect(shortcutsOf("Reply to First post")).toBe("r");
  expect(shortcutsOf("More")).toBe("n Control+Enter");
});

test("on macOS Mod is Meta in what a control says of its keys", () => {
  screenOf({ mac: true });

  expect(shortcutsOf("More")).toBe("n Meta+Enter");
});

test("without a provider a part declares nothing and works on its own", async () => {
  const user = userEvent.setup();
  const ran = screenOf({ provider: false });

  expect(screen.getByRole("button", { name: "Reply to First post" }).hasAttribute("aria-keyshortcuts")).toBe(false);
  expect(screen.getByRole("button", { name: "More" }).hasAttribute("aria-keyshortcuts")).toBe(false);
  await user.tab();
  await user.keyboard("rn?");
  expect(ran).toEqual([]);
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("a screen action runs on each of its keys wherever the focus is, told whether the key is held", async () => {
  const user = userEvent.setup();
  const ran = screenOf();

  await user.keyboard("n");
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("article", { name: "First post" }));
  await user.keyboard("{Control>}{Enter}{/Control}");
  await user.keyboard("{n>3/}");

  expect(ran).toEqual(["compose", "compose", "compose", "compose, held", "compose, held"]);
});

test("a focus action runs only while the focus is inside what it acts on, and on that one", async () => {
  const user = userEvent.setup();
  const ran = screenOf();

  await user.keyboard("r");
  expect(ran).toEqual([]);
  await user.tab();
  await user.keyboard("r");
  // On the post's own reply button, inside the post.
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Reply to First post" }));
  await user.keyboard("r");
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("article", { name: "Second post" }));
  await user.keyboard("r");

  expect(ran).toEqual(["reply First post", "reply First post", "reply Second post"]);
});

test("in a text field a key types, and only an action the keymap allows there runs", async () => {
  const user = userEvent.setup();
  const ran = screenOf();
  const field = screen.getByRole<HTMLInputElement>("textbox", { name: "Search" });
  field.focus();

  await user.keyboard("rn?");
  await user.keyboard("{Control>}{Enter}{/Control}");
  expect(ran).toEqual([]);
  expect(field.value).toBe("rn?");
  expect(screen.queryByRole("dialog")).toBeNull();

  await user.keyboard("{Control>}s{/Control}");
  expect(ran).toEqual(["send"]);
});

test("an action that ran takes its key from the page; one that did nothing, and a free key, leave it", () => {
  const ran = screenOf();
  const post = screen.getByRole("article", { name: "First post" });
  post.focus();

  // fireEvent tells whether the page still gets the key: false once something took it.
  expect(fireEvent.keyDown(post, { key: "r" })).toBe(false);
  expect(fireEvent.keyDown(post, { key: "j" })).toBe(true);
  expect(fireEvent.keyDown(post, { key: "x" })).toBe(true);
  expect(ran).toEqual(["reply First post", "next, nowhere"]);
});

test("while a menu is open over the screen, the screen's actions wait", async () => {
  const user = userEvent.setup();
  const ran = screenOf();
  await user.click(screen.getByRole("button", { name: "More" }));
  await screen.findByRole("menu", { name: "More" });

  await user.keyboard("n");

  expect(ran).toEqual([]);
  expect(screen.queryByRole("menu", { name: "More" })).not.toBeNull();
});

/** A section of the help by its heading, and its actions as rows of a label and its keys. */
function section(help: HTMLElement, name: string | RegExp) {
  const region = within(help).getByRole("region", { name });
  const labels = within(region).getAllByRole("term");
  const keys = within(region).getAllByRole("definition");
  return labels.map((label, i) => [
    label.textContent,
    ...within(keys[i] ?? label)
      .getAllByText(/./)
      .map((key) => key.textContent),
  ]);
}

/** The headings of the help's sections, in order, each as its words then its mark, if it has one. */
const sections = (help: HTMLElement) =>
  within(help)
    .getAllByRole("heading", { level: 3 })
    .map((heading) =>
      [...heading.childNodes]
        .map((part) => (part.textContent ?? "").trim())
        .filter((part) => part !== "")
        .join(" | "),
    );

test("the help's key opens a dialog that lists the screen's actions, then those of what has the focus, marked current", async () => {
  const user = userEvent.setup();
  const ran = screenOf();
  await user.tab();
  await user.tab();
  await user.tab();
  const post = screen.getByRole("article", { name: "Second post" });
  expect(document.activeElement).toBe(post);

  await user.keyboard("?");

  const help = await screen.findByRole("dialog", { name: "Keyboard shortcuts" });
  expect(help.contains(document.activeElement)).toBe(true);
  expect(within(help).getByRole("heading", { level: 2, name: "Keyboard shortcuts" })).not.toBeNull();
  expect(section(help, "This screen")).toEqual([
    ["Show keyboard shortcuts", "?"],
    ["New post", "n", "Ctrl+Enter"],
    ["Post", "Ctrl+s"],
    ["Go to next post", "j", "↓"],
  ]);
  expect(section(help, /^When a post is focused\s*Current$/)).toEqual([["Reply", "r"]]);

  // Under the help the screen's and the post's actions wait; Escape closes it and gives the focus back.
  await user.keyboard("rn");
  expect(ran).toEqual([]);
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(document.activeElement).toBe(post);
});

test("with the focus on no post the help lists the posts' actions too, none marked current, and its close closes it", async () => {
  const user = userEvent.setup();
  screenOf();

  await user.keyboard("?");

  const help = await screen.findByRole("dialog", { name: "Keyboard shortcuts" });
  expect(sections(help)).toEqual(["This screen", "When a post is focused"]);
  expect(section(help, "When a post is focused")).toEqual([["Reply", "r"]]);
  await user.click(within(help).getByRole("button", { name: "Close" }));
  expect(screen.queryByRole("dialog")).toBeNull();
});

const listed = [
  { id: "shortcuts.help", label: "Show keyboard shortcuts", keys: ["?"] },
  { id: "compose.open", label: "New post", keys: ["n", "Mod+Enter"] },
];
const post: ListedGroup = {
  id: "post",
  label: "When a post is focused",
  current: true,
  actions: [{ id: "post.reply", label: "Reply", keys: ["r"] }],
};
const timeline: ListedGroup = {
  id: "timeline",
  label: "When the timeline is focused",
  current: false,
  actions: [{ id: "timeline.next", label: "Go to next post", keys: ["j", "ArrowDown"] }],
};

/** The help at rest, as the provider shows it, in a locale. */
const helpIn = (locale: string, groups: ListedGroup[], mac = false) => {
  render(
    <I18nProvider locale={locale}>
      <ShortcutList screen={listed} groups={groups} mac={mac} />
    </I18nProvider>,
  );
  return screen.getByRole("dialog");
};

test("the help lists the screen's actions first, then each group in the order given, the current one marked once", () => {
  const help = helpIn("en", [post, timeline]);

  expect(screen.getByRole("dialog", { name: "Keyboard shortcuts" })).toBe(help);
  expect(within(help).getByRole("button", { name: "Close" })).not.toBeNull();
  expect(sections(help)).toEqual(["This screen", "When a post is focused | Current", "When the timeline is focused"]);
  expect(within(help).getAllByText("Current")).toHaveLength(1);
  expect(section(help, "This screen")).toEqual([
    ["Show keyboard shortcuts", "?"],
    ["New post", "n", "Ctrl+Enter"],
  ]);
  expect(section(help, "When the timeline is focused")).toEqual([["Go to next post", "j", "↓"]]);
});

test("in ja the screen's heading and the current mark are the design system's, and macOS names its keys", () => {
  const help = helpIn("ja", [{ ...post, label: "投稿にフォーカスしているとき" }], true);

  expect(sections(help)).toEqual(["この画面", "投稿にフォーカスしているとき | 現在"]);
  expect(section(help, "この画面")).toEqual([
    ["Show keyboard shortcuts", "?"],
    ["New post", "n", "⌘Enter"],
  ]);
});

test("with nothing focused every group shows, none marked", () => {
  const help = helpIn("en", [
    { ...post, current: false },
    { ...timeline, current: false },
  ]);

  expect(sections(help)).toEqual(["This screen", "When a post is focused", "When the timeline is focused"]);
  expect(within(help).queryByText("Current")).toBeNull();
});
