// SPDX-License-Identifier: AGPL-3.0-only
// The profile pages through the whole app (web-client/README.md T4): the real router and QueryClient, with MSW as
// the server (FR-204, FR-301 to FR-303). Each state of the viewer's relationship with the account shows its
// own buttons; the changes go to the server and the page shows the relationship the server returns.
import { fireEvent, isInaccessible, screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import { findLoadingRegion } from "../../test/loading";
import { apiBase, problem, resumableSession } from "../../test/msw/http";
import {
  imageFixture,
  otherAccountFixture,
  relationshipFixture,
  remoteAccountFixture,
  statusPage,
  timelineStatusFixture,
  viewerFixture,
} from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { newTabName } from "../../test/links";
import { renderApp } from "../../test/render";
import type { Relationship } from "./api/accounts";

const bob = { ...otherAccountFixture, followers_count: 12, following_count: 1 };

/** Bob's profile, with the viewer's relationship with him; returns the changes the server received. */
function bobServer(relationship: Partial<Relationship> = {}, account: Partial<typeof bob> = {}) {
  const changes: string[] = [];
  server.use(
    http.get(`${apiBase}/accounts/lookup`, ({ request }) =>
      new URL(request.url).searchParams.get("acct") === "bob"
        ? HttpResponse.json({ ...bob, ...account })
        : problem(404, "about:blank", "Not Found"),
    ),
    http.get(`${apiBase}/accounts/relationships`, () => HttpResponse.json([relationshipFixture(bob.id, relationship)])),
    http.post(`${apiBase}/accounts/:id/:action`, ({ params }) => {
      const action = String(params["action"]);
      changes.push(action);
      const after: Partial<Relationship> =
        action === "follow"
          ? account.locked === true
            ? { requested: true }
            : { following: true }
          : action === "block"
            ? { blocking: true }
            : action === "mute"
              ? { muting: true, muting_notifications: true }
              : action === "subscribe"
                ? { ...relationship, notify: true }
                : action === "unsubscribe"
                  ? { ...relationship, notify: false }
                  : {};
      return HttpResponse.json(relationshipFixture(String(params["id"]), after));
    }),
  );
  return { changes };
}

/** The page's main region, where the profile is. */
function main() {
  return screen.getByRole("main");
}

describe("a profile", () => {
  test("shows the account's header, its counts and its posts", async () => {
    bobServer(
      {},
      {
        note: "<p>I bake bread.</p>",
        fields: [{ name: "Site", value: '<a href="https://bob.example">bob.example</a>' }],
      },
    );
    await renderApp({ path: "/@bob" });

    expect(await screen.findByRole("heading", { level: 1, name: "Bob" })).toBeInTheDocument();
    // The name is the user's text in any script (web-client/README.md Y10).
    expect(within(screen.getByRole("heading", { level: 1 })).getByText("Bob")).toHaveAttribute("dir", "auto");
    // The first is the header's; the posts below show it too.
    expect(within(main()).getAllByText("@bob")[0]).toHaveAttribute("dir", "ltr");
    expect(within(main()).getByText("I bake bread.")).toBeInTheDocument();
    expect(within(main()).getByRole("term")).toHaveTextContent("Site");
    expect(within(main()).getByRole("definition")).toHaveTextContent("bob.example");
    expect(within(main()).getByText("Joined on October 1, 2026")).toBeInTheDocument();
    expect(within(main()).getByRole("link", { name: "12 followers" })).toHaveAttribute("href", "/@bob/followers");
    expect(within(main()).getByRole("link", { name: "1 following" })).toHaveAttribute("href", "/@bob/following");

    expect(screen.getByRole("tab", { name: "Posts", selected: true })).toBeInTheDocument();
    const feed = screen.getByRole("feed", { name: "Posts" });
    expect(within(feed).getByRole("article", { description: "Hello from the local timeline!" })).toBeInTheDocument();
  });

  test("draws the custom emoji of the name, the profile text and the fields, read as their shortcodes", async () => {
    const blobcat = {
      shortcode: "blobcat",
      url: "https://my.example/media/proxy/e1/full",
      static_url: "https://my.example/media/proxy/e1/full",
    };
    bobServer(
      {},
      {
        display_name: "Bob :blobcat:",
        note: "<p>I bake :blobcat: bread.</p>",
        fields: [{ name: "Cat :blobcat:", value: "<p>:blobcat:</p>" }],
        emojis: [blobcat],
      },
    );
    await renderApp({ path: "/@bob" });

    const heading = await screen.findByRole("heading", { level: 1, name: "Bob :blobcat:" });
    expect(within(heading).getByRole("img", { name: ":blobcat:" })).toHaveAttribute("src", blobcat.static_url);
    expect(within(within(main()).getByRole("term")).getByRole("img", { name: ":blobcat:" })).toBeInTheDocument();
    expect(within(within(main()).getByRole("definition")).getByRole("img", { name: ":blobcat:" })).toBeInTheDocument();
    expect(within(main()).getByText(/I bake/)).toHaveTextContent("I bake bread.");
  });

  test("tells that the account moved, with a link to the new account", async () => {
    const carol = {
      ...otherAccountFixture,
      id: "0192f0c4-7a8e-7000-8000-0000000000c9",
      username: "carol",
      acct: "carol@new.example",
    };
    bobServer({}, { moved: carol });
    await renderApp({ path: "/@bob" });

    await screen.findByRole("heading", { level: 1, name: "Bob" });
    const note = within(main()).getByText(/moved to/);
    expect(within(note).getByRole("link", { name: "@carol@new.example" })).toHaveAttribute(
      "href",
      "/@carol@new.example",
    );
    // The link to manage the move is for the moved account itself.
    expect(within(main()).queryByRole("link", { name: "Manage the move" })).not.toBeInTheDocument();
  });

  test("shows the viewer's own moved profile with the way to manage the move", async () => {
    const carol = {
      ...otherAccountFixture,
      id: "0192f0c4-7a8e-7000-8000-0000000000c9",
      username: "carol",
      acct: "carol@new.example",
    };
    server.use(
      resumableSession(),
      http.get(`${apiBase}/accounts/verify_credentials`, () => HttpResponse.json({ ...viewerFixture, moved: carol })),
      http.get(`${apiBase}/accounts/lookup`, () =>
        HttpResponse.json({ ...viewerFixture, moved: carol, followers_count: 0, following_count: 0 }),
      ),
    );
    await renderApp({ path: "/@alice" });

    await screen.findByRole("heading", { level: 1, name: "Alice" });
    expect(await within(main()).findByText(/You moved to/)).toBeInTheDocument();
    expect(within(main()).getByRole("link", { name: "Manage the move" })).toHaveAttribute("href", "/settings/move");
    expect(within(main()).queryByRole("button", { name: /Follow/ })).not.toBeInTheDocument();
  });

  test("shows the account's header image and avatar, decorative, with the band in place of an image that fails", async () => {
    const header = "https://my.example/media/header/full.png";
    const avatar = "https://my.example/media/avatar/full.png";
    bobServer({}, { header: imageFixture(header), avatar: imageFixture(avatar) });
    await renderApp({ path: "/@bob" });
    await screen.findByRole("heading", { level: 1, name: "Bob" });

    // Both are decorative (alt=""): the name next to them says whose they are.
    const images = within(main()).getAllByRole("presentation");
    expect(images.map((image) => image.getAttribute("src"))).toEqual(expect.arrayContaining([header, avatar]));
    const headerImage = images.find((image) => image.getAttribute("src") === header);
    if (headerImage === undefined) throw new Error("no header image");
    fireEvent.error(headerImage);
    await waitFor(() => {
      expect(
        within(main())
          .getAllByRole("presentation")
          .map((image) => image.getAttribute("src")),
      ).not.toContain(header);
    });
  });

  test("shows the first letter of the name without an avatar", async () => {
    bobServer();
    await renderApp({ path: "/@bob" });
    await screen.findByRole("heading", { level: 1, name: "Bob" });
    expect(within(main()).queryAllByRole("presentation")).toHaveLength(0);
    expect(within(main()).getAllByText("B").length).toBeGreaterThan(0);
  });

  test("offers to sign in to follow while signed out", async () => {
    bobServer();
    await renderApp({ path: "/@bob" });

    const signIn = await within(await screen.findByRole("main")).findByRole("link", { name: "Sign in to follow" });
    expect(signIn).toHaveAttribute("href", "/login?return_to=%2F%40bob");
    expect(screen.queryByRole("button", { name: "Follow" })).not.toBeInTheDocument();
  });

  test("is not found for a handle that names no account", async () => {
    await renderApp({ path: "/@nobody" });

    expect(await screen.findByRole("heading", { level: 1, name: "Account not found" })).toBeInTheDocument();
  });

  test("shows the viewer's own profile with a link to edit it, and marks it in the navigation", async () => {
    server.use(resumableSession());
    await renderApp({ path: "/@alice" });

    expect(await within(await screen.findByRole("main")).findByRole("link", { name: "Edit profile" })).toHaveAttribute(
      "href",
      "/settings/profile",
    );
    expect(screen.queryByRole("button", { name: "Follow" })).not.toBeInTheDocument();
    expect(
      within(screen.getByRole("navigation", { name: "Main" })).getByRole("link", { name: "Profile" }),
    ).toHaveAttribute("aria-current", "page");
  });

  test("opens from the navigation's Profile at the viewer's own address", async () => {
    server.use(resumableSession());
    const { router } = await renderApp({ path: "/profile" });

    expect(await screen.findByRole("heading", { level: 1, name: viewerFixture.display_name })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/@alice");
  });

  test("switches between its tabs, keeping the tab in the address", async () => {
    const queries: string[] = [];
    server.use(
      http.get(`${apiBase}/accounts/:id/statuses`, ({ request }) => {
        queries.push(new URL(request.url).searchParams.toString());
        return HttpResponse.json(statusPage([timelineStatusFixture]));
      }),
    );
    bobServer();
    const { user, router } = await renderApp({ path: "/@bob" });

    await screen.findByRole("feed", { name: "Posts" });
    await user.click(screen.getByRole("tab", { name: "Posts and replies" }));

    expect(await screen.findByRole("feed", { name: "Posts and replies" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Posts and replies", selected: true })).toBeInTheDocument();
    expect(router.state.location.search).toEqual({ tab: "replies" });

    await user.click(screen.getByRole("tab", { name: "Without boosts" }));
    expect(await screen.findByRole("feed", { name: "Without boosts" })).toBeInTheDocument();
    // The posts tab also reads the pinned posts (FR-403); the other tabs do not.
    expect(queries.toSorted()).toEqual([
      "",
      "exclude_replies=true",
      "exclude_replies=true&exclude_boosts=true",
      "pinned=true",
    ]);
  });
});

describe("the follow button", () => {
  test("follows an account without a relationship", async () => {
    server.use(resumableSession());
    const { changes } = bobServer();
    const { user } = await renderApp({ path: "/@bob" });

    await user.click(await screen.findByRole("button", { name: "Follow" }));

    expect(await screen.findByRole("button", { name: "Following" })).toBeInTheDocument();
    expect(changes).toEqual(["follow"]);
    expect(await screen.findByText(/^You follow \u2068Bob\u2069 now\.$/)).toBeInTheDocument();
  });

  test("sends a follow request to an account that approves its followers", async () => {
    server.use(resumableSession());
    const { changes } = bobServer({}, { locked: true });
    const { user } = await renderApp({ path: "/@bob" });

    expect(await within(await screen.findByRole("main")).findByText("Approves followers")).toBeInTheDocument();
    await user.click(await screen.findByRole("button", { name: "Request to follow" }));

    expect(await screen.findByRole("button", { name: "Requested" })).toBeInTheDocument();
    expect(changes).toEqual(["follow"]);
  });

  test("offers to follow back an account that follows the viewer", async () => {
    server.use(resumableSession());
    bobServer({ followed_by: true });
    await renderApp({ path: "/@bob" });

    expect(await screen.findByRole("button", { name: "Follow back" })).toBeInTheDocument();
    expect(within(main()).getByText("Follows you")).toBeInTheDocument();
  });

  test("asks before unfollowing, and changes nothing when cancelled", async () => {
    server.use(resumableSession());
    const { changes } = bobServer({ following: true });
    const { user } = await renderApp({ path: "/@bob" });

    const following = await screen.findByRole("button", { name: "Following" });
    await user.click(following);
    const dialog = await screen.findByRole("alertdialog", { name: "Unfollow \u2068Bob\u2069?" });
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    // React Aria restores the focus to the trigger once the dialog has gone.
    await waitFor(() => {
      expect(following).toHaveFocus();
    });
    expect(changes).toEqual([]);

    await user.click(following);
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Unfollow" }));

    expect(await screen.findByRole("button", { name: "Follow" })).toBeInTheDocument();
    expect(changes).toEqual(["unfollow"]);
  });

  test("withdraws a pending follow request after asking", async () => {
    server.use(resumableSession());
    const { changes } = bobServer({ requested: true });
    const { user } = await renderApp({ path: "/@bob" });

    await user.click(await screen.findByRole("button", { name: "Requested" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Withdraw your follow request to \u2068Bob\u2069?" });
    await user.click(within(dialog).getByRole("button", { name: "Withdraw request" }));

    expect(await screen.findByRole("button", { name: "Follow" })).toBeInTheDocument();
    expect(changes).toEqual(["unfollow"]);
  });

  test("shows a block, and unblocks after asking", async () => {
    server.use(resumableSession());
    const { changes } = bobServer({ blocking: true });
    const { user } = await renderApp({ path: "/@bob" });

    await user.click(await screen.findByRole("button", { name: "Blocked" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Unblock \u2068Bob\u2069?" });
    await user.click(within(dialog).getByRole("button", { name: "Unblock" }));

    expect(await screen.findByRole("button", { name: "Follow" })).toBeInTheDocument();
    expect(changes).toEqual(["unblock"]);
  });

  test("keeps the relationship and says why when a change fails", async () => {
    server.use(resumableSession());
    bobServer();
    server.use(http.post(`${apiBase}/accounts/:id/follow`, () => problem(503, "about:blank", "Service Unavailable")));
    const { user } = await renderApp({ path: "/@bob" });

    await user.click(await screen.findByRole("button", { name: "Follow" }));

    expect(
      await screen.findAllByText("Could not change your relationship with the account: Service Unavailable"),
    ).not.toHaveLength(0);
    expect(screen.getByRole("button", { name: "Follow" })).toBeInTheDocument();
  });
});

describe("the menu of a profile", () => {
  test("mutes and unmutes the account", async () => {
    server.use(resumableSession());
    const { changes } = bobServer();
    const { user } = await renderApp({ path: "/@bob" });

    await user.click(await screen.findByRole("button", { name: "More actions for \u2068Bob\u2069" }));
    await user.click(within(await screen.findByRole("menu")).getByRole("menuitem", { name: "Mute" }));

    expect(await within(main()).findByText("Muted")).toBeInTheDocument();
    expect(changes).toEqual(["mute"]);
    await user.click(screen.getByRole("button", { name: "More actions for \u2068Bob\u2069" }));
    expect(within(await screen.findByRole("menu")).getByRole("menuitem", { name: "Unmute" })).toBeInTheDocument();
  });

  test("blocks the account after asking", async () => {
    server.use(resumableSession());
    const { changes } = bobServer({ following: true });
    const { user } = await renderApp({ path: "/@bob" });

    await user.click(await screen.findByRole("button", { name: "More actions for \u2068Bob\u2069" }));
    await user.click(within(await screen.findByRole("menu")).getByRole("menuitem", { name: "Block" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Block \u2068Bob\u2069?" });
    await user.click(within(dialog).getByRole("button", { name: "Block" }));

    expect(await screen.findByRole("button", { name: "Blocked" })).toBeInTheDocument();
    expect(changes).toEqual(["block"]);
    // The menu's item is gone with the menu, so the focus goes back to the menu button.
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "More actions for \u2068Bob\u2069" })).toHaveFocus();
    });
  });
});

describe("the bell of a profile", () => {
  /** Carol of another server, with the viewer's relationship with her; returns the changes the server received. */
  function carolServer(relationship: Partial<Relationship> = {}) {
    const changes: string[] = [];
    server.use(
      http.get(`${apiBase}/accounts/relationships`, () =>
        HttpResponse.json([relationshipFixture(remoteAccountFixture.id, relationship)]),
      ),
      http.post(`${apiBase}/accounts/:id/:action`, ({ params }) => {
        const action = String(params["action"]);
        changes.push(action);
        return HttpResponse.json(
          relationshipFixture(String(params["id"]), { ...relationship, notify: action === "subscribe" }),
        );
      }),
    );
    return { changes };
  }

  test("turns the notifications of a local account's new posts on and off at once, without a follow", async () => {
    server.use(resumableSession());
    const { changes } = bobServer();
    const { user } = await renderApp({ path: "/@bob" });

    const bell = await screen.findByRole("button", { name: "Notify me of new posts" });
    expect(bell).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("button", { name: "Follow" })).toBeInTheDocument();

    await user.click(bell);

    await waitFor(() => {
      expect(bell).toHaveAttribute("aria-pressed", "true");
    });
    expect(changes).toEqual(["subscribe"]);
    expect(await screen.findByText(/^You will be notified of the new posts of ⁨Bob⁩\.$/)).toBeInTheDocument();
    // The focus stays on the bell, whose name does not change.
    expect(bell).toHaveFocus();

    await user.click(bell);

    await waitFor(() => {
      expect(bell).toHaveAttribute("aria-pressed", "false");
    });
    expect(changes).toEqual(["subscribe", "unsubscribe"]);
  });

  test("cannot be pressed on an account of another server that the viewer does not follow, and says why", async () => {
    server.use(resumableSession());
    const { changes } = carolServer();
    await renderApp({ path: "/@carol@remote.example" });

    const bell = await screen.findByRole("button", { name: "Notify me of new posts" });
    expect(bell).toBeDisabled();
    expect(bell).toHaveAttribute("aria-pressed", "false");
    expect(bell).toHaveAccessibleDescription("Follow this account to be notified of its new posts.");
    expect(within(main()).getByText("Follow this account to be notified of its new posts.")).toBeVisible();
    expect(changes).toEqual([]);
  });

  test("turns on for an account of another server that the viewer follows", async () => {
    server.use(resumableSession());
    const { changes } = carolServer({ following: true });
    const { user } = await renderApp({ path: "/@carol@remote.example" });

    const bell = await screen.findByRole("button", { name: "Notify me of new posts" });
    expect(bell).toBeEnabled();
    expect(within(main()).queryByText("Follow this account to be notified of its new posts.")).not.toBeInTheDocument();

    await user.click(bell);

    await waitFor(() => {
      expect(bell).toHaveAttribute("aria-pressed", "true");
    });
    expect(changes).toEqual(["subscribe"]);
  });

  test("is not shown while the viewer blocks the account", async () => {
    server.use(resumableSession());
    bobServer({ blocking: true });
    await renderApp({ path: "/@bob" });

    expect(await screen.findByRole("button", { name: "Blocked" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Notify me of new posts" })).not.toBeInTheDocument();
  });
});

describe("the follow lists", () => {
  test("list an account's followers, linked to their profiles, and load the next page", async () => {
    const cursors: (string | null)[] = [];
    bobServer();
    server.use(
      http.get(`${apiBase}/accounts/:id/followers`, ({ request }) => {
        const cursor = new URL(request.url).searchParams.get("cursor");
        cursors.push(cursor);
        return HttpResponse.json(
          cursor === null
            ? { accounts: [viewerFixture], next_cursor: "page-2" }
            : {
                accounts: [
                  {
                    ...viewerFixture,
                    id: "0192f0c4-7a8e-7000-8000-000000000003",
                    username: "carol",
                    acct: "carol@remote.example",
                    display_name: "",
                  },
                ],
              },
        );
      }),
    );
    const { user } = await renderApp({ path: "/@bob/followers" });

    expect(await screen.findByRole("heading", { level: 1, name: "Followers of \u2068Bob\u2069" })).toBeInTheDocument();
    expect(within(main()).getByRole("link", { name: "Alice @alice" })).toHaveAttribute("href", "/@alice");
    expect(within(main()).getByRole("link", { name: "Back to the profile of \u2068Bob\u2069" })).toHaveAttribute(
      "href",
      "/@bob",
    );

    await user.click(screen.getByRole("button", { name: "Show more" }));

    expect(await within(main()).findByRole("link", { name: "carol @carol@remote.example" })).toHaveAttribute(
      "href",
      "/@carol@remote.example",
    );
    expect(cursors).toEqual([null, "page-2"]);
    expect(screen.queryByRole("button", { name: "Show more" })).not.toBeInTheDocument();
  });

  test("show the heading and a skeleton of rows while the first page loads", async () => {
    bobServer();
    let release = () => {};
    const released = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.get(`${apiBase}/accounts/:id/followers`, async () => {
        await released;
        return HttpResponse.json({ accounts: [viewerFixture] });
      }),
    );
    const { user } = await renderApp({ path: "/@bob" });
    await user.click(await within(await screen.findByRole("main")).findByRole("link", { name: "12 followers" }));

    // After the router's pending delay, the heading names the account by the handle in the address.
    await findLoadingRegion("Loading accounts");
    expect(screen.getByRole("heading", { level: 1, name: "Followers of ⁨@bob⁩" })).toBeInTheDocument();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    release();

    expect(await screen.findByRole("heading", { level: 1, name: "Followers of ⁨Bob⁩" })).toBeInTheDocument();
    expect(within(main()).getByRole("link", { name: "Alice @alice" })).toBeInTheDocument();
    expect(screen.queryByText("Loading accounts")).not.toBeInTheDocument();
  });

  test("say who each account is and follow it from its row, asking for every row's relationship at once", async () => {
    server.use(resumableSession());
    bobServer();
    const carol = {
      ...viewerFixture,
      id: "0192f0c4-7a8e-7000-8000-000000000003",
      username: "carol",
      acct: "carol",
      display_name: "Carol",
      note: "<p>Grows tomatoes.</p><p>Bakes bread.</p>",
    };
    const dave = { ...carol, id: "0192f0c4-7a8e-7000-8000-000000000004", username: "dave", acct: "dave", note: "" };
    const requested: string[][] = [];
    const follows: string[] = [];
    server.use(
      http.get(`${apiBase}/accounts/:id/followers`, () =>
        HttpResponse.json({ accounts: [viewerFixture, carol, dave] }),
      ),
      http.get(`${apiBase}/accounts/relationships`, ({ request }) => {
        const ids = new URL(request.url).searchParams.getAll("id");
        requested.push(ids);
        return HttpResponse.json(
          ids.map((id) => relationshipFixture(id, id === carol.id ? { followed_by: true } : {})),
        );
      }),
      http.post(`${apiBase}/accounts/:id/follow`, ({ params }) => {
        follows.push(String(params["id"]));
        return HttpResponse.json(relationshipFixture(String(params["id"]), { following: true }));
      }),
    );
    const { user } = await renderApp({ path: "/@bob/followers" });
    await screen.findByRole("heading", { level: 1, name: "Followers of \u2068Bob\u2069" });

    // eslint-disable-next-line testing-library/no-node-access -- the row of a link is its list item.
    const carolRow = within(main()).getByRole("link", { name: "Carol @carol" }).closest("li");
    if (carolRow === null) throw new Error("no row of Carol");
    expect(within(carolRow).getByText("Grows tomatoes. Bakes bread.")).toBeInTheDocument();
    expect(await within(carolRow).findByText("Follows you")).toBeInTheDocument();
    // The viewer's own row has no button.
    // eslint-disable-next-line testing-library/no-node-access -- the row of a link is its list item.
    const ownRow = within(main()).getByRole("link", { name: "Alice @alice" }).closest("li");
    if (ownRow === null) throw new Error("no row of the viewer");
    expect(within(ownRow).queryByRole("button")).not.toBeInTheDocument();
    expect(requested.filter((ids) => ids.includes(carol.id))).toEqual([[carol.id, dave.id]]);

    await user.click(within(carolRow).getByRole("button", { name: "Follow back" }));

    expect(await within(carolRow).findByRole("button", { name: "Following" })).toBeInTheDocument();
    expect(follows).toEqual([carol.id]);
  });

  test("say so when an account follows nobody", async () => {
    bobServer();
    server.use(http.get(`${apiBase}/accounts/:id/following`, () => HttpResponse.json({ accounts: [] })));
    await renderApp({ path: "/@bob/following" });

    expect(
      await screen.findByRole("heading", { level: 1, name: "Accounts \u2068Bob\u2069 follows" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 2, name: "Not following anyone yet" })).toBeInTheDocument();
  });
});

describe("a profile of another server", () => {
  test("shows the full handle, links to the profile on its server, and shows the account's posts", async () => {
    const { user } = await renderApp({ path: "/@carol@remote.example" });

    expect(await screen.findByRole("heading", { level: 1, name: "Carol" })).toBeInTheDocument();
    expect(within(main()).getAllByText("@carol@remote.example")[0]).toHaveAttribute("dir", "ltr");
    // The host inside the sentence is isolated, as names are (web-client/README.md Y10); the link leaves the app.
    expect(within(main()).getByRole("link", { name: newTabName("See on ⁨remote.example⁩") })).toHaveAttribute(
      "href",
      "https://remote.example/@carol",
    );

    const feed = await screen.findByRole("feed", { name: "Posts" });
    const article = within(feed).getByRole("article", { name: /^Carol @carol@remote\.example / });
    // Signed out: no actions, but the link to the post on its server.
    expect(
      within(article).getByRole("link", { name: "Open the original post on remote.example (opens in a new tab)" }),
    ).toHaveAttribute("href", "https://remote.example/@carol/1");
    expect(within(article).getByText("Cats")).toBeVisible();
    await user.click(within(article).getByRole("button", { name: "Show content", expanded: false }));
    expect(within(article).getByText("Hello from another server!")).toBeVisible();
    expect(within(article).getByRole("img", { name: "No description" })).toBeInTheDocument();
  });

  test("is not offered for an account of this instance", async () => {
    bobServer();
    await renderApp({ path: "/@bob" });

    // Neither on the profile nor on its post, and no notice above the posts.
    expect(await screen.findByRole("article", { description: "Hello from the local timeline!" })).toBeInTheDocument();
    expect(within(main()).queryByRole("link", { name: /^(Open the original|See on)/ })).not.toBeInTheDocument();
    expect(within(main()).queryByText("Only the posts this server has received are shown.")).not.toBeInTheDocument();
  });
});

describe("a timeline's post", () => {
  test("links its author's name and, for the pointer, picture to the author's profile", async () => {
    await renderApp({ path: "/local" });

    const article = await screen.findByRole("article");
    const links = within(article).getAllByRole("link", { name: /^Bob/ });
    expect(links.map((link) => link.getAttribute("href"))).toEqual(["/@bob"]);
    // The picture's link is hidden from screen readers, which reach the profile by the name.
    const [avatar] = within(article).getAllByRole("link", { hidden: true });
    if (avatar === undefined) throw new Error("want the picture's link");
    expect(isInaccessible(avatar)).toBe(true);
    expect(avatar).toHaveAttribute("href", "/@bob");
  });
});

describe("the badges of a profile", () => {
  const badges = [
    { role_id: "0192f0c4-7a8e-7000-8000-0000000000b1", label: "Staff", color: "purple" },
    { role_id: "0192f0c4-7a8e-7000-8000-0000000000b2", label: "داعم", color: "green" },
  ] as const;

  test("are a named list next to the name, drawn as the staff wrote them, and not on the account's posts", async () => {
    bobServer({}, { badges: [...badges] });
    // Even an author that carried badges would show none on its posts (ADR-0071 §3); the server sends none there.
    server.use(
      http.get(`${apiBase}/accounts/:id/statuses`, ({ request }) =>
        HttpResponse.json(
          statusPage(
            new URL(request.url).searchParams.get("pinned") === "true"
              ? []
              : [{ ...timelineStatusFixture, account: { ...bob, badges: [...badges] } }],
          ),
        ),
      ),
    );
    await renderApp({ path: "/@bob" });

    const list = await screen.findByRole("list", { name: "Badges" });
    expect(
      within(list)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(["Staff", "داعم"]);
    // The label is the staff's text in any script and no recorded language (web-client/README.md Y10).
    expect(within(list).getByText("داعم")).toHaveAttribute("dir", "auto");
    expect(within(list).getByText("داعم")).toHaveAttribute("lang", "");
    const feed = await screen.findByRole("feed", { name: "Posts" });
    expect(await within(feed).findAllByRole("article")).not.toHaveLength(0);
    expect(within(feed).queryByText("Staff")).not.toBeInTheDocument();
  });

  test("are not shown when the account shows none", async () => {
    bobServer();
    await renderApp({ path: "/@bob" });

    expect(await screen.findByRole("heading", { level: 1, name: "Bob" })).toBeInTheDocument();
    expect(screen.queryByRole("list", { name: "Badges" })).not.toBeInTheDocument();
  });
});
