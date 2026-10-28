// SPDX-License-Identifier: AGPL-3.0-only
// The emoji reactions of a post through the whole app (web-client/README.md T4, FR-116, ADR-0052): the counts under the
// post, joining and leaving one, the picker, the Like slot shared with the favourite, and the accounts that reacted.
import { screen, waitFor, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { frequentEmojisKey, skinToneKey } from "../../lib/storage";
import { apiBase, oauthPaths, problem, tokenResponse } from "../../test/msw/http";
import { type Schemas, statusFixture } from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import type { Status } from "./api/status";

const postId = "0192f0c4-7a8e-7000-8000-0000000000d1";
const blobcat: Schemas["CustomEmoji"] = {
  shortcode: "blobcat",
  url: "https://remote.example/blobcat.png",
  static_url: "https://remote.example/blobcat.png",
};
const post: Status = {
  ...statusFixture(postId),
  content: "<p>React to me</p>",
  favourited: true,
  favourites_count: 2,
  reactions: [
    { name: "👍", count: 2, me: false },
    {
      name: "blobcat@remote.example",
      shortcode: "blobcat",
      domain: "remote.example",
      emoji: blobcat,
      count: 1,
      me: false,
    },
  ],
};

function signedIn() {
  server.use(http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")));
}

/** Serves the post, and records the emoji of each PUT of a reaction in `sent` (null for a DELETE). */
function servePost(sent: (string | null)[] = []) {
  server.use(
    http.get(`${apiBase}/statuses/:id`, () => HttpResponse.json(post)),
    http.put(`${apiBase}/statuses/:id/reaction`, async ({ request }) => {
      const { emoji } = (await request.json()) as Schemas["ReactToStatusRequest"];
      sent.push(emoji);
      const thumbs = emoji === "👍";
      return HttpResponse.json({
        ...post,
        favourited: false,
        favourites_count: 1,
        reactions: [
          { name: "👍", count: thumbs ? 3 : 2, me: thumbs },
          post.reactions[1] as Schemas["StatusReaction"],
          ...(thumbs ? [] : [{ name: emoji, count: 1, me: true }]),
        ],
      } satisfies Status);
    }),
    http.delete(`${apiBase}/statuses/:id/reaction`, () => {
      sent.push(null);
      return HttpResponse.json({ ...post, favourited: false, favourites_count: 1 });
    }),
  );
}

/** The article of the post the page is about. */
async function findPost() {
  return screen.findByRole("article", { description: "React to me" });
}

/** Opens the picker of reactions of `article` and waits for its emoji, with the search field focused. */
async function openPicker(user: UserEvent, article: HTMLElement) {
  await user.click(await within(article).findByRole("button", { name: "React" }));
  const picker = await screen.findByRole("dialog", { name: "Emoji to react with" });
  const search = await within(picker).findByRole("searchbox", { name: "Search emoji" }, { timeout: 5000 });
  await waitFor(() => {
    expect(search).toHaveFocus();
  });
  return picker;
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"], now: new Date("2026-10-16T12:00:00Z") });
  sessionStorage.clear();
  localStorage.clear();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("the reactions of a post", () => {
  test("show each emoji with its count, and pressing one reacts with it in place of the favourite", async () => {
    signedIn();
    const sent: (string | null)[] = [];
    servePost(sent);
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    const article = await findPost();
    const reactions = within(article).getByRole("group", { name: "Reactions" });
    const thumbs = await within(reactions).findByRole("button", { name: "React with 👍" });
    expect(thumbs).toHaveAttribute("aria-pressed", "false");
    expect(thumbs).toHaveAccessibleDescription("2");
    // Another server's custom emoji is shown with its count, but nobody here can react with it (ADR-0052 §2).
    expect(within(reactions).getByRole("img", { name: ":blobcat:" })).toBeInTheDocument();
    expect(within(reactions).queryByRole("button", { name: /blobcat/ })).not.toBeInTheDocument();
    const favourite = within(article).getByRole("button", { name: "Favourite" });
    expect(favourite).toHaveAttribute("aria-pressed", "true");

    await user.click(thumbs);

    // The reaction takes the favourite's place at once.
    expect(within(article).getByRole("button", { name: "React with 👍" })).toHaveAttribute("aria-pressed", "true");
    expect(within(article).getByRole("button", { name: "Favourite" })).toHaveAttribute("aria-pressed", "false");
    await waitFor(() => {
      expect(sent).toEqual(["👍"]);
    });
    expect(within(article).getByRole("button", { name: "React with 👍" })).toHaveAccessibleDescription("3");

    // Pressing it again takes the reaction back.
    await user.click(within(article).getByRole("button", { name: "React with 👍" }));
    await waitFor(() => {
      expect(sent).toEqual(["👍", null]);
    });
    expect(within(article).getByRole("button", { name: "React with 👍" })).toHaveAttribute("aria-pressed", "false");
  });

  test("react with a Unicode emoji found by searching the picker", async () => {
    signedIn();
    const sent: (string | null)[] = [];
    servePost(sent);
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    const article = await findPost();

    const picker = await openPicker(user, article);
    // Nothing used yet: the default reactions come first, then the nine groups of every Unicode emoji.
    expect(within(picker).getAllByRole("gridcell")[0]).toHaveAccessibleName("thumbs up");
    expect(within(picker).getByRole("toolbar", { name: "Emoji categories" })).toBeInTheDocument();
    await user.type(within(picker).getByRole("searchbox", { name: "Search emoji" }), "party popper");
    await user.click(within(picker).getByRole("gridcell", { name: "party popper" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Emoji to react with" })).not.toBeInTheDocument();
    });
    expect(await within(article).findByRole("button", { name: "React with 🎉" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    await waitFor(() => {
      expect(sent).toEqual(["🎉"]);
    });
    // The post form's picker offers it first from now on.
    expect(frequentEmojisKey.read().map((use) => use.emoji)).toEqual(["🎉"]);
  }, 30_000);

  test("react with the first emoji of a category moved to with its button", async () => {
    signedIn();
    const sent: (string | null)[] = [];
    servePost(sent);
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    const article = await findPost();

    const picker = await openPicker(user, article);
    const toolbar = within(picker).getByRole("toolbar", { name: "Emoji categories" });
    await user.click(within(toolbar).getByRole("button", { name: "Activities" }));
    // The button moves focus to the first emoji of its section.
    await waitFor(() => {
      expect(within(picker).getByRole("gridcell", { name: "jack-o-lantern" })).toHaveFocus();
    });
    await user.keyboard("{Enter}");

    await waitFor(() => {
      expect(sent).toEqual(["🎃"]);
    });
    expect(screen.queryByRole("dialog", { name: "Emoji to react with" })).not.toBeInTheDocument();
  }, 30_000);

  test("react with an emoji in the skin tone chosen, which the counts then show", async () => {
    signedIn();
    const sent: (string | null)[] = [];
    servePost(sent);
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    const article = await findPost();

    const picker = await openPicker(user, article);
    await user.click(within(picker).getByRole("button", { name: "Skin tone" }));
    await user.click(await screen.findByRole("menuitemradio", { name: "Medium skin tone" }));
    expect(skinToneKey.read()).toBe("medium");
    await user.type(within(picker).getByRole("searchbox", { name: "Search emoji" }), "waving hand");
    await user.click(within(picker).getByRole("gridcell", { name: "waving hand: medium skin tone" }));

    await waitFor(() => {
      expect(sent).toEqual(["👋🏽"]);
    });
    const reactions = within(article).getByRole("group", { name: "Reactions" });
    const waving = await within(reactions).findByRole("button", { name: "React with 👋🏽" });
    expect(waving).toHaveAttribute("aria-pressed", "true");
    expect(waving).toHaveAccessibleDescription("1");
    // It is counted as used without its skin tone.
    expect(frequentEmojisKey.read().map((use) => use.emoji)).toEqual(["👋"]);
  }, 30_000);

  test("close the picker after one emoji even when it is picked with Shift held", async () => {
    signedIn();
    const sent: (string | null)[] = [];
    servePost(sent);
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    const article = await findPost();

    const picker = await openPicker(user, article);
    await user.keyboard("{Shift>}");
    await user.click(within(picker).getAllByRole("gridcell", { name: "party popper" })[0] as HTMLElement);
    await user.keyboard("{/Shift}");

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Emoji to react with" })).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(sent).toEqual(["🎉"]);
    });
  }, 30_000);

  test("react with a custom emoji of this server", async () => {
    signedIn();
    const sent: (string | null)[] = [];
    servePost(sent);
    const partyblob = {
      shortcode: "partyblob",
      url: "https://media.example/emoji/partyblob.png",
      static_url: "https://media.example/emoji/partyblob.png",
      aliases: [],
    };
    server.use(
      http.get(`${apiBase}/custom_emojis`, () =>
        HttpResponse.json({ items: [partyblob] } satisfies Schemas["PickerCustomEmojiList"]),
      ),
    );
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    const article = await findPost();

    const picker = await openPicker(user, article);
    await user.type(within(picker).getByRole("searchbox", { name: "Search emoji" }), "partyblob");
    await user.click(await within(picker).findByRole("gridcell", { name: ":partyblob:" }));

    await waitFor(() => {
      expect(sent).toEqual(["partyblob"]);
    });
    expect(frequentEmojisKey.read().map((use) => use.emoji)).toEqual([":partyblob:"]);
  }, 30_000);

  test("put the favourite back and say why when the server refuses the reaction", async () => {
    signedIn();
    servePost();
    server.use(
      http.put(`${apiBase}/statuses/:id/reaction`, () =>
        problem(422, "about:blank", "Not an emoji you can react with."),
      ),
    );
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    const article = await findPost();

    await user.click(await within(article).findByRole("button", { name: "React with 👍" }));

    expect(
      await within(await screen.findByRole("region", { name: "Messages" })).findByText(
        /Not an emoji you can react with/,
      ),
    ).toBeInTheDocument();
    expect(within(article).getByRole("button", { name: "React with 👍" })).toHaveAttribute("aria-pressed", "false");
    expect(within(article).getByRole("button", { name: "Favourite" })).toHaveAttribute("aria-pressed", "true");
  });

  test("list the accounts that reacted", async () => {
    signedIn();
    servePost();
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    const article = await findPost();

    await user.click(within(article).getByRole("button", { name: "Who reacted" }));

    const dialog = await screen.findByRole("dialog", { name: "Who reacted" });
    const [row] = await within(dialog).findAllByRole("listitem");
    expect(within(row as HTMLElement).getByRole("link", { name: /Bob/ })).toBeInTheDocument();
    expect(row).toHaveTextContent("👍");

    // The × button closes it by click or tap alone.
    await user.click(within(dialog).getByRole("button", { name: "Close" }));
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
  });

  test("are shown without buttons while nobody is signed in", async () => {
    servePost();
    await renderApp({ path: `/@bob/status/${postId}` });
    const article = await findPost();

    expect(within(article).getByRole("group", { name: "Reactions" })).toHaveTextContent("👍");
    expect(within(article).queryByRole("button", { name: "React with 👍" })).not.toBeInTheDocument();
  });
});
