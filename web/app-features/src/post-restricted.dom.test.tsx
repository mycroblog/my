// SPDX-License-Identifier: AGPL-3.0-only

// A post's action refused for a partial restriction of the reader's account (#3216): the notice at the bottom says the
// server's sentence and links to Account status, with no Try again, since trying again is refused the same; any other
// failure still offers Try again. The actions are pressed on the home timeline under PostActions.

import { describe, expect, test } from "bun:test";
import { accountStatusPath } from "@my/app-components";
import { scenario } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { NavigationProvider } from "@my/design-system";
import { PostActions } from "./post-actions";
import { fakeData, renderFeature } from "./test/feature";
import { HomeTimeline, loadHomeTimeline } from "./timeline/home";

/** The server's sentence of an interaction refused for a restriction. */
const interacting = "A moderation action keeps your account from interacting with others for now.";
/** The server's sentence of a post refused for a restriction. */
const posting = "A moderation action keeps your account from posting for now.";

/** A problem of the server of `type`, saying `detail`. */
const refusal = (type: string, detail: string) =>
  ({ type: `/api/v1/problems/${type}`, title: "Forbidden", status: 403, detail }) as const;

/** The home timeline of a reader who follows sam, who asks in a poll, under PostActions and the app's navigation. */
async function timeline() {
  const made = fakeData({
    seed: {
      people: scenario.people,
      follows: [{ follower: "yuki", followee: "sam" }],
      posts: [
        {
          key: "asks",
          author: "sam",
          text: "Which loaves?",
          visibility: "public",
          language: "en",
          poll: { options: ["Sourdough", "Rye"], days: 1 },
        },
      ],
    },
  });
  await loadHomeTimeline(made.data);
  const went: string[] = [];
  const user = renderFeature(
    made.data,
    <NavigationProvider navigate={(to) => went.push(to)}>
      <PostActions onCompose={() => undefined}>
        <HomeTimeline />
      </PostActions>
    </NavigationProvider>,
  );
  const post = () => {
    const found = screen
      .getAllByRole("article")
      .find((article) => within(article).queryAllByText("Which loaves?").length > 0);
    if (found === undefined) throw new Error("test: the timeline shows no post of sam's poll");
    return within(found);
  };
  return { ...made, user, went, post };
}

type Drawn = Awaited<ReturnType<typeof timeline>>;

/** The status that says `words`, once it does. */
const noticeSaying = (words: string) =>
  waitFor(() => {
    const notice = screen.queryAllByRole("status").find((region) => region.textContent.includes(words));
    if (notice === undefined) throw new Error(`test: no status says "${words}"`);
    return notice;
  });

type Action = {
  name: string;
  route:
    | "POST /statuses/{id}/favourite"
    | "POST /statuses/{id}/boost"
    | "PUT /statuses/{id}/reaction"
    | "POST /statuses/{id}/poll/votes"
    | "POST /statuses/{id}/bookmark";
  /** Presses the action on sam's post. */
  press: (drawn: Drawn) => Promise<void>;
};

const actions: readonly Action[] = [
  {
    name: "a favourite",
    route: "POST /statuses/{id}/favourite",
    press: async ({ user, post }) => {
      await user.click(post().getByRole("button", { name: "Favourite", pressed: false }));
    },
  },
  {
    name: "a boost",
    route: "POST /statuses/{id}/boost",
    press: async ({ user, post }) => {
      await user.click(post().getByRole("button", { name: /^Boost/ }));
      await user.click(await screen.findByRole("menuitem", { name: "Boost" }));
    },
  },
  {
    name: "a reaction",
    route: "PUT /statuses/{id}/reaction",
    press: async ({ user, post }) => {
      await user.click(post().getByRole("button", { name: "React" }));
      const grid = await within(await screen.findByRole("dialog", { name: "React" })).findByRole("grid", {
        name: "Emoji",
      });
      await user.click(
        within(within(grid).getByRole("rowgroup", { name: "Smileys & emotion" })).getByRole("gridcell", {
          name: "grinning face",
        }),
      );
    },
  },
  {
    name: "a vote",
    route: "POST /statuses/{id}/poll/votes",
    press: async ({ user, post }) => {
      const poll = within(post().getByRole("group", { name: "Poll" }));
      await user.click(poll.getByRole("radio", { name: "Rye" }));
      await user.click(poll.getByRole("button", { name: "Vote" }));
    },
  },
  {
    name: "a bookmark from the More menu",
    route: "POST /statuses/{id}/bookmark",
    press: async ({ user, post }) => {
      await user.click(post().getByRole("button", { name: "More" }));
      await user.click(
        within(await screen.findByRole("menu", { name: "More" })).getByRole("menuitem", { name: "Bookmark" }),
      );
    },
  },
];

describe.each(actions)("$name", ({ route, press }) => {
  for (const [type, words] of [
    ["interaction-restricted", interacting],
    ["posting-restricted", posting],
  ] as const)
    test(`refused with ${type}: the notice says the server's sentence with the link to Account status, and no Try again`, async () => {
      const drawn = await timeline();
      const sent = drawn.fake.script(route, { fail: refusal(type, words) });

      await press(drawn);

      const notice = await noticeSaying(words);
      expect(sent.seen.length).toBeGreaterThan(0);
      const link = within(notice).getByRole("link", { name: "Account status" });
      expect(link.getAttribute("href")).toBe(accountStatusPath);
      expect(screen.queryAllByRole("button", { name: "Try again" })).toHaveLength(0);

      await drawn.user.click(link);
      expect(drawn.went).toEqual([accountStatusPath]);
    });

  test("refused with a 403 of no type of its own: the notice says the server's words with Try again, and no link", async () => {
    const drawn = await timeline();
    const words = "You may not do this.";
    drawn.fake.script(route, { fail: { type: "about:blank", title: words, status: 403, detail: words } });

    await press(drawn);

    const notice = await waitFor(() => {
      const found = screen
        .queryAllByRole("status")
        .find((region) => within(region).queryAllByRole("button").length > 0);
      if (found === undefined) throw new Error("test: no status offers an action");
      return found;
    });
    expect(within(notice).getAllByRole("button", { name: "Try again" })).toHaveLength(1);
    expect(screen.queryAllByRole("link", { name: "Account status" })).toHaveLength(0);
  });
});
