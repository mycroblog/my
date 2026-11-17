// SPDX-License-Identifier: AGPL-3.0-only

// A person opens a post's pictures whole in the media viewer, from a timeline and from a thread, moves between them
// and closes it back to the picture that opened it; plays a video and a sound in place; and shows sensitive media. A
// link card is a link to its page, and one that cannot be drawn leaves its post drawn. What the viewer, the players
// and the cover do by themselves is the design system's (viewer.dom.test.tsx, media.dom.test.tsx, card.dom.test.tsx):
// these tests hold what a screen adds, the post's own media in them and the focus back in the post.

import { expect, spyOn, test } from "bun:test";
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

test("a sound plays in place and pauses, its time read as where it is of its length", async () => {
  const app = await renderApp("/", { viewer: "hana" });
  const post = within(postOf(await feedOf("Home timeline"), "Marco", "Rain on the tin roof"));

  expect(post.getByText("0:00 of 3:14")).toBeDefined();
  await app.user.click(post.getByRole("button", { name: "Play audio" }));
  await app.user.click(post.getByRole("button", { name: "Pause audio" }));
  expect(post.getByRole("button", { name: "Play audio" })).toBeDefined();
});

test("sensitive media show no picture until shown; the line and a press on the cover show them, and the line hides them", async () => {
  const app = await renderApp("/", { viewer: "hana" });
  const post = within(postOf(await feedOf("Home timeline"), "Yuki 雪", "転んで擦りむいた膝"));
  /** The line's button, Show or Hide, described by what it covers. */
  const line = (action: "Show" | "Hide") => post.getByRole("button", { name: action, description: "Sensitive image" });
  const pictures = () => post.queryAllByRole("button", { name: /^Open image/ });

  expect(line("Show")).toBeDefined();
  expect(pictures()).toHaveLength(0);
  await app.user.click(line("Show"));
  expect(pictures()).toHaveLength(1);
  await app.user.click(line("Hide"));
  expect(pictures()).toHaveLength(0);

  // The covered media are a press too, which a screen reader does not hear: it hears the line alone.
  await app.user.click(post.getByRole("button", { name: "Sensitive image · Show", hidden: true }));
  expect(pictures()).toHaveLength(1);
  expect(line("Hide")).toBeDefined();
});

test("a post's link card is a link to the page, in a new tab", async () => {
  await renderApp("/", { viewer: "hana" });
  const post = within(postOf(await feedOf("Home timeline"), "ليلى Layla", "Translating a short novel"));

  const card = post.getByRole("link", {
    name: /Layla's notes\s*Why jokes are the hardest thing to translate\s*A pun lives in one language\..*\(opens in a new tab\)$/,
  });
  expect(card.getAttribute("href")).toBe("https://layla.notes.test/translating-jokes");
  expect(card.getAttribute("target")).toBe("_blank");
});

test("a link card that cannot be drawn leaves its post drawn", async () => {
  // React reports what the card threw as it drew: the test reads it, and keeps it out of the run's output.
  const reported = spyOn(console, "error").mockImplementation(() => undefined);
  try {
    // The reader starts elsewhere, so that the home timeline's first answer is the one with the card that breaks.
    const app = await renderApp("/notifications", { viewer: "hana" });
    const id = app.fake.idOf("layla-jokes");
    // A title that is no text: the card throws as it draws.
    const read = app.fake.script("GET /timelines/home", {
      change: (page) => ({
        ...page,
        items: page.items.map((status) =>
          status.id === id && status.card != null
            ? { ...status, card: { ...status.card, title: { broken: true } as unknown as string } }
            : status,
        ),
      }),
    });
    await app.navigate("/");
    const post = within(postOf(await feedOf("Home timeline"), "ليلى Layla", "Translating a short novel"));
    expect(read.seen).toHaveLength(1);
    expect(post.queryByRole("link", { name: /\(opens in a new tab\)$/ })).toBeNull();
    expect(post.getByRole("paragraph").textContent).toStartWith("Translating a short novel");
    expect(post.getByRole("button", { name: /^Favourite/ })).toBeDefined();
    // An object drawn as text: the card did throw, and its boundary caught it.
    expect(reported.mock.calls.flat().some((line) => /Objects are not valid as a React child/.test(String(line)))).toBe(
      true,
    );
  } finally {
    reported.mockRestore();
  }
});
