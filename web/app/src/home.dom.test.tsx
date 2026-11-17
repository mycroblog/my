// SPDX-License-Identifier: AGPL-3.0-only

// The home timeline shows the posts of the people the reader follows as the server answers them: images with their
// alt text, a boost with who boosted it, and a content warning before the text it hides.

import { expect, test } from "bun:test";
import { within } from "@testing-library/react";
import { renderApp } from "./test/app";
import { boostOf, feedOf, postOf } from "./test/posts";

test("the images of the home timeline's posts show with their alt text", async () => {
  await renderApp("/", { viewer: "yuki" });
  const feed = await feedOf("Home timeline");

  const walk = within(postOf(feed, "Yuki 雪", "週末の散歩、三枚"));
  for (const alt of ["朝の山並み。尾根が三つ重なっている", "昼の海。空は淡い青", "丸いパンを上から見たところ"])
    expect(walk.getByRole("img", { name: alt })).toBeDefined();
  expect(
    within(postOf(feed, "Sam Rivera", "Day 3 of the sourdough")).getByRole("img", {
      name: "A round loaf of sourdough on a wooden board, seen from above",
    }),
  ).toBeDefined();
});

test("a boost in the home timeline shows the boosted post with who boosted it", async () => {
  await renderApp("/", { viewer: "yuki" });
  const feed = await feedOf("Home timeline");

  const boost = boostOf(feed, "Hana 花", "Sam Rivera", "Day 3 of the sourdough");
  expect(within(boost).getByRole("link", { name: "Sam Rivera" }).getAttribute("href")).toBe("/@sam");
  // The post itself is in the timeline too, without the line of a boost.
  expect(postOf(feed, "Sam Rivera", "Day 3 of the sourdough")).not.toBe(boost);
});

test("a content warning in the home timeline hides the text until the reader shows it", async () => {
  const app = await renderApp("/", { viewer: "yuki" });
  const post = postOf(await feedOf("Home timeline"), "Sam Rivera", "The ending of the film we watched");
  const hidden = () =>
    within(post)
      .queryAllByRole("paragraph")
      .filter((p) => p.textContent.includes("The phone call in the first scene"));

  expect(hidden()).toHaveLength(0);
  await app.user.click(within(post).getByRole("button", { name: "Show" }));
  expect(hidden()).toHaveLength(1);
});
