// SPDX-License-Identifier: AGPL-3.0-only

// An app the account authorized, under Apps (#3206, ADR-0090 §5): what the screen knows of the app, a fact an item
// (where it comes from by its registration, the site it declared under that, the day it was authorized over its last
// use), and that the server has not verified an app that is not its own (§4); what the app may do, in lists by risk,
// the cautions of a scope under it; and Revoke, which asks first, then takes the app's access away and goes back to
// Apps. Revoking the server's own web client signs every browser out, this one too: the screen says so before,
// sends the one revocation and nothing after it, tells its route once that the session ended, and keeps its
// confirmation open and at work until the app has left. That the entry no longer lists a revoked app, and where the app goes once its
// session ended, is the app's test (web/app, settings-apps.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { settingsFindings } from "@my/design-system/test/settings";
import { screen, waitFor, within } from "@testing-library/react";
import { appAt, authorizedApps, buttons, listed, loading, notAmong, notLoaded } from "../../test/apps";
import {
  atRest,
  buttonsOf,
  closed,
  dayOf,
  dialogs,
  hourOf,
  offered,
  removedElsewhere,
  sections,
  words,
} from "../../test/security";
import { settingsOn, type Given } from "../../test/settings";
import { AuthorizedAppScreen } from "./authorized";

/** The apps the fake's reader authorized, by their place in its list. */
const web = 0;
const selfRegistered = 1;
const byDocument = 2;
const byUser = 3;

/** An ID the server gave no app. */
const unknown = "01900000-0000-7000-8000-000000000000";

/** The screen of the authorized app at `index` of the fake's list, or of `unknown`, before its reads answered. */
function drawn(index: number | typeof unknown, given: Given = {}) {
  let id = "";
  return settingsOn({}, (address) => <AuthorizedAppScreen id={id} {...address} />, {
    ...given,
    before: (on) => {
      id = typeof index === "number" ? on.fake.appId("authorized", index) : index;
      given.before?.(on);
    },
  });
}

/** The screen of the authorized app at `index`, once it shows the app. */
async function read(index: number, given: Given = {}) {
  const on = await drawn(index, given);
  await screen.findByRole("region", { name: "About this app" });
  await atRest(on);
  return { ...on, app: appAt(await authorizedApps(on.fake), index) };
}

/** The button that revokes the app, which the row of its section offers. */
const revoke = () => offered("Revoke access", "Stop this app from using your account", "Revoke");
/** The confirmation that asks before `name` is revoked, once it is open. */
const asking = (name: string) => screen.findByRole("alertdialog", { name: `Revoke ${name}?` });

const notVerified = "This server has not verified this app. Its name is the app's own claim.";
const attention = "Permissions that need your attention";
const changes = "Permissions that change things";
const reads = "Permissions that only read";

describe("until the app is read", () => {
  test("the screen says that it loads the settings while the list of authorized apps has no answer, and shows the app once it has", async () => {
    let held: { release: () => void } | undefined;
    await drawn(selfRegistered, {
      before: ({ fake }) => {
        held = fake.script("GET /authorized_apps", { hold: "request" });
      },
    });

    expect((await screen.findByRole("status")).textContent).toBe(loading);
    expect(sections()).toEqual([]);

    held?.release();

    await screen.findByRole("region", { name: "About this app" });
    expect(screen.queryAllByRole("status").map(words)).not.toContain(loading);
  });

  test("the screen says that the settings could not be loaded when the list fails, and Try again reads it", async () => {
    const { user } = await drawn(selfRegistered, {
      before: ({ fake }) => {
        fake.script("GET /authorized_apps", { fail: 500, times: 1 });
      },
    });

    const failed = await screen.findByRole("alert");
    expect(words(failed)).toMatch(notLoaded);
    expect(sections()).toEqual([]);

    await user.click(within(failed).getByRole("button", { name: "Try again" }));

    await screen.findByRole("region", { name: "About this app" });
    expect(screen.queryAllByRole("alert")).toHaveLength(0);
  });
});

describe("About this app", () => {
  // ADR-0090 §4: only the host of a metadata document and the user who registered an app are verified; the rest is
  // the app's own claim, and is said so.
  for (const [what, index, origin, declared] of [
    ["the server's own web client", web, "This server's own web app", undefined],
    [
      "an app that registered itself",
      selfRegistered,
      "An app that registered itself, with details it declared on its own",
      "https://tuskr.example/",
    ],
    ["an app known by its metadata document", byDocument, "Comes from reader.example", "https://reader.example/"],
    ["an app a user of the server registered", byUser, "An app registered by @hana", undefined],
  ] as const)
    test(`${what}: where it comes from${declared === undefined ? "" : ", the site it declared under that"}, then the day it was authorized over its last use`, async () => {
      const { app } = await read(index);

      expect(listed("About this app")).toEqual([
        declared === undefined ? origin : `${origin} Website it declared: ${declared}`,
        `Authorized ${dayOf(app.authorized_at)} ${
          app.last_used_at === undefined ? "No use recorded" : `Last used ${hourOf(app.last_used_at)}`
        }`,
      ]);
      // The fake's four apps are one of each: the test names the one it means.
      expect(app.website).toBe(declared);
    });

  test("an app that declared no site has no line of it", async () => {
    await read(selfRegistered, {
      before: ({ fake }) => {
        fake.script("GET /authorized_apps", {
          change: (body) => ({
            ...body,
            items: body.items.map((app) => ({
              id: app.id,
              client_id: app.client_id,
              name: app.name,
              registration: app.registration,
              first_party: app.first_party,
              scope: app.scope,
              authorized_at: app.authorized_at,
            })),
          }),
        });
      },
    });

    expect(listed("About this app")).toHaveLength(2);
    expect(listed("About this app").join(" ")).not.toContain("Website");
    expect(listed("About this app").join(" ")).toContain("No use recorded");
  });

  for (const [what, index, said] of [
    ["the server's own web client is not said to be unverified", web, false],
    ["an app that registered itself is said not to be verified by this server", selfRegistered, true],
    ["an app known by its metadata document is said not to be verified by this server", byDocument, true],
    ["an app a user registered is said not to be verified by this server", byUser, true],
  ] as const)
    test(what, async () => {
      await read(index);

      expect(words(screen.getByRole("region", { name: "About this app" })).includes(notVerified)).toBe(said);
    });

  test("the site an app declared is words, never a way out of the app", async () => {
    await read(selfRegistered);

    expect(within(screen.getByRole("region", { name: "About this app" })).queryAllByRole("link")).toHaveLength(0);
  });
});

describe("the permissions", () => {
  test("stand in lists by risk, each counted: what needs attention first, a scope's cautions under it, then what changes things", async () => {
    await read(selfRegistered);

    expect(sections()).toEqual(["About this app", `${attention} 2`, `${changes} 3`, "Revoke access"]);
    expect(listed(attention)).toEqual([
      "Read your account, posts, and timelines Can read your direct messages",
      "Post, edit, delete, boost, and pin posts, vote in polls, and set who may quote a post Can post, edit and delete posts as you",
    ]);
    expect(listed(changes)).toEqual([
      "Add and remove favourites and emoji reactions",
      "Upload images, videos, and audio, and edit their descriptions",
      "Receive push notifications",
    ]);
  });

  test("the broad scope `write` has two cautions under it", async () => {
    await read(web);

    expect(sections()).toEqual(["About this app", `${attention} 2`, `${changes} 2`, "Revoke access"]);
    expect(listed(attention)).toEqual([
      "Read your account, posts, and timelines Can read your direct messages",
      "Post and change things for you Can post, edit and delete posts as you Can change your profile",
    ]);
    expect(listed(changes)).toEqual(["Follow, block, and mute accounts", "Receive push notifications"]);
  });

  test("an app that only reads has one list, of what only reads", async () => {
    await read(byDocument);

    expect(sections()).toEqual(["About this app", `${reads} 2`, "Revoke access"]);
    expect(listed(reads)).toEqual(["Read accounts, your profile, and your settings", "Read posts and timelines"]);
  });

  test("with permissions of every risk, the lists stand in the order attention, changes, reads, whatever the order of the scopes", async () => {
    await read(byDocument, {
      before: ({ fake }) => {
        fake.script("GET /authorized_apps", {
          change: (body) => ({
            ...body,
            items: body.items.map((app) => ({ ...app, scope: "read:statuses push write:statuses" })),
          }),
        });
      },
    });

    expect(sections()).toEqual(["About this app", `${attention} 1`, `${changes} 1`, `${reads} 1`, "Revoke access"]);
    expect(listed(reads)).toEqual(["Read posts and timelines"]);
    expect(listed(changes)).toEqual(["Receive push notifications"]);
  });

  test("the scope `read:favourites` reads the reader's favourites, and only reads", async () => {
    await read(byDocument, {
      before: ({ fake }) => {
        fake.script("GET /authorized_apps", {
          change: (body) => ({ ...body, items: body.items.map((app) => ({ ...app, scope: "read:favourites" })) }),
        });
      },
    });

    expect(sections()).toEqual(["About this app", `${reads} 1`, "Revoke access"]);
    expect(listed(reads)).toEqual(["Read your favourites"]);
  });
});

describe("as it stands, the screen keeps the rules of a settings screen (ADR-0110 §6)", () => {
  for (const [what, index] of [
    ["of the server's own web client", web],
    ["of an app that registered itself", selfRegistered],
    ["of an app known by its metadata document", byDocument],
    ["of an app a user registered", byUser],
  ] as const)
    test(what, async () => {
      await read(index);

      expect(sections()).toContain("Revoke access");
      expect(settingsFindings(document.body)).toEqual([]);
    });
});

describe("Revoke", () => {
  test("asks first, naming the app, and once confirmed the server no longer holds the app and the screen goes back to Apps", async () => {
    const { user, fake, address, app } = await read(selfRegistered);
    const sent = fake.script("DELETE /authorized_apps/{id}");
    expect(words(screen.getByRole("region", { name: "Revoke access" }))).toContain(
      "The app can no longer read or change anything with your account. To use it again, allow it again from the app.",
    );

    await user.click(revoke());

    const dialog = await asking("Tuskr for Android");
    expect(words(dialog)).toContain(
      "The app loses access to your account at once. You can allow it again later from the app.",
    );
    expect(buttonsOf(dialog)).toEqual(["Close", "Cancel", "Revoke"]);
    expect(sent.seen).toHaveLength(0);

    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

    await waitFor(() => {
      if (address.backs === 0) throw new Error("test: the screen did not go back yet");
    });
    expect(address.backs).toBe(1);
    expect(sent.seen.map((s) => s.url.pathname.split("/").at(-1))).toEqual([app.id]);
    expect((await authorizedApps(fake)).map((a) => a.name)).toEqual(["mycroblog", "Feedlet", "Morning bot"]);
    // An app of another maker: the reader's session is as it was.
    expect(address.sessionsEnded).toBe(0);
  });

  test("Cancel sends nothing, leaves the reader on the screen, and gives the focus back to the button", async () => {
    const { user, address, requests } = await read(selfRegistered);
    const button = revoke();
    await user.click(button);
    const dialog = await asking("Tuskr for Android");
    const sent = requests.length;

    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    await closed();
    await waitFor(() => {
      if (document.activeElement !== button) throw new Error("test: the focus is not back on the button yet");
    });
    expect(requests.slice(sent)).toEqual([]);
    expect(address.backs).toBe(0);
  });

  test("a revocation the server refuses stays in the confirmation, which says the server's words, and the screen does not go back", async () => {
    const { user, fake, address } = await read(selfRegistered);
    fake.script("DELETE /authorized_apps/{id}", {
      fail: { type: "about:blank", title: "The server could not revoke the app", status: 500 },
    });
    await user.click(revoke());
    const dialog = await asking("Tuskr for Android");

    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

    const refused = await screen.findByRole("alert");
    expect(words(refused)).toBe("The server could not revoke the app");
    expect((await asking("Tuskr for Android")).contains(refused)).toBe(true);
    expect(address.backs).toBe(0);
    expect(await authorizedApps(fake)).toHaveLength(4);
  });

  // Another browser of the reader's revoked it first: what the reader asked for is done.
  test("a revocation of an app the server no longer holds (404) goes back to Apps, as one the server did", async () => {
    const { user, fake, address, app } = await read(selfRegistered);
    await user.click(revoke());
    const dialog = await asking("Tuskr for Android");
    await removedElsewhere(fake, `/authorized_apps/${app.id}`);

    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

    await waitFor(() => {
      if (address.backs === 0) throw new Error("test: the screen did not go back yet");
    });
    expect(address.backs).toBe(1);
    expect(address.sessionsEnded).toBe(0);
    expect(screen.queryAllByRole("alert")).toHaveLength(0);
  });

  // The app the reader is using: its tokens are the session of every browser the account is signed in to.
  describe("of the server's own web client", () => {
    /** The buttons of the confirmation, each as its name and what it is to a reader meanwhile. */
    const offers = (dialog: HTMLElement) =>
      within(dialog)
        .getAllByRole<HTMLButtonElement>("button")
        .map((button) => [
          button.getAttribute("aria-label") ?? button.textContent,
          button.getAttribute("aria-busy") === "true" ? "busy" : button.disabled ? "disabled" : "to press",
        ]);
    /** The confirmation at work: Revoke is busy, and nothing closes it. */
    const atWork = [
      ["Close", "disabled"],
      ["Cancel", "disabled"],
      ["Revoke", "busy"],
    ];

    test("says before and in its confirmation that every browser is signed out, this one included", async () => {
      const { user, address } = await read(web);
      expect(words(screen.getByRole("region", { name: "Revoke access" }))).toContain(
        "This is the app you are using now. Revoking it signs you out of every browser, this one included.",
      );

      await user.click(revoke());

      const dialog = await asking("mycroblog");
      expect(words(dialog)).toContain(
        "This is the app you are using now. Revoking it signs you out of every browser at once, this one included. You can sign in again afterwards.",
      );
      expect(offers(dialog)).toEqual([
        ["Close", "to press"],
        ["Cancel", "to press"],
        ["Revoke", "to press"],
      ]);
      expect(address.sessionsEnded).toBe(0);
    });

    // Once the server took the revocation the session is over: the screen asks the server for nothing more, not for
    // the list it showed and not to revoke a token that is revoked already, and has no way back to offer. Leaving is
    // the route's, by a load of the page: until then the confirmation stays, at work.
    test("once confirmed, sends the one revocation and nothing after it, tells its route once that the session ended, and keeps its confirmation open and at work, with nothing to press", async () => {
      const on = await read(web);
      const { user, fake, address, app, requests } = on;
      const sent = fake.script("DELETE /authorized_apps/{id}", { hold: "response" });
      await user.click(revoke());
      const dialog = await asking("mycroblog");
      const asked = requests.length;

      await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

      // The server did it, and its answer is on its way.
      await sent.served();
      await waitFor(() => {
        if (within(dialog).getByRole("button", { name: "Revoke" }).getAttribute("aria-busy") !== "true")
          throw new Error("test: Revoke is not busy yet");
      });
      expect(offers(dialog)).toEqual(atWork);
      expect(address.sessionsEnded).toBe(0);

      sent.release();

      await waitFor(() => {
        if (address.sessionsEnded === 0) throw new Error("test: the screen did not say that the session ended yet");
      });
      await atRest(on);
      expect(address.sessionsEnded).toBe(1);
      expect(address.backs).toBe(0);
      expect(requests.slice(asked)).toEqual([`DELETE /authorized_apps/${app.id}`]);
      expect(fake.revocations).toEqual([]);
      expect(dialogs().map((open) => open === dialog)).toEqual([true]);
      expect(offers(dialog)).toEqual(atWork);
    });

    test("a revocation the server answers 404, the web client revoked already, tells its route once that the session ended all the same, and does not go back", async () => {
      const on = await read(web);
      const { user, fake, address, app } = on;
      await user.click(revoke());
      const dialog = await asking("mycroblog");
      await removedElsewhere(fake, `/authorized_apps/${app.id}`);

      await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

      await waitFor(() => {
        if (address.sessionsEnded === 0) throw new Error("test: the screen did not say that the session ended yet");
      });
      await atRest(on);
      expect(address.sessionsEnded).toBe(1);
      expect(address.backs).toBe(0);
    });
  });
});

describe("an address that names no authorized app", () => {
  test("says that the app is not among the reader's apps, leads to the entry, and offers nothing to revoke", async () => {
    const on = await drawn(unknown);
    const missing = await screen.findByRole("alert");
    await atRest(on);

    expect(words(missing)).toMatch(notAmong);
    expect(within(missing).getByRole("link", { name: "See your apps" }).getAttribute("href")).toBe("/settings/apps");
    expect(sections()).toEqual([]);
    expect(buttons()).toEqual(["Apps"]);
    expect(dialogs()).toHaveLength(0);
    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("its way back leads to Apps", async () => {
    const { user, address } = await drawn(unknown);
    await screen.findByRole("alert");

    await user.click(screen.getByRole("button", { name: "Apps" }));

    expect(address.backs).toBe(1);
  });
});
