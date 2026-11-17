// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { screen, within } from "@testing-library/react";
import { renderApp } from "./test/app";

test("a person's notifications show the follows, replies, favourites and boosts of others", async () => {
  const app = await renderApp("/notifications", { viewer: "sam" });

  expect(app.title()).toBe("Notifications");
  const main = screen.getByRole("main");
  expect(within(main).getByRole("heading", { level: 1, name: "Notifications" })).toBeDefined();
  const feed = within(main).getByRole("feed", { name: "Notifications" });
  expect(within(feed).getByRole("article", { name: "Yuki 雪 followed you" })).toBeDefined();
  // A mention is the post that mentions the reader, named by its author.
  const mention = within(feed)
    .getAllByRole("article", { name: "Yuki 雪" })
    .filter((post) => post.textContent.includes("北側の尾根道です"));
  expect(mention).toHaveLength(1);
  expect(mention[0]?.textContent).toContain("Yuki 雪 mentioned you");
  expect(within(feed).getByRole("article", { name: "Yuki 雪 favourited your post" }).textContent).toContain(
    "Day 3 of the sourdough",
  );
  expect(within(feed).getByRole("article", { name: "Hana 花 boosted your post" }).textContent).toContain(
    "Day 3 of the sourdough",
  );
});

test("a notification's line leads to what it tells of, where the screen's heading takes the focus, and Back returns", async () => {
  const app = await renderApp("/notifications", { viewer: "sam" });
  const feed = within(screen.getByRole("main")).getByRole("feed", { name: "Notifications" });

  await app.user.click(within(feed).getByRole("link", { name: "Yuki 雪 followed you" }));

  const heading = await within(screen.getByRole("main")).findByRole("heading", { level: 1, name: /^Yuki 雪/ });
  expect(app.path()).toBe("/@yuki");
  expect(app.title()).toBe(heading.textContent);
  expect(app.focused()).toBe(heading);

  await app.back();
  expect(app.path()).toBe("/notifications");
  expect(app.title()).toBe("Notifications");
});

test("a visitor at the notifications is asked to sign in, in their language", async () => {
  await renderApp("/notifications", { visitor: true, locale: "ja" });

  const main = screen.getByRole("main");
  expect(within(main).getByRole("heading", { level: 1, name: "通知" })).toBeDefined();
  expect(within(main).getByRole("link", { name: "ログイン" }).getAttribute("href")).toBe("/login");
});
