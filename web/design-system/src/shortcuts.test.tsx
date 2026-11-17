// SPDX-License-Identifier: AGPL-3.0-only

// What the shortcuts refuse when a part is drawn. What they do on a page is in shortcuts.dom.test.tsx.

import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { IconButton } from "./icon-button";
import { ShortcutProvider, useAction } from "./shortcuts";

const keymap = { "post.reply": { keys: ["r"] } };

function Reply() {
  const keys = useAction("post.reply", () => undefined, { scope: "focus", label: "Reply" });
  return <IconButton icon="reply" label="Reply" {...keys} />;
}

test("a focus action outside an ActionScope is refused", () => {
  expect(() =>
    renderToStaticMarkup(
      <ShortcutProvider keymap={keymap} mac={false}>
        <Reply />
      </ShortcutProvider>,
    ),
  ).toThrow(/no ActionScope/);
});

test("the provider refuses a keymap with a chord the browser keeps", () => {
  expect(() =>
    renderToStaticMarkup(
      <ShortcutProvider keymap={{ "compose.open": { keys: ["Mod+T"] } }} mac={false}>
        <div />
      </ShortcutProvider>,
    ),
  ).toThrow(/keeps/);
});
