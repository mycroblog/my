// SPDX-License-Identifier: AGPL-3.0-only

// Issuing a token of one's own app for one's own account, under the app's screen (#3206, ADR-0090 §17): the scopes
// start as the app's, without the permission scopes (`admin:<permission>`) of permissions the reader's account does
// not have, since the server issues those only for a permission the reader holds, so the form waits for the app and
// for the account; an emptied field asks for the scopes it started with; a scope the server refuses is said under
// the field; and the issued pair is shown once, each token with a way to copy it. An app the administrators disabled
// gets no token, and the screen says so in the form's place. Its way back leads to the app's screen, which the route
// decides (web/app, settings-apps.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { settingsFindings } from "@my/design-system/test/settings";
import { screen, waitFor, within } from "@testing-library/react";
import {
  alerts,
  appAt,
  before,
  buttons,
  clipboard,
  loading,
  notAmong,
  notDone,
  notLoaded,
  notShownAgain,
  ownApps,
  said,
  shown,
  write,
} from "../../test/apps";
import { atRest, describedBy, focusOn, inOrder, section, sections, words } from "../../test/security";
import { settingsOn, type Given } from "../../test/settings";
import { AppToken } from "./token";

/** The reader's apps on the fake, by their place in its list. */
const confidential = 0;
const disabled = 2;

/** An ID the server gave no app. */
const unknown = "01900000-0000-7000-8000-000000000000";

/** The token's screen of the reader's app at `index` of the fake's list, or of `unknown`, before its reads answered. */
function drawn(index: number | typeof unknown, given: Given = {}) {
  let id = "";
  return settingsOn({}, (address) => <AppToken id={id} {...address} />, {
    ...given,
    before: (on) => {
      id = typeof index === "number" ? on.fake.appId("own", index) : index;
      given.before?.(on);
    },
  });
}

/** The token's screen of the reader's app at `index`, once it shows its section, and the app as the server holds it. */
async function read(index: number, given: Given = {}) {
  const on = await drawn(index, given);
  await screen.findByRole("region", { name: "Token for your own account" });
  await atRest(on);
  return { ...on, app: appAt(await ownApps(on.fake), index) };
}

/** The field of the scopes the token carries. */
const scopes = () => screen.getByRole<HTMLInputElement>("textbox", { name: "Scopes" });
/** The form's own button. */
const issue = () => screen.getByRole("button", { name: "Issue token" });

/** The app's scopes as the server answers them, with two permission scopes among them. */
const withPermissionScopes = (permissions: readonly ("manage_reports" | "manage_invites")[]): Given => ({
  before: ({ fake }) => {
    fake.script("GET /oauth_apps/{id}", {
      change: (app) => ({ ...app, scope: "read admin:manage_invites admin:manage_reports write:statuses" }),
    });
    fake.script("GET /accounts/verify_credentials", {
      change: (account) => ({ ...account, permissions: [...permissions] }),
    });
  },
});

const hint = "Separated by spaces, within the scopes of the app. An empty field gets the scopes it started with.";
const about =
  "The server issues an access token and a refresh token of this app for your own account, with no page to allow the app and no redirect.";
const onlyHere =
  "Only a token issued here can carry a permission scope such as admin:manage_invites, and only for a permission your account has. An app that asks for one on the page where people allow it is refused.";

// What the field starts with needs both: the app's scopes, and the permissions of the reader's account.
describe("until the app and the reader's account are read", () => {
  /** The two reads the form waits for, and each as the app's data sends it for the app at `id`. */
  const reads = [
    ["the app", "GET /oauth_apps/{id}", (id: string) => `GET /oauth_apps/${id}`],
    ["the reader's account", "GET /accounts/verify_credentials", () => "GET /accounts/verify_credentials"],
  ] as const;

  for (const [what, route] of reads)
    test(`the screen says that it loads the settings while ${what} has no answer, with no field and nothing to send, and shows the form once it has`, async () => {
      let held: { release: () => void } | undefined;
      const { data } = await drawn(confidential, {
        before: ({ fake }) => {
          held = fake.script(route, { hold: "request" });
        },
      });
      // The other read answered, and this one alone keeps the screen waiting.
      await waitFor(() => {
        if (data.queryClient.isFetching() !== 1)
          throw new Error(`test: ${String(data.queryClient.isFetching())} reads are on their way`);
      });

      expect(said()).toEqual([loading]);
      expect(screen.queryAllByRole("textbox")).toHaveLength(0);
      expect(buttons()).not.toContain("Issue token");

      held?.release();

      await screen.findByRole("textbox", { name: "Scopes" });
      expect(said()).toEqual([]);
    });

  for (const [what, route, sent] of reads)
    test(`the screen says that the settings could not be loaded when the read of ${what} fails, with no field, and Try again reads that alone, then shows the form`, async () => {
      const on = await drawn(confidential, {
        before: ({ fake }) => {
          fake.script(route, { fail: 500, times: 1 });
        },
      });
      const { user, fake, requests } = on;

      const failed = await screen.findByRole("alert");
      await atRest(on);
      expect(words(failed)).toMatch(notLoaded);
      expect(screen.queryAllByRole("textbox")).toHaveLength(0);
      expect(buttons()).not.toContain("Issue token");
      const before = requests.length;

      await user.click(within(failed).getByRole("button", { name: "Try again" }));

      await screen.findByRole("textbox", { name: "Scopes" });
      await atRest(on);
      expect(alerts()).toEqual([]);
      expect(requests.slice(before)).toEqual([sent(fake.appId("own", confidential))]);
    });

  // The app's scopes alone would start the field without the permission scopes, which the reader then could not
  // tell from scopes the app lacks.
  test("an account that answers after the app still starts the field with the permission scopes of the permissions it has", async () => {
    let held: { release: () => void } | undefined;
    const { data } = await drawn(confidential, {
      before: ({ fake }) => {
        fake.script("GET /oauth_apps/{id}", {
          change: (app) => ({ ...app, scope: "read admin:manage_invites admin:manage_reports write:statuses" }),
        });
        held = fake.script("GET /accounts/verify_credentials", {
          hold: "request",
          change: (account) => ({ ...account, permissions: ["manage_reports" as const] }),
        });
      },
    });
    await waitFor(() => {
      if (data.queryClient.isFetching() !== 1)
        throw new Error(`test: ${String(data.queryClient.isFetching())} reads are on their way`);
    });
    expect(screen.queryAllByRole("textbox")).toHaveLength(0);

    held?.release();

    expect((await screen.findByRole<HTMLInputElement>("textbox", { name: "Scopes" })).value).toBe(
      "read admin:manage_reports write:statuses",
    );
  });
});

describe("the form", () => {
  test("says what the server issues, takes the scopes with what the field takes, starting as the app's, and keeps the rules of a settings screen (ADR-0110 §6)", async () => {
    const { app } = await read(confidential);

    expect(sections()).toEqual(["Token for your own account"]);
    expect(words(section("Token for your own account"))).toMatch(
      inOrder("Token for your own account", about, "Scopes", hint, onlyHere),
    );
    expect(app.scope).toBe("read write:statuses");
    expect(scopes().value).toBe(app.scope);
    expect(describedBy(scopes())).toEqual([hint]);
    // Its way back is named by the app it leads to.
    expect(buttons()).toEqual(["Morning bot", "Issue token"]);
    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("its way back, named by the app, asks its route for the way back", async () => {
    const { user, address } = await read(confidential);

    await user.click(screen.getByRole("button", { name: "Morning bot" }));

    expect(address.backs).toBe(1);
  });

  // ADR-0090 §17: the server issues a permission scope only for a permission the reader's role has.
  for (const [who, permissions, starts] of [
    ["has neither permission", [], "read write:statuses"],
    ["has one of the two", ["manage_reports"], "read admin:manage_reports write:statuses"],
    ["has both", ["manage_invites", "manage_reports"], "read admin:manage_invites admin:manage_reports write:statuses"],
  ] as const)
    test(`for a reader who ${who}, the scopes start as the app's without the permission scopes the reader's account lacks`, async () => {
      await read(confidential, withPermissionScopes(permissions));

      expect(scopes().value).toBe(starts);
    });
});

describe("Issue token", () => {
  test("sends the scopes of the field, a space apart", async () => {
    const { user, fake, app } = await read(confidential);
    const sent = fake.script("POST /oauth_apps/{id}/tokens");
    await write(user, scopes(), "  read ");

    await user.click(issue());

    await screen.findByRole("region", { name: "The token is issued" });
    expect(sent.seen.map((s) => [s.url.pathname.split("/").at(-2), s.body])).toEqual([[app.id, { scope: "read" }]]);
  });

  // Sent without a scope, the server gives all the app's, the permission scopes of permissions the reader lacks
  // among them, which it then refuses: an emptied field asks for what the field started with.
  test("with the field emptied, sends the scopes the field started with, not the permission scopes the reader's account lacks", async () => {
    const { user, fake } = await read(confidential, withPermissionScopes(["manage_reports"]));
    const sent = fake.script("POST /oauth_apps/{id}/tokens");
    expect(scopes().value).toBe("read admin:manage_reports write:statuses");
    await write(user, scopes(), "");

    await user.click(issue());

    await sent.arrived();
    expect(sent.seen.map((s) => s.body)).toEqual([{ scope: "read admin:manage_reports write:statuses" }]);
  });

  // An app whose scopes are all permission scopes the reader's account lacks: nothing is left to start with.
  test("only where the field started empty too, an empty field sends no scope", async () => {
    const { user, fake } = await read(confidential, {
      before: (on) => {
        on.fake.script("GET /oauth_apps/{id}", { change: (app) => ({ ...app, scope: "admin:manage_invites" }) });
        on.fake.script("GET /accounts/verify_credentials", {
          change: (account) => ({ ...account, permissions: [] }),
        });
      },
    });
    const sent = fake.script("POST /oauth_apps/{id}/tokens");
    expect(scopes().value).toBe("");

    await user.click(issue());

    await sent.arrived();
    expect(sent.seen.map((s) => s.body ?? {})).toEqual([{}]);
  });

  test("a scope the server refuses is said under the field in the server's words, the field takes the focus, the form says before its button that this could not be done, and no token is shown", async () => {
    const { user } = await read(confidential);
    // A scope beyond the app's, which no app has.
    await write(user, scopes(), "admin");

    await user.click(issue());

    await waitFor(() => {
      if (scopes().getAttribute("aria-invalid") !== "true") throw new Error("test: the field is not marked yet");
    });
    await focusOn(scopes());
    expect(describedBy(scopes())).toEqual([hint, "This value is not valid. Check what you entered."]);
    const [refused, ...others] = screen.getAllByRole("alert");
    expect(others).toHaveLength(0);
    expect(refused === undefined ? "" : words(refused)).toBe(notDone);
    expect(refused === undefined ? false : before(refused, issue())).toBe(true);
    expect(scopes().value).toBe("admin");
    expect(sections()).toEqual(["Token for your own account"]);
    expect(words(screen.getByRole("main"))).not.toContain("my_at_");
  });
});

describe("a token, once issued", () => {
  /** A token of the confidential app issued with its scopes, on the screen. */
  async function issued() {
    const on = await read(confidential);
    await on.user.click(issue());
    await screen.findByRole("region", { name: "The token is issued" });
    return on;
  }

  test("is shown in the form's place: its scopes and how long it works, then the access token and the refresh token, each with the words that it is not shown again and its Copy, and Finish", async () => {
    await issued();

    expect(sections()).toEqual(["The token is issued", "Access token", "Refresh token"]);
    expect(words(section("The token is issued"))).toMatch(
      inOrder(
        "The token is issued",
        "Its scopes are read write:statuses. The access token works for 60 minutes. The refresh token then gets a new pair, and works one time.",
      ),
    );
    expect(shown("Access token")).toMatch(/^my_at_[A-Za-z0-9_-]{43}$/);
    expect(shown("Refresh token")).toMatch(/^my_rt_[A-Za-z0-9_-]{43}$/);
    expect(words(section("Access token"))).toContain(notShownAgain);
    expect(words(section("Refresh token"))).toContain(notShownAgain);
    expect(screen.queryAllByRole("textbox")).toHaveLength(0);
    expect(buttons()).toEqual(["Copy access token", "Copy refresh token", "Finish"]);
    expect(said()).toEqual([]);
    expect(alerts()).toEqual([]);
    expect(settingsFindings(document.body)).toEqual([]);
  });

  for (const [button, list] of [
    ["Copy access token", "Access token"],
    ["Copy refresh token", "Refresh token"],
  ] as const)
    test(`${button} puts that token on the clipboard, and its section says so`, async () => {
      const { user } = await issued();

      await user.click(screen.getByRole("button", { name: button }));

      await waitFor(() => {
        if (said().length === 0) throw new Error("test: the screen says nothing yet");
      });
      expect(within(section(list)).getAllByRole("status").map(words)).toEqual(["Copied"]);
      expect(said()).toEqual(["Copied"]);
      expect(await clipboard().readText()).toBe(shown(list));
    });

  test("Finish asks its route for the way back, to the app's screen, and not before", async () => {
    const { user, address } = await issued();
    expect(address.backs).toBe(0);

    await user.click(screen.getByRole("button", { name: "Finish" }));

    expect(address.backs).toBe(1);
  });
});

test("for an app the administrators disabled, the screen says in the form's place that it gets no token: no field, and nothing to send", async () => {
  const { requests } = await read(disabled);

  expect(sections()).toEqual(["Token for your own account"]);
  expect(words(section("Token for your own account"))).toMatch(
    inOrder(
      "Token for your own account",
      "The server's administrators have disabled this app, so it gets no token until they enable it again.",
    ),
  );
  expect(screen.queryAllByRole("textbox")).toHaveLength(0);
  expect(buttons()).toEqual(["Old importer"]);
  expect(requests.filter((request) => !request.startsWith("GET "))).toEqual([]);
  expect(settingsFindings(document.body)).toEqual([]);
});

test("an address that names no app of the reader's says that the app is not among their apps, and leads to the entry", async () => {
  const on = await drawn(unknown);
  const missing = await screen.findByRole("alert");
  await atRest(on);

  expect(words(missing)).toMatch(notAmong);
  expect(within(missing).getByRole("link", { name: "See your apps" }).getAttribute("href")).toBe("/settings/apps");
  expect(screen.queryAllByRole("textbox")).toHaveLength(0);
  // No app to go back to: the way back is the entry's.
  expect(buttons()).toEqual(["Apps"]);
});
