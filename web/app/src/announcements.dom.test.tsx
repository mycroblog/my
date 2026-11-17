// SPDX-License-Identifier: AGPL-3.0-only

// The server's announcements: the line above the home timeline that counts the unread and leads to their list, where
// one the reader opens is read. The reader is Sam, who has read none of the scenario's three.

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { within } from "@testing-library/react";
import { main, renderApp } from "./test/app";

test("a line above the home timeline counts the unread announcements and leads to the list, where one opened is read", async () => {
  const [opened] = scenario.announcements;
  if (opened === undefined) throw new Error("test: the scenario has no announcement");
  const all = scenario.announcements.length;
  // The line stands above the timeline whatever it holds: the server has the announcements and no post.
  const app = await renderApp("/", {
    viewer: "sam",
    seed: { people: scenario.people, announcements: scenario.announcements },
  });

  await app.user.click(await main().findByRole("link", { name: `${String(all)} unread announcements` }));

  expect(await main().findByRole("heading", { level: 1, name: "Announcements" })).toBeDefined();
  const entry = await main().findByRole("article", { name: opened.title });
  expect(within(entry).getByRole("img", { name: "Unread" })).toBeDefined();
  const read = app.fake.script("POST /announcements/{id}/read");
  await app.user.click(within(entry).getByRole("link", { name: opened.title }));
  expect(await main().findByRole("heading", { level: 1, name: opened.title })).toBeDefined();
  await read.arrived();

  await app.navigate("/");
  expect(await main().findByRole("link", { name: `${String(all - 1)} unread announcements` })).toBeDefined();
});
