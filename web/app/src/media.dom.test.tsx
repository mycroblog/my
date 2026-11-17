// SPDX-License-Identifier: AGPL-3.0-only

// A person opens a post's pictures whole in the media viewer, from a timeline and from a thread, moves between them
// and closes it back to the picture that opened it, and opens a video whole and plays it in place. What the viewer, the
// players and the cover do by themselves is the design system's (viewer.dom.test.tsx, media.dom.test.tsx), and what a
// post shows of its media, its sensitive cover and its link card is the post's (post.dom.test.tsx): these tests hold
// what a screen adds, the post's own media in them and the focus back in the post.

import { expect, test } from "bun:test";
import { screen, within } from "@testing-library/react";
import { renderApp } from "./test/app";
import { feedOf, postOf } from "./test/posts";

/** The media viewer, once it is open over the screen. */
const viewer = async () => within(await screen.findByRole("dialog", { name: "Media" }));

/** Whether the media viewer is open. */
const viewing = () => screen.queryByRole("dialog", { name: "Media" }) !== null;

const ridge = "朝の山並み。尾根が三つ重なっている";
const sea = "昼の海。空は淡い青";
const loaf = "丸いパンを上から見たところ";

test("a picture opens whole from the home timeline, the arrow keys move between the post's pictures, and Escape returns to it", async () => {
  const app = await renderApp("/", { viewer: "hana" });
  const post = within(postOf(await feedOf("Home timeline"), "Yuki 雪", "週末の散歩、三枚"));
  const opener = post.getByRole("button", { name: `Open image: ${ridge}` });

  await app.user.click(opener);
  const media = await viewer();
  expect(media.getByText("1 of 3")).toBeDefined();
  expect(media.getByRole("paragraph").textContent).toBe(ridge);

  await app.user.keyboard("{ArrowRight}");
  expect(media.getByText("2 of 3")).toBeDefined();
  expect(media.getByRole("paragraph").textContent).toBe(sea);
  expect(media.getByText(`2 of 3, ${sea}`)).toBeDefined();
  await app.user.keyboard("{ArrowRight}");
  expect(media.getByText("3 of 3")).toBeDefined();
  expect(media.getByRole("paragraph").textContent).toBe(loaf);
  // The last of the post's pictures: there is no further to go.
  await app.user.keyboard("{ArrowRight}");
  expect(media.getByText("3 of 3")).toBeDefined();
  await app.user.keyboard("{ArrowLeft}{ArrowLeft}");
  expect(media.getByText("1 of 3")).toBeDefined();
  expect(media.getByText(`1 of 3, ${ridge}`)).toBeDefined();

  await app.user.keyboard("{Escape}");
  expect(viewing()).toBe(false);
  expect(app.focused()).toBe(opener);
});

test("a picture opens from a thread at its own place among the post's pictures, Previous and Next move, and Close returns to it", async () => {
  const app = await renderApp("/", { viewer: "hana" });
  await app.navigate(app.fake.postPath("yuki-walk"));
  const post = within(postOf(await feedOf("Thread"), "Yuki 雪", "週末の散歩、三枚"));
  const opener = post.getByRole("button", { name: `Open image: ${sea}` });

  await app.user.click(opener);
  const media = await viewer();
  expect(media.getByText("2 of 3")).toBeDefined();
  await app.user.click(media.getByRole("button", { name: "Next" }));
  expect(media.getByText("3 of 3")).toBeDefined();
  expect(media.queryByRole("button", { name: "Next" })).toBeNull();
  await app.user.click(media.getByRole("button", { name: "Previous" }));
  await app.user.click(media.getByRole("button", { name: "Previous" }));
  expect(media.getByText("1 of 3")).toBeDefined();
  expect(media.queryByRole("button", { name: "Previous" })).toBeNull();

  await app.user.click(media.getByRole("button", { name: "Close" }));
  expect(viewing()).toBe(false);
  expect(app.focused()).toBe(opener);
});

test("a video opens whole from Open video, with its description and no place among others, and plays in place", async () => {
  const description = "夕暮れの浜に波が寄せる。空が橙から紫に変わっていく";
  const app = await renderApp("/", { viewer: "hana" });
  const article = postOf(await feedOf("Home timeline"), "Yuki 雪", "夕方の潮が満ちてくるところ");
  const post = within(article);
  const open = post.getByRole("button", { name: "Open video" });

  await app.user.click(open);
  const media = await viewer();
  expect(media.getByRole("paragraph").textContent).toBe(description);
  expect(media.queryByText("1 of 1")).toBeNull();
  await app.user.click(media.getByRole("button", { name: "Close" }));
  expect(viewing()).toBe(false);
  expect(app.focused()).toBe(open);

  await app.user.click(post.getByRole("button", { name: `Play video: ${description}` }));
  expect(post.queryByRole("button", { name: `Play video: ${description}` })).toBeNull();
  // The playing video has no role to find it by: it is named by its description.
  expect(post.getByLabelText(description).tagName).toBe("VIDEO");
});
