// SPDX-License-Identifier: AGPL-3.0-only

// Apps, the settings' entry of the apps that can use an account (#3206): the apps the account authorized, each with
// where it comes from and when it was last used (ADR-0090 §5), and the apps it registered itself, each with its type
// and the day it was registered (§1); what each list says in its place when it is empty; the one place that adds an
// app; and what the reader's own apps say on a server whose administrators turned registration off. The screens under
// the entry have their own tests beside this one, and what needs the routes is the app's (web/app,
// settings-apps.dom.test.tsx). That a row's facts stand one a line is the look of the design system's list row, which
// a layout test decides.

import { describe, expect, test } from "bun:test";
import { settingsFindings } from "@my/design-system/test/settings";
import { screen, waitFor, within } from "@testing-library/react";
import { appAt, authorizedApps, before, loading, notLoaded, ownApps, registrationOff } from "../../test/apps";
import { atRest, dayOf, hourOf, inOrder, row, rows, section, sections, words } from "../../test/security";
import { settingsOn, type Access, type Given } from "../../test/settings";
import { SettingsEntryScreen } from "../screen";

/** Apps drawn on a server whose account stands as `access` says, before its reads answered. */
const drawn = (access: Access = {}, given: Given = {}) =>
  settingsOn(access, (address) => <SettingsEntryScreen entry="apps" languages={["en", "ja"]} {...address} />, given);

/** Apps with its sections drawn: every read answered. */
async function read(access: Access = {}, given: Given = {}) {
  const on = await drawn(access, given);
  await screen.findByRole("region", { name: /^Authorized apps/ });
  await atRest(on);
  return on;
}

/** The two lists the entry shows, each of which it waits for. */
const reads = ["GET /authorized_apps", "GET /oauth_apps"] as const;

const paths = {
  authorized: (id: string) => `/settings/apps/authorized/${id}`,
  own: (id: string) => `/settings/apps/${id}`,
  register: "/settings/apps/new",
};

/** The links of `part`, each as its words and where it leads. */
const links = (part: HTMLElement) =>
  within(part)
    .queryAllByRole("link")
    .map((link) => [words(link), link.getAttribute("href")]);

/** The ways to register an app that `part` offers. */
const registers = (part: HTMLElement) => links(part).filter(([name]) => name === "Register an app");

const aboutAuthorized = "Apps you allowed to use your account. Revoke one you no longer use or do not recognize.";
const aboutOwn = "Apps you registered for a bot, a script or a client of your own.";
const noneAuthorized = "No app can use your account now. An app shows here once you allow it to use your account.";
const noneOwn = "You have not registered an app yet. Register one to get its client ID and tokens.";

describe("until the entry is read", () => {
  for (const route of reads)
    test(`it says that it loads the settings while ${route} has no answer, and shows them once it has`, async () => {
      let held: { release: () => void } | undefined;
      const { data } = await drawn(
        {},
        {
          before: ({ fake }) => {
            held = fake.script(route, { hold: "request" });
          },
        },
      );
      // The other list answered, and this one alone keeps the screen waiting.
      await waitFor(() => {
        if (data.queryClient.isFetching() !== 1)
          throw new Error(`test: ${String(data.queryClient.isFetching())} reads are on their way`);
      });
      expect(screen.getByRole("status").textContent).toBe(loading);
      expect(sections()).toEqual([]);

      held?.release();

      await screen.findByRole("region", { name: /^Authorized apps/ });
      expect(screen.queryAllByRole("status").map(words)).not.toContain(loading);
    });

  for (const route of reads)
    test(`it says that the settings could not be loaded when ${route} fails, and Try again reads them`, async () => {
      const { user } = await drawn(
        {},
        {
          before: ({ fake }) => {
            fake.script(route, { fail: 500, times: 1 });
          },
        },
      );

      const failed = await screen.findByRole("alert");
      expect(words(failed)).toMatch(notLoaded);
      expect(sections()).toEqual([]);

      await user.click(within(failed).getByRole("button", { name: "Try again" }));

      await screen.findByRole("region", { name: /^Authorized apps/ });
      expect(screen.queryAllByRole("alert")).toHaveLength(0);
      expect(sections()).toEqual(["Authorized apps 4", "Your apps 3"]);
    });
});

test("a typical account's entry has its two sections, each with its count and the line about it", async () => {
  await read();

  expect(sections()).toEqual(["Authorized apps 4", "Your apps 3"]);
  expect(words(section("Authorized apps"))).toContain(aboutAuthorized);
  expect(words(section("Your apps"))).toContain(aboutOwn);
});

describe("as it stands, the entry keeps the rules of a settings screen (ADR-0110 §6)", () => {
  for (const [what, access] of [
    ["with apps in both lists", {}],
    ["with no app in either list", { authorizedApps: "none", oauthApps: "none" }],
    ["on a server that does not let users register apps", { oauthApps: "disabled" }],
  ] as const)
    test(what, async () => {
      await read(access);

      // It drew the sections whose controls the check counts.
      expect(sections().map((title) => title.replace(/ \d+$/, ""))).toEqual(["Authorized apps", "Your apps"]);
      expect(settingsFindings(document.body)).toEqual([]);
    });
});

describe("Authorized apps", () => {
  test("each row says the app's name, where it comes from, and when it was last used or that no use is recorded, and leads to the app's screen", async () => {
    const { fake } = await read();
    const apps = await authorizedApps(fake);
    const [web, selfRegistered, byDocument, byUser] = [
      appAt(apps, 0),
      appAt(apps, 1),
      appAt(apps, 2),
      appAt(apps, 3),
    ] as const;

    expect(apps).toHaveLength(4);
    expect(byUser.last_used_at).toBeUndefined();
    expect(rows("Authorized apps")).toEqual([
      row(`mycroblog This server's own web app Last used ${hourOf(web.last_used_at ?? "")}`, {
        leads: paths.authorized(web.id),
      }),
      row(
        `Tuskr for Android An app that registered itself, with details it declared on its own Last used ${hourOf(selfRegistered.last_used_at ?? "")}`,
        { leads: paths.authorized(selfRegistered.id) },
      ),
      // The host of its metadata document, which the server fetched: reader.example.
      row(`Feedlet Comes from reader.example Last used ${hourOf(byDocument.last_used_at ?? "")}`, {
        leads: paths.authorized(byDocument.id),
      }),
      row("Morning bot An app registered by @hana No use recorded", { leads: paths.authorized(byUser.id) }),
    ]);
  });

  test("each fact of a row is a part of its own: the origin and the last use are told apart", async () => {
    await read();

    const list = within(screen.getByRole("list", { name: "Authorized apps" }));
    const feedlet = list.getAllByRole("listitem").filter((item) => words(item).startsWith("Feedlet"));
    expect(feedlet).toHaveLength(1);
    for (const item of feedlet) {
      expect(within(item).getAllByText("Feedlet")).toHaveLength(1);
      expect(within(item).getAllByText("Comes from reader.example")).toHaveLength(1);
      expect(within(item).getAllByText(/^Last used /)).toHaveLength(1);
    }
  });

  test("an app last used in another year says the year", async () => {
    const then = "2024-03-05T12:00:00Z";
    await read(
      {},
      {
        before: ({ fake }) => {
          fake.script("GET /authorized_apps", {
            change: (body) => ({
              ...body,
              items: body.items.slice(1, 2).map((app) => ({ ...app, last_used_at: then })),
            }),
          });
        },
      },
    );

    expect(hourOf(then)).toContain("Mar 5, 2024");
    expect(rows("Authorized apps").map((r) => r.says)).toEqual([
      `Tuskr for Android An app that registered itself, with details it declared on its own Last used ${hourOf(then)}`,
    ]);
    expect(sections()).toContain("Authorized apps 1");
  });

  test("without an authorized app the section says so in the list's place, with no count and no way to add one", async () => {
    await read({ authorizedApps: "none" });

    expect(sections()).toEqual(["Authorized apps", "Your apps 3"]);
    expect(rows("Authorized apps")).toEqual([]);
    expect(words(section("Authorized apps"))).toMatch(inOrder("Authorized apps", aboutAuthorized, noneAuthorized));
    // An app is authorized from the app, never from here.
    expect(links(section("Authorized apps"))).toEqual([]);
    expect(within(section("Authorized apps")).queryAllByRole("button")).toHaveLength(0);
  });
});

describe("Your apps", () => {
  test("each row says the app's name, its type and the day it was registered, a disabled one that it is disabled at its end, and leads to the app's screen", async () => {
    const { fake } = await read();
    const apps = await ownApps(fake);
    const [confidential, open, disabled] = [appAt(apps, 0), appAt(apps, 1), appAt(apps, 2)] as const;

    expect(apps).toHaveLength(3);
    expect(rows("Your apps")).toEqual([
      row(`Morning bot Confidential Registered ${dayOf(confidential.created_at)}`, {
        leads: paths.own(confidential.id),
      }),
      row(`Home dashboard Public Registered ${dayOf(open.created_at)}`, { leads: paths.own(open.id) }),
      row(`Old importer Confidential Registered ${dayOf(disabled.created_at)} Disabled`, {
        leads: paths.own(disabled.id),
      }),
    ]);
  });

  test("an app registered in another year says the year", async () => {
    await read(
      {},
      {
        before: ({ fake }) => {
          fake.script("GET /oauth_apps", {
            change: (body) => ({
              ...body,
              items: body.items.slice(0, 1).map((app) => ({ ...app, created_at: "2024-03-05T12:00:00Z" })),
            }),
          });
        },
      },
    );

    expect(rows("Your apps").map((r) => r.says)).toEqual(["Morning bot Confidential Registered Mar 5, 2024"]);
  });

  test("Register an app stands once on the entry, at the head of this section, and leads to the form", async () => {
    await read();

    expect(registers(section("Your apps"))).toEqual([["Register an app", paths.register]]);
    expect(registers(screen.getByRole("main"))).toHaveLength(1);
    // At the section's head: before the list it adds to.
    const add = within(section("Your apps")).getByRole("link", { name: "Register an app" });
    expect(before(add, screen.getByRole("list", { name: "Your apps" }))).toBe(true);
  });

  test("without an app the section says so in the list's place, then leads to registering one, once, with no count", async () => {
    await read({ oauthApps: "none" });

    expect(sections()).toEqual(["Authorized apps 4", "Your apps"]);
    expect(rows("Your apps")).toEqual([]);
    expect(words(section("Your apps"))).toMatch(inOrder("Your apps", aboutOwn, noneOwn, "Register an app"));
    expect(registers(screen.getByRole("main"))).toEqual([["Register an app", paths.register]]);
  });

  test("where the administrators turned registration off, the section says so from its first read, with no way to register, no sentence of an empty list, and nothing sent to find out", async () => {
    const { requests } = await read({ oauthApps: "disabled" });

    expect(sections()).toEqual(["Authorized apps 4", "Your apps"]);
    expect(words(section("Your apps"))).toMatch(inOrder("Your apps", aboutOwn, registrationOff));
    expect(registers(screen.getByRole("main"))).toEqual([]);
    expect(within(section("Your apps")).queryAllByRole("button")).toHaveLength(0);
    // The list's own answer says it (`can_create`): no registration was tried.
    expect(requests.filter((request) => !request.startsWith("GET "))).toEqual([]);
  });

  test("where registration was turned off after the reader registered apps, the apps are still listed, each leading to its screen, with the sentence and no way to register", async () => {
    const { fake } = await read(
      {},
      {
        before: (on) => {
          on.fake.script("GET /oauth_apps", { change: (body) => ({ ...body, can_create: false }) });
        },
      },
    );
    const apps = await ownApps(fake);

    expect(sections()).toContain("Your apps 3");
    expect(rows("Your apps").map((r) => r.leads)).toEqual(apps.map((app) => paths.own(app.id)));
    expect(words(section("Your apps"))).toContain(registrationOff);
    expect(registers(screen.getByRole("main"))).toEqual([]);
  });
});
