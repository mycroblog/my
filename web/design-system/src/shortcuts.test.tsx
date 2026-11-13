// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { useRef } from "react";
import { I18nProvider } from "react-aria-components";
import { renderToStaticMarkup } from "react-dom/server";
import { IconButton } from "./icon-button";
import { MenuButton, MenuGroup, MenuItem } from "./menu";
import { ActionScope, ShortcutList, ShortcutProvider, useAction } from "./shortcuts";

const keymap = { "post.reply": { keys: ["r"] }, "compose.open": { keys: ["n", "Mod+Enter"] } };

function Reply() {
  const keys = useAction("post.reply", () => undefined, { scope: "focus", label: "Reply" });
  return <IconButton icon="reply" label="Reply" {...keys} />;
}

function Post() {
  const ref = useRef<HTMLElement>(null);
  return (
    <article ref={ref}>
      <ActionScope target={ref}>
        <Reply />
      </ActionScope>
    </article>
  );
}

function More() {
  const keys = useAction("compose.open", () => undefined, { scope: "screen", label: "New post" });
  return (
    <MenuButton icon="more" label="More" {...keys}>
      <MenuGroup>
        <MenuItem id="a" icon="link">
          Copy link
        </MenuItem>
      </MenuGroup>
    </MenuButton>
  );
}

test("a declared action puts its keys on its control, Mod named for the platform", () => {
  const html = (mac: boolean) =>
    renderToStaticMarkup(
      <ShortcutProvider keymap={keymap} mac={mac}>
        <Post />
        <More />
      </ShortcutProvider>,
    );
  expect(html(false)).toContain('aria-keyshortcuts="r"');
  expect(html(false)).toContain('aria-keyshortcuts="n Control+Enter"');
  expect(html(true)).toContain('aria-keyshortcuts="n Meta+Enter"');
});

test("without a provider a part declares nothing and works on its own", () => {
  const html = renderToStaticMarkup(<Post />);
  expect(html).toContain('aria-label="Reply"');
  expect(html).not.toContain("aria-keyshortcuts");
});

test("a focus action outside an ActionScope is refused", () => {
  expect(() =>
    renderToStaticMarkup(
      <ShortcutProvider keymap={keymap}>
        <Reply />
      </ShortcutProvider>,
    ),
  ).toThrow(/no ActionScope/);
});

test("the provider refuses a keymap with a chord the browser keeps", () => {
  expect(() =>
    renderToStaticMarkup(
      <ShortcutProvider keymap={{ "compose.open": { keys: ["Mod+T"] } }}>
        <div />
      </ShortcutProvider>,
    ),
  ).toThrow(/keeps/);
});

const screen = [
  { id: "shortcuts.help", label: "Show keyboard shortcuts", keys: ["?"] },
  { id: "compose.open", label: "New post", keys: ["n", "Mod+Enter"] },
];
const post = {
  id: "post",
  label: "When a post is focused",
  current: true,
  actions: [{ id: "post.reply", label: "Reply", keys: ["r"] }],
};
const timeline = {
  id: "timeline",
  label: "When the timeline is focused",
  current: false,
  actions: [{ id: "timeline.next", label: "Go to next post", keys: ["j", "ArrowDown"] }],
};

/** Each section's heading, whether it is marked current, and the ids of its actions, in order. */
function sections(html: string): [string, boolean, string[]][] {
  return html
    .split("<section aria-labelledby")
    .slice(1)
    .map((section) => {
      const heading = /<h3[^>]*>(.*?)<\/h3>/.exec(section)?.[1] ?? "";
      const title = /^[^<]*/.exec(heading)?.[0] ?? "";
      const actions = [...section.matchAll(/data-action="([^"]+)"/g)].map((m) => m[1] ?? "");
      return [title, /^[^>]*data-current/.test(section), actions];
    });
}

const render = (locale: string, groups: (typeof post)[], mac = false) =>
  renderToStaticMarkup(
    <I18nProvider locale={locale}>
      <ShortcutList screen={screen} groups={groups} mac={mac} />
    </I18nProvider>,
  );

test("the help lists the screen's actions first, then each group, the current one marked", () => {
  const html = render("en", [post, timeline]);
  expect(html).toContain('role="dialog"');
  expect(html).toContain("Keyboard shortcuts");
  expect(sections(html)).toEqual([
    ["This screen", false, ["shortcuts.help", "compose.open"]],
    ["When a post is focused", true, ["post.reply"]],
    ["When the timeline is focused", false, ["timeline.next"]],
  ]);
  expect(html).toContain(">Current</span></h3>");
  expect(html.match(/>Current</g)).toHaveLength(1);
  expect(html).toContain(">Ctrl+Enter</kbd>");
  expect(html).toContain(">↓</kbd>");
  expect(html).toContain('aria-label="Close"');
});

test("in ja the screen's heading and the current mark are the design system's", () => {
  const ja = render("ja", [{ ...post, label: "投稿にフォーカスしているとき" }], true);
  expect(sections(ja).map(([title, current]) => [title, current])).toEqual([
    ["この画面", false],
    ["投稿にフォーカスしているとき", true],
  ]);
  expect(ja).toContain(">現在</span></h3>");
  expect(ja).toContain(">⌘Enter</kbd>");
});

test("with nothing focused every group shows, none marked", () => {
  const html = render("en", [
    { ...post, current: false },
    { ...timeline, current: false },
  ]);
  expect(sections(html).map(([title, current]) => [title, current])).toEqual([
    ["This screen", false],
    ["When a post is focused", false],
    ["When the timeline is focused", false],
  ]);
});
