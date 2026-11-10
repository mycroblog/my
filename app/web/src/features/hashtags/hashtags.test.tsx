// SPDX-License-Identifier: AGPL-3.0-only
// Followed and featured hashtags through the whole app (web-client/README.md T4; web-client/screens/tag.md,
// settings-followed-tags.md, profile.md, settings-profile.md): MSW plays the server's follows and featured hashtags.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { otherAccountFixture, type Schemas, statusFixture, statusPage } from "../../test/fixtures";
import { apiBase, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { sectionHeadings } from "../../test/sections";
import { featuredTagFixture, followedTagFixture, hashtagFixture } from "./test/handlers";

/** The hashtag of a path parameter, which the client sends with its `#`. */
function nameOf(param: unknown): string {
  return String(param).replace(/^#/u, "");
}

/** Plays a server that keeps the viewer's followed hashtags, the latest first, and records the follows and unfollows. */
function useFollowedTags(initial: string[]) {
  let followed = [...initial];
  const changes: string[] = [];
  server.use(
    http.get(`${apiBase}/tags/:hashtag`, ({ params }) => {
      const name = nameOf(params["hashtag"]);
      return HttpResponse.json(hashtagFixture(name, followed.includes(name.toLowerCase())));
    }),
    http.post(`${apiBase}/tags/:hashtag/follow`, ({ params }) => {
      const name = nameOf(params["hashtag"]);
      changes.push(`follow ${name}`);
      followed = [name.toLowerCase(), ...followed.filter((tag) => tag !== name.toLowerCase())];
      return HttpResponse.json(hashtagFixture(name.toLowerCase(), true));
    }),
    http.delete(`${apiBase}/tags/:hashtag/follow`, ({ params }) => {
      const name = nameOf(params["hashtag"]);
      changes.push(`unfollow ${name}`);
      followed = followed.filter((tag) => tag !== name.toLowerCase());
      return HttpResponse.json(hashtagFixture(name.toLowerCase(), false));
    }),
    http.get(`${apiBase}/followed_tags`, () =>
      HttpResponse.json({
        items: followed.map((name) => followedTagFixture(name)),
      } satisfies Schemas["FollowedHashtagCollection"]),
    ),
  );
  return { changes, followed: () => followed };
}

/** A post of Bob with #cats, whom the viewer does not follow, as the home timeline answers it for a followed hashtag. */
const catPost: Schemas["Status"] = {
  ...statusFixture("0192f0c4-7a8e-7000-8000-0000000000c1"),
  content: "<p>A sleepy cat. #cats</p>",
  mentions: [],
  tags: [{ name: "cats", url: "https://my.example/tags/cats" }],
  followed_tags: ["cats"],
};

describe("the hashtag page", () => {
  test("follows the hashtag, whose posts are then on home without a reload", async () => {
    server.use(signedInHandler);
    const { changes, followed } = useFollowedTags([]);
    let homeRequests = 0;
    server.use(
      http.get(`${apiBase}/timelines/home`, () => {
        homeRequests++;
        return HttpResponse.json(statusPage(followed().includes("cats") ? [catPost] : []));
      }),
    );
    const { user, router } = await renderApp({ path: "/home" });
    expect(await screen.findByRole("heading", { level: 1, name: "Home" })).toBeInTheDocument();
    const before = homeRequests;

    // To the hashtag's page and back, as a reader does, without loading the app again.
    await router.navigate({ to: "/tags/$tag", params: { tag: "cats" } });
    const follow = await screen.findByRole("button", { name: "Follow" });
    expect(follow).toHaveAccessibleDescription(
      "Its public posts join your home timeline, also from people you don't follow.",
    );
    await user.click(follow);

    const following = await screen.findByRole("button", { name: "Following" });
    expect(following).toHaveFocus();
    expect(await screen.findByText(/^You follow ⁨#cats⁩ now\.$/u)).toBeInTheDocument();
    expect(changes).toEqual(["follow cats"]);

    await router.navigate({ to: "/home" });
    const post = await screen.findByRole("article", { name: /^Followed hashtag #cats/u });
    expect(homeRequests).toBeGreaterThan(before);
    // The mark leads to the hashtag's page, where the follow is undone.
    expect(within(post).getByRole("link", { name: "#cats" })).toHaveAttribute("href", "/tags/cats");
  });

  test("unfollows the hashtag with the same button", async () => {
    server.use(signedInHandler);
    const { changes } = useFollowedTags(["cats"]);
    const { user } = await renderApp({ path: "/tags/cats" });

    await user.click(await screen.findByRole("button", { name: "Following" }));

    expect(await screen.findByRole("button", { name: "Follow" })).toHaveFocus();
    expect(await screen.findByText(/^You no longer follow ⁨#cats⁩\.$/u)).toBeInTheDocument();
    expect(changes).toEqual(["unfollow cats"]);
  });

  test("says in a toast that no more hashtags can be followed, and keeps the button", async () => {
    server.use(
      signedInHandler,
      http.post(`${apiBase}/tags/:hashtag/follow`, () =>
        HttpResponse.json(
          { type: "/api/v1/problems/too-many-followed-tags", title: "Too many followed hashtags", status: 422 },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: "/tags/cats" });

    await user.click(await screen.findByRole("button", { name: "Follow" }));

    expect(
      await screen.findByText("You follow as many hashtags as you can (100). Unfollow one to follow another."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Follow" })).toBeInTheDocument();
  });

  test("has no follow button for a visitor who is not signed in", async () => {
    await renderApp({ path: "/tags/cats" });

    expect(await screen.findByRole("heading", { level: 1, name: "#cats" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Follow" })).not.toBeInTheDocument();
  });
});

describe("the followed hashtags in the settings", () => {
  beforeEach(() => {
    server.use(signedInHandler);
  });

  test("opens from the settings, follows a hashtag typed with its #, and lists it first", async () => {
    const { changes } = useFollowedTags(["sourdough"]);
    const { user } = await renderApp({ path: "/settings" });

    await user.click(await screen.findByRole("link", { name: "Followed hashtags" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Followed hashtags" })).toBeInTheDocument();
    // docs/architecture/web-client/screens/settings-followed-tags.md §3
    expect(sectionHeadings()).toEqual(["Hashtags you follow"]);
    const list = screen.getByRole("region", { name: "Hashtags you follow" });
    expect(within(list).getByRole("link", { name: "#sourdough" })).toHaveAttribute("href", "/tags/sourdough");

    await user.click(within(list).getByRole("button", { name: "Follow a hashtag" }));
    const form = within(list).getByRole("region", { name: "Follow a hashtag" });
    const field = within(form).getByRole("textbox", { name: "Hashtag" });
    expect(field).toHaveFocus();
    await user.type(field, "#Bread");
    await user.click(within(form).getByRole("button", { name: "Follow" }));

    expect(await within(list).findByRole("status")).toHaveTextContent("You follow ⁨#bread⁩ now.");
    expect(within(list).getByRole("button", { name: "Follow a hashtag" })).toHaveFocus();
    expect(changes).toEqual(["follow Bread"]);
    const rows = await within(list).findAllByRole("listitem");
    expect(rows.map((row) => row.getAttribute("data-row-id"))).toEqual(["bread", "sourdough"]);
  });

  test("refuses a text that is no hashtag before sending it", async () => {
    const { changes } = useFollowedTags([]);
    const { user } = await renderApp({ path: "/settings/followed-tags" });

    await user.click(await screen.findByRole("button", { name: "Follow a hashtag" }));
    const field = screen.getByRole("textbox", { name: "Hashtag" });
    await user.type(field, "two words");
    await user.click(screen.getByRole("button", { name: "Follow" }));

    expect(
      await screen.findByText("A hashtag has letters, digits and _ only, with at least one letter."),
    ).toBeInTheDocument();
    expect(changes).toEqual([]);
  });

  test("unfollowing takes the hashtag off the list and keeps focus in the list", async () => {
    const { changes } = useFollowedTags(["bread", "cats"]);
    const { user } = await renderApp({ path: "/settings/followed-tags" });

    await user.click(await screen.findByRole("button", { name: "Unfollow: #bread" }));

    await waitFor(() => {
      expect(screen.queryByRole("link", { name: "#bread" })).not.toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Unfollow: #cats" })).toHaveFocus();
    expect(await screen.findByText("You no longer follow ⁨#bread⁩.")).toBeInTheDocument();
    expect(changes).toEqual(["unfollow bread"]);
  });
});

describe("featured hashtags", () => {
  test("the profile lists the hashtags it features, in its order, with their counts", async () => {
    server.use(
      http.get(`${apiBase}/accounts/:id/featured_tags`, ({ params }) =>
        HttpResponse.json({
          items:
            params["id"] === otherAccountFixture.id
              ? [featuredTagFixture("sourdough", 12, "2026-11-03T09:00:00Z"), featuredTagFixture("rye")]
              : [],
        } satisfies Schemas["FeaturedHashtagCollection"]),
      ),
    );
    await renderApp({ path: "/@bob" });

    const region = await screen.findByRole("region", { name: "Featured hashtags" });
    const items = within(region).getAllByRole("listitem");
    expect(items.map((item) => item.textContent)).toEqual(["#sourdough12 posts, last on Nov 3, 2026", "#rye0 posts"]);
    expect(within(region).getByRole("link", { name: "#sourdough" })).toHaveAttribute("href", "/tags/sourdough");
  });

  test("the address another server links a featured hashtag to opens the hashtag's page", async () => {
    await renderApp({ path: "/@bob/tagged/sourdough" });

    expect(await screen.findByRole("heading", { level: 1, name: "#sourdough" })).toBeInTheDocument();
  });

  test("the profile form saves the featured hashtags with the profile, in the order chosen", async () => {
    server.use(signedInHandler);
    const saved: string[][] = [];
    server.use(
      http.get(`${apiBase}/featured_tags`, () =>
        HttpResponse.json({ items: [featuredTagFixture("rye", 3)] } satisfies Schemas["FeaturedHashtagCollection"]),
      ),
      http.put(`${apiBase}/featured_tags`, async ({ request }) => {
        const { names } = (await request.json()) as Schemas["SetFeaturedTagsRequest"];
        saved.push(names);
        return HttpResponse.json({ items: names.map((name) => featuredTagFixture(name.replace(/^#/u, ""))) });
      }),
    );
    const { user } = await renderApp({ path: "/settings/profile" });

    const region = await screen.findByRole("region", { name: "Featured hashtags" });
    expect(within(region).getByRole("textbox", { name: "Hashtag 1" })).toHaveValue("rye");
    await user.click(within(region).getByRole("button", { name: "Add a hashtag" }));
    await user.type(within(region).getByRole("textbox", { name: "Hashtag 2" }), "#sourdough");
    // Moved up: the focus stays with it, on the button left that it can still press.
    await user.click(within(region).getByRole("button", { name: "Move hashtag 2 up" }));
    expect(await screen.findByText("Moved ⁨#sourdough⁩ to place 1.")).toBeInTheDocument();
    expect(within(region).getByRole("textbox", { name: "Hashtag 1" })).toHaveValue("#sourdough");
    await waitFor(() => {
      expect(within(region).getByRole("button", { name: "Move hashtag 1 down" })).toHaveFocus();
    });

    await user.click(screen.getByRole("button", { name: "Save profile" }));

    expect(await screen.findByText("Your profile has been saved.")).toBeInTheDocument();
    expect(saved).toEqual([["sourdough", "rye"]]);
  });
});
