// SPDX-License-Identifier: AGPL-3.0-only

// Invites, the settings' entry of an account's own invites (#3207, ADR-0106 §14, §15, §17, §18): the list as an
// account that may create invites reads it, each invite a row that says its state as itself and leads to its screen;
// the pages after the first, and one that could not be read; the one way to create, the sentence in its place at the server's limit, and the one
// before the last invite left; what an account that may not create invites is told, each cause as its own sentence
// in English and in Japanese (#3116), with its invites still listed; a server that takes no new account; and a
// one-person server, which has no invites and is asked for none (ADR-0152). An invite's screen and the form that
// creates one have their tests beside this one; what needs the routes and the settings' menu is the app's (web/app,
// settings-invites.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import type { OwnInvite } from "@my/app-data";
import { settingsFindings } from "@my/design-system/test/settings";
import { screen, waitFor, within } from "@testing-library/react";
import { before } from "../../test/apps";
import { heldInvites, invitesDrawn, invitesRead, loading, notLoaded } from "../../test/invites";
import { atRest, focused, hourOf, inOrder, row, rows, section, sections, words } from "../../test/security";

const about = "A person signs up with an invite's link, or enters its code on the sign-up page.";
const cannot = "You cannot create invites";
const atLimit = "You have as many usable invites as allowed (5). Revoke one, or wait until one expires or is used up.";

/** Where the list's Create invite leads, once for each time it is offered. */
const creates = () =>
  within(screen.getByRole("main"))
    .queryAllByRole("link", { name: "Create invite" })
    .map((link) => link.getAttribute("href"));

/** A row of the list: the invite's code, then `lines`, and its screen's address. */
const listed = (invite: OwnInvite, lines: string) =>
  row(`${invite.code} ${lines}`, { leads: `/settings/invites/${invite.id}` });

/** What a row says after its invite's code. */
type Lines = readonly ((invite: OwnInvite) => string)[];

/** What the rows of the fake's six invites say, for a reader who may create invites. */
const lines: Lines = [
  (invite) => `Usable until ${hourOf(invite.expires_at)} Used 0 of 1 time`,
  (invite) => `Usable until ${hourOf(invite.expires_at)} Used 1 of 5 times`,
  () => "Revoked Used 0 of 3 times",
  (invite) => `Expired ${hourOf(invite.expires_at)} Used 1 of 1 time`,
  () => "Used up Used 4 of 4 times",
  (invite) => `Not usable while you cannot create invites Used 0 of 2 times Expires ${hourOf(invite.expires_at)}`,
];

/** The same where the two that would work do not, for what `why` says: each then says when it expires. */
const meanwhile = (why: string): Lines => [
  (invite) => `${why} Used 0 of 1 time Expires ${hourOf(invite.expires_at)}`,
  (invite) => `${why} Used 1 of 5 times Expires ${hourOf(invite.expires_at)}`,
  ...lines.slice(2),
];

/** The rows of the fake's invites `held`, each saying what `said` has at its place. */
const listing = (held: readonly OwnInvite[], said: Lines) =>
  held.map((invite, n) => listed(invite, said[n]?.(invite) ?? ""));

describe("until the list is read", () => {
  test("the entry says that it loads the settings, and lists the invites once the server answered", async () => {
    let held: { release: () => void } | undefined;
    await invitesDrawn(
      {},
      {
        before: ({ fake }) => {
          held = fake.script("GET /invites", { hold: "request" });
        },
      },
    );
    expect((await screen.findByRole("status")).textContent).toBe(loading);
    expect(sections()).toEqual([]);

    held?.release();

    await screen.findByRole("list", { name: "Your invites" });
    expect(screen.queryAllByRole("status").map(words)).not.toContain(loading);
  });

  test("the entry says that the settings could not be loaded when the list fails, and Try again reads it", async () => {
    const { user } = await invitesDrawn(
      {},
      {
        before: ({ fake }) => {
          fake.script("GET /invites", { fail: 500, times: 1 });
        },
      },
    );

    const failed = await screen.findByRole("alert");
    expect(words(failed)).toMatch(notLoaded);
    expect(sections()).toEqual([]);

    await user.click(within(failed).getByRole("button", { name: "Try again" }));

    await screen.findByRole("list", { name: "Your invites" });
    expect(screen.queryAllByRole("alert")).toHaveLength(0);
  });
});

describe("for an account that may create invites", () => {
  test("the section counts the invites and says what an invite is for, each row says its invite's code and state as itself and leads to its screen, and Create invite is offered once", async () => {
    const { fake } = await invitesRead();
    const held = await heldInvites(fake);

    expect(held.map((invite) => invite.state)).toEqual([
      "usable",
      "usable",
      "revoked",
      "expired",
      "exhausted",
      "creator_ineligible",
    ]);
    expect(sections()).toEqual(["Your invites 6"]);
    expect(rows("Your invites")).toEqual(listing(held, lines));
    expect(words(section("Your invites"))).toContain(about);
    expect(creates()).toEqual(["/settings/invites/new"]);
    // Three more can be created: the section says nothing of the limit yet.
    expect(words(section("Your invites"))).not.toMatch(/You can create|as many usable invites/);
    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("a time in another year says the year", async () => {
    await invitesRead(
      {},
      {
        before: ({ fake }) => {
          fake.script("GET /invites", {
            change: (body) => ({
              ...body,
              items: body.items
                .filter((invite) => invite.state === "expired")
                .map((invite) => ({ ...invite, expires_at: "2024-03-05T12:00:00Z" })),
            }),
          });
        },
      },
    );

    expect(rows("Your invites").map((shown) => shown.says.split(" ").slice(1).join(" "))).toEqual([
      "Expired Mar 5, 2024, 12 PM Used 1 of 1 time",
    ]);
  });

  test("with more invites than a page, the section counts them only once Show more read the rest", async () => {
    const { fake, user, requests } = await invitesRead({ invites: "many" });
    const held = await heldInvites(fake);
    expect(held).toHaveLength(26);

    // Twenty of them, and how many there are is not known yet.
    expect(sections()).toEqual(["Your invites"]);
    expect(rows("Your invites").map((shown) => shown.leads)).toEqual(
      held.slice(0, 20).map((invite) => `/settings/invites/${invite.id}`),
    );

    await user.click(within(section("Your invites")).getByRole("button", { name: "Show more" }));

    await waitFor(() => {
      if (rows("Your invites").length !== 26) throw new Error("test: the rest is not listed yet");
    });
    expect(sections()).toEqual(["Your invites 26"]);
    expect(rows("Your invites").map((shown) => shown.leads)).toEqual(
      held.map((invite) => `/settings/invites/${invite.id}`),
    );
    expect(within(section("Your invites")).queryAllByRole("button", { name: "Show more" })).toHaveLength(0);
    expect(requests.filter((request) => request.startsWith("GET /invites"))).toHaveLength(2);
    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("when the rest could not be read, the rows stay and the section says so under them; the same button, now Try again, keeps the focus and reads the rest, and the section then counts them", async () => {
    const { fake, user } = await invitesRead({ invites: "many" });
    const held = await heldInvites(fake);
    const asked = fake.script("GET /invites", { fail: 500, times: 1 });
    const notRead = "The rest of this list could not be loaded.";
    const more = within(section("Your invites")).getByRole("button", { name: "Show more" });

    await user.click(more);

    await within(section("Your invites")).findByRole("button", { name: "Try again" });
    // The button the reader pressed, under another name: the focus has nowhere else to go.
    expect(words(more)).toBe("Try again");
    expect(focused()).toBe("button: Try again");
    expect(rows("Your invites").map((shown) => shown.leads)).toEqual(
      held.slice(0, 20).map((invite) => `/settings/invites/${invite.id}`),
    );
    const said = within(section("Your invites")).getByText(notRead);
    expect(before(screen.getByRole("list", { name: "Your invites" }), said)).toBe(true);
    expect(before(said, more)).toBe(true);
    expect(sections()).toEqual(["Your invites"]);
    expect(settingsFindings(document.body)).toEqual([]);

    await user.click(more);

    await waitFor(() => {
      if (rows("Your invites").length !== 26) throw new Error("test: the rest is not listed yet");
    });
    // The same page, asked for a second time.
    const [first, second, ...others] = asked.seen.map((seen) => seen.url.searchParams.get("cursor"));
    expect(others).toEqual([]);
    expect(first).not.toBeNull();
    expect(second).toBe(first ?? "");
    expect(within(section("Your invites")).queryAllByText(notRead)).toHaveLength(0);
    expect(within(section("Your invites")).queryAllByRole("button")).toHaveLength(0);
    expect(sections()).toEqual(["Your invites 26"]);
  });

  test("without an invite the section says so in the list's place and leads to creating one, with no count", async () => {
    await invitesRead({ invites: "none" });

    expect(sections()).toEqual(["Your invites"]);
    expect(rows("Your invites")).toEqual([]);
    expect(words(section("Your invites"))).toMatch(
      inOrder(
        "Your invites",
        about,
        "You have no invites yet. Create one to get a link for the person you invite.",
        "Create invite",
      ),
    );
    expect(creates()).toEqual(["/settings/invites/new"]);
    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("with as many usable invites as the server allows, the section says so and what to do, and offers no way to create", async () => {
    await invitesRead({ invites: "at-cap" });

    expect(sections()).toEqual(["Your invites 9"]);
    expect(within(section("Your invites")).getByText(atLimit)).not.toBeNull();
    expect(creates()).toEqual([]);
    expect(rows("Your invites")).toHaveLength(9);
    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("with one invite left to create, the section says so and how many the server allows, and still offers to create", async () => {
    await invitesRead(
      {},
      {
        before: ({ fake }) => {
          fake.script("GET /invites", { change: (body) => ({ ...body, usable: 4 }) });
        },
      },
    );

    expect(
      within(section("Your invites")).getByText(
        "You can create 1 more invite. You can have 5 usable invites at a time.",
      ),
    ).not.toBeNull();
    expect(creates()).toEqual(["/settings/invites/new"]);
  });
});

// #3116: each cause the server names reads as its own sentence, and a refusal that names none as one that says so.
const causes = [
  [
    "not_permitted",
    "Only some accounts can create invites on this server. Ask its administrators to let you create them.",
    "このサーバーでは、一部のアカウントだけが招待を作れます。作りたいときは、サーバーの管理者に頼んでください。",
  ],
  [
    "not_approved",
    "Your registration is not approved. A moderator needs to approve it before you can create invites.",
    "登録が承認されていません。招待を作るには、モデレーターの承認が必要です。",
  ],
  [
    "restricted",
    "A moderation action keeps your account from creating invites for now.",
    "モデレーションの処分により、いまはこのアカウントで招待を作れません。",
  ],
  ["unknown", "You cannot create invites now.", "いまは招待を作れません。"],
] as const;
const closed = [
  "This server takes no new accounts now, so no invite can be created.",
  "このサーバーはいま新しいアカウントを受け付けていないので、招待を作れません。",
] as const;

describe("for an account that may not create invites", () => {
  for (const [reason, sentence] of causes)
    test(`a summary says why (${reason}), no way to create is offered, and the invites still list, the ones that would work as not usable meanwhile`, async () => {
      const { fake } = await invitesRead({ invites: "not-allowed", invitesRefusal: reason });

      expect(sections()).toEqual([cannot, "Your invites 6"]);
      expect(words(section(cannot))).toMatch(inOrder(cannot, sentence));
      expect(within(section(cannot)).queryAllByRole("link")).toHaveLength(0);
      expect(within(section(cannot)).queryAllByRole("button")).toHaveLength(0);
      expect(creates()).toEqual([]);
      expect(rows("Your invites")).toEqual(
        listing(await heldInvites(fake), meanwhile("Not usable while you cannot create invites")),
      );
      expect(settingsFindings(document.body)).toEqual([]);
    });

  for (const [reason, , sentence] of causes)
    test(`in Japanese, the summary says why (${reason}) as its own sentence`, async () => {
      await invitesDrawn({ invites: "not-allowed", invitesRefusal: reason }, { locale: "ja" });

      expect(words(await screen.findByRole("region", { name: "招待を作れません" }))).toMatch(
        inOrder("招待を作れません", sentence),
      );
    });
});

describe("on a server that takes no new account", () => {
  const server = { registrations: "closed" } as const;

  test("the summary says so, no way to create is offered, and an invite that would work reads as not usable while sign-ups are closed", async () => {
    const { fake } = await invitesRead({}, { server });

    expect(sections()).toEqual([cannot, "Your invites 6"]);
    expect(words(section(cannot))).toMatch(inOrder(cannot, closed[0]));
    expect(creates()).toEqual([]);
    expect(rows("Your invites")).toEqual(
      listing(await heldInvites(fake), meanwhile("Not usable while sign-ups are closed")),
    );
    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("in Japanese, the summary says so as its own sentence", async () => {
    await invitesDrawn({}, { server, locale: "ja" });

    expect(words(await screen.findByRole("region", { name: "招待を作れません" }))).toMatch(
      inOrder("招待を作れません", closed[1]),
    );
  });

  test("an account without an invite is told it has none, and is not led to creating one", async () => {
    await invitesRead({ invites: "none" }, { server });

    expect(sections()).toEqual([cannot, "Your invites"]);
    expect(words(section("Your invites"))).toMatch(inOrder("Your invites", about, "You have no invites."));
    expect(creates()).toEqual([]);
  });
});

// ADR-0152: a one-person server has no invites. The screen is the settings' for an address that names no entry.
test("on a one-person server the entry is no settings page, and the server is asked for no invites", async () => {
  const { data, requests } = await invitesDrawn({}, { server: { singleUser: true, viewer: "hana" } });

  const missing = await screen.findByRole("alert");
  expect(words(missing)).toMatch(/^This settings page does not exist\. Check the address\.\s*See all settings$/);
  expect(within(missing).getByRole("link", { name: "See all settings" }).getAttribute("href")).toBe("/settings");
  await atRest({ data });
  expect(sections()).toEqual([]);
  expect(requests.filter((request) => request.includes("/invites"))).toEqual([]);
});
