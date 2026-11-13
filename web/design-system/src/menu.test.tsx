// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { MenuButton, MenuGroup, MenuItem, MenuPanel } from "./menu";
import { PreviewItems } from "./preview";

const items = (
  <>
    <MenuGroup>
      <MenuItem id="link" icon="link">
        Copy link
      </MenuItem>
    </MenuGroup>
    <MenuGroup title="@mika">
      <MenuItem id="block" icon="block">
        Block…
      </MenuItem>
    </MenuGroup>
  </>
);

test("the menu is named, its items are menu items, and a group's caption names it", () => {
  const html = renderToStaticMarkup(<MenuPanel label="More">{items}</MenuPanel>);
  expect(html).toContain('role="menu"');
  expect(html).toContain('aria-label="More"');
  expect(html).toContain('role="menuitem"');
  expect(html).toContain(">@mika</header>");
  expect(html).toContain("shadow-overlay");
});

test("an item shows a state at rest by its id", () => {
  const html = renderToStaticMarkup(
    <PreviewItems states={{ block: "focus-visible" }}>
      <MenuPanel label="More">{items}</MenuPanel>
    </PreviewItems>,
  );
  expect(html.match(/shadow-focus-inset/g)?.length).toBe(1);
  expect(html.match(/bg-surface-raised(?!-)/g)?.length).toBe(1);
});

test("the button opens a menu", () => {
  const html = renderToStaticMarkup(
    <MenuButton icon="more" label="More">
      {items}
    </MenuButton>,
  );
  expect(html).toContain('aria-haspopup="true"');
  expect(html).toContain('aria-label="More"');
});

test("the button may show a count and an on state, and line its menu up with its start", () => {
  const html = renderToStaticMarkup(
    <MenuButton icon="boost" label="Boost" count={12} pressed align="start">
      {items}
    </MenuButton>,
  );
  expect(html).toContain('aria-label="Boost, 12"');
  expect(html).toContain('aria-pressed="true"');
  expect(html).toContain(">12</span>");
  const plain = renderToStaticMarkup(
    <MenuButton icon="more" label="More">
      {items}
    </MenuButton>,
  );
  expect(plain).not.toContain("aria-pressed=");
});
