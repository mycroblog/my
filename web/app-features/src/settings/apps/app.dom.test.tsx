// SPDX-License-Identifier: AGPL-3.0-only

// An app the reader registered, under Apps (#3206, ADR-0090 §1): its client ID to copy; for a confidential app a new
// secret, asked for first and then shown once; the way to issue a token; its settings, saved as a whole (the name,
// the website, the redirect URIs, the scopes), with a refused field said under it and the type as a sentence, since
// it never changes; and deleting it, asked for first. An app the administrators disabled says so, gets no token, and
// can still be changed and deleted. Issuing the token is the screen under this one (token.dom.test.tsx), the form's
// own checks are pinned with the form that registers (register.dom.test.tsx), and what the routes decide is the
// app's test (web/app, settings-apps.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { settingsFindings } from "@my/design-system/test/settings";
import { screen, waitFor, within } from "@testing-library/react";
import {
  alerts,
  appAt,
  before,
  buttons,
  clipboard,
  field,
  hints,
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
import {
  atRest,
  buttonsOf,
  closed,
  describedBy,
  dialogs,
  focused,
  focusOn,
  heldBy,
  offered,
  row,
  rows,
  section,
  sections,
  words,
} from "../../test/security";
import { settingsOn, type Given } from "../../test/settings";
import { OwnApp } from "./app";

/** The reader's apps on the fake, by their place in its list. */
const confidential = 0;
const open = 1;
const disabled = 2;

/** An ID the server gave no app. */
const unknown = "01900000-0000-7000-8000-000000000000";

/** The screen of the reader's app at `index` of the fake's list, or of `unknown`, before its read answered. */
function drawn(index: number | typeof unknown, given: Given = {}) {
  let id = "";
  return settingsOn({}, (address) => <OwnApp id={id} {...address} />, {
    ...given,
    before: (on) => {
      id = typeof index === "number" ? on.fake.appId("own", index) : index;
      given.before?.(on);
    },
  });
}

/** The screen of the reader's app at `index`, once it shows the app, and the app as the server holds it. */
async function read(index: number, given: Given = {}) {
  const on = await drawn(index, given);
  await screen.findByRole("region", { name: "Credentials" });
  await atRest(on);
  return { ...on, app: appAt(await ownApps(on.fake), index) };
}

/** The form's own button. */
const save = () => screen.getByRole("button", { name: "Save changes" });
/** The button that makes a new secret, which the row of the secret offers. */
const newSecret = () => offered("Credentials", "Client secret", "Make a new secret");
/** The button that deletes the app, which the row of its section offers. */
const remove = () => offered("Delete app", "Delete this app for good", "Delete");
/** The confirmation named `name`, once it is open. */
const asking = (name: string) => screen.findByRole("alertdialog", { name });

const secretRow = row("Client secret Shown only when it is made. If you lost it, make a new one.", {
  offers: ["Make a new secret"],
});
const tokenRow = (id: string) =>
  row("Issue token An access token and a refresh token for your own account, with no page to allow the app", {
    leads: `/settings/apps/${id}/token`,
  });
const isDisabled = "The server's administrators have disabled this app";
const stopped =
  "Its tokens have stopped working and will not work again. Until the administrators enable it again, nobody can allow it and it gets no new tokens.";
const typeOf = {
  confidential:
    "This is a confidential app: it has a secret. The type cannot be changed. To use the other type, register a new app.",
  public:
    "This is a public app: it has no secret. The type cannot be changed. To use the other type, register a new app.",
} as const;

describe("until the app is read", () => {
  test("the screen says that it loads the settings while the app has no answer, and shows it once it has", async () => {
    let held: { release: () => void } | undefined;
    await drawn(confidential, {
      before: ({ fake }) => {
        held = fake.script("GET /oauth_apps/{id}", { hold: "request" });
      },
    });

    expect((await screen.findByRole("status")).textContent).toBe(loading);
    expect(sections()).toEqual([]);
    expect(screen.queryAllByRole("textbox")).toHaveLength(0);

    held?.release();

    await screen.findByRole("region", { name: "Credentials" });
    expect(said()).toEqual([]);
  });

  test("the screen says that the settings could not be loaded when the read fails, and Try again reads the app", async () => {
    const { user } = await drawn(confidential, {
      before: ({ fake }) => {
        fake.script("GET /oauth_apps/{id}", { fail: 500, times: 1 });
      },
    });

    const failed = await screen.findByRole("alert");
    expect(words(failed)).toMatch(notLoaded);
    expect(sections()).toEqual([]);

    await user.click(within(failed).getByRole("button", { name: "Try again" }));

    await screen.findByRole("region", { name: "Credentials" });
    expect(alerts()).toEqual([]);
  });
});

describe("a confidential app", () => {
  test("shows its client ID to copy, the row that makes a new secret, the way to issue a token, its settings as the server holds them, and deleting it; and keeps the rules of a settings screen (ADR-0110 §6)", async () => {
    const { app } = await read(confidential);

    expect(sections()).toEqual(["Credentials", "App settings", "Delete app"]);
    expect(shown("Client ID")).toBe(app.client_id);
    expect(rows("Credentials")).toEqual([secretRow, tokenRow(app.id)]);
    expect([field("Name").value, field("Website").value, field("Redirect URIs").value, field("Scopes").value]).toEqual([
      "Morning bot",
      "https://bot.example/",
      "http://127.0.0.1/callback",
      "read write:statuses",
    ]);
    expect(words(section("App settings"))).toContain(
      "Changes apply from the next time someone allows the app. Tokens the app already holds keep the scopes they have.",
    );
    expect(buttons()).toEqual(["Apps", "Copy client ID", "Make a new secret", "Delete", "Save changes"]);
    // The secret is shown when it is made, never here.
    expect(words(screen.getByRole("main"))).not.toContain("my_cs_");
    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("says its type as a sentence: nothing chooses it", async () => {
    await read(confidential);

    expect(words(section("App settings"))).toContain(typeOf.confidential);
    expect(screen.queryAllByRole("radio")).toHaveLength(0);
    expect(screen.queryAllByRole("combobox")).toHaveLength(0);
  });

  test("Copy client ID puts it on the clipboard and says so", async () => {
    const { user, app } = await read(confidential);

    await user.click(screen.getByRole("button", { name: "Copy client ID" }));

    await waitFor(() => {
      if (said().length === 0) throw new Error("test: the screen says nothing yet");
    });
    expect(said()).toEqual(["Copied"]);
    expect(await clipboard().readText()).toBe(app.client_id);
  });
});

describe("a public app", () => {
  test("has no row of a secret, lists its redirect URIs one per line, and says its type as a sentence", async () => {
    const { app } = await read(open);

    expect(shown("Client ID")).toBe(app.client_id);
    expect(rows("Credentials")).toEqual([tokenRow(app.id)]);
    expect(field("Redirect URIs").value).toBe("https://dash.example/callback\nhttp://127.0.0.1:5173/callback");
    expect(field("Website").value).toBe("");
    expect(words(section("App settings"))).toContain(typeOf.public);
    expect(buttons()).toEqual(["Apps", "Copy client ID", "Delete", "Save changes"]);
    expect(settingsFindings(document.body)).toEqual([]);
  });
});

describe("Make a new secret", () => {
  test("asks first, saying what stops working, and sends nothing until the reader confirms", async () => {
    const { user, fake } = await read(confidential);
    const sent = fake.script("POST /oauth_apps/{id}/rotate_secret");

    await user.click(newSecret());

    const dialog = await asking("Make a new secret for Morning bot?");
    expect(words(dialog)).toContain(
      "The secret the app has now stops working at once, and this cannot be undone. The app needs the new secret to renew its tokens. The tokens it holds keep working.",
    );
    expect(buttonsOf(dialog)).toEqual(["Close", "Cancel", "Make a new secret"]);
    expect(sent.seen).toHaveLength(0);

    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    await closed();
    expect(sent.seen).toHaveLength(0);
    expect(sections()).toEqual(["Credentials", "App settings", "Delete app"]);
  });

  /** A new secret made for the confidential app, on the screen. */
  async function made() {
    const on = await read(confidential);
    const sent = on.fake.script("POST /oauth_apps/{id}/rotate_secret");
    await on.user.click(newSecret());
    const dialog = await asking("Make a new secret for Morning bot?");
    await on.user.click(within(dialog).getByRole("button", { name: "Make a new secret" }));
    await screen.findByRole("region", { name: "The new secret is ready" });
    return { ...on, sent };
  }

  test("once confirmed, shows the new secret in the screen's place with the words that it is not shown again, its Copy, and Finish", async () => {
    const { sent, app } = await made();

    expect(dialogs()).toHaveLength(0);
    expect(sections()).toEqual(["The new secret is ready", "Client secret"]);
    expect(words(section("The new secret is ready"))).toContain(
      "The old secret no longer works. The app needs the new one to renew its tokens.",
    );
    expect(shown("Client secret")).toMatch(/^my_cs_[A-Za-z0-9_-]{43}$/);
    expect(words(section("Client secret"))).toContain(notShownAgain);
    expect(buttons()).toEqual(["Copy secret", "Finish"]);
    expect(sent.seen.map((s) => s.url.pathname.split("/").at(-2))).toEqual([app.id]);
    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("Copy secret puts the new secret on the clipboard and says so", async () => {
    const { user } = await made();

    await user.click(screen.getByRole("button", { name: "Copy secret" }));

    await waitFor(() => {
      if (said().length === 0) throw new Error("test: the screen says nothing yet");
    });
    expect(said()).toEqual(["Copied"]);
    expect(await clipboard().readText()).toBe(shown("Client secret"));
  });

  test("Finish leads back to the app's screen, where the secret is no longer shown, and not to Apps", async () => {
    const { user, address } = await made();
    const secret = shown("Client secret");

    await user.click(screen.getByRole("button", { name: "Finish" }));

    await screen.findByRole("region", { name: "Credentials" });
    expect(sections()).toEqual(["Credentials", "App settings", "Delete app"]);
    expect(words(screen.getByRole("main"))).not.toContain(secret);
    expect(address.backs).toBe(0);
  });
});

describe("Save changes", () => {
  test("sends the name, the website, the redirect URIs one per line and the scopes a space apart, and the form says Saved before its button", async () => {
    const { user, fake, app } = await read(confidential);
    const sent = fake.script("PATCH /oauth_apps/{id}");
    await write(user, field("Name"), "Evening bot");
    await write(user, field("Website"), "https://evening.example/");
    await write(user, field("Redirect URIs"), "https://bot.example/callback\n\n http://127.0.0.1/callback ");
    await write(user, field("Scopes"), " read   write:statuses  push ");

    await user.click(save());

    await waitFor(() => {
      if (said().length === 0) throw new Error("test: the screen says nothing yet");
    });
    expect(said()).toEqual(["Saved"]);
    const [status] = screen.getAllByRole("status").filter((place) => words(place) === "Saved");
    expect(status === undefined ? false : before(status, save())).toBe(true);
    expect(sent.seen.map((s) => [s.url.pathname.split("/").at(-1), s.body])).toEqual([
      [
        app.id,
        {
          name: "Evening bot",
          website: "https://evening.example/",
          redirect_uris: ["https://bot.example/callback", "http://127.0.0.1/callback"],
          scope: "read write:statuses push",
        },
      ],
    ]);
    expect((await heldBy<{ name: string }>(fake, `/oauth_apps/${app.id}`)).name).toBe("Evening bot");
    // The reader stays on the app's screen, which shows what was saved, with the focus on the button they pressed.
    expect(field("Name").value).toBe("Evening bot");
    expect(alerts()).toEqual([]);
    expect(focused()).toBe("button: Save changes");
  });

  test("a website emptied is sent empty, which removes it", async () => {
    const { user, fake } = await read(confidential);
    const sent = fake.script("PATCH /oauth_apps/{id}");
    await write(user, field("Website"), "");

    await user.click(save());

    await waitFor(() => {
      if (said().length === 0) throw new Error("test: the screen says nothing yet");
    });
    expect(sent.seen.map((s) => s.body.website)).toEqual([""]);
  });

  for (const [name, needs] of [
    ["Name", "Enter a name."],
    ["Scopes", "Enter at least one scope."],
  ] as const)
    test(`${name} emptied is said under its field, which takes the focus, and nothing is sent`, async () => {
      const { user, fake } = await read(confidential);
      const sent = fake.script("PATCH /oauth_apps/{id}");
      await write(user, field(name), "");

      await user.click(save());

      await waitFor(() => {
        if (field(name).getAttribute("aria-invalid") !== "true") throw new Error("test: the field is not marked yet");
      });
      await focusOn(field(name));
      expect(describedBy(field(name))).toEqual([hints[name], needs]);
      expect(sent.seen).toHaveLength(0);
      expect(said()).toEqual([]);
    });

  test("a field the server refuses is said under it in the server's words and takes the focus, the form says before its button that this could not be done, and not that it saved", async () => {
    const { user, fake } = await read(confidential);
    const sent = fake.script("PATCH /oauth_apps/{id}");
    await write(user, field("Redirect URIs"), "not an address");

    await user.click(save());

    await waitFor(() => {
      if (field("Redirect URIs").getAttribute("aria-invalid") !== "true")
        throw new Error("test: the field is not marked yet");
    });
    await focusOn(field("Redirect URIs"));
    expect(sent.seen.map((s) => s.body.redirect_uris)).toEqual([["not an address"]]);
    expect(describedBy(field("Redirect URIs"))).toEqual([
      hints["Redirect URIs"],
      "This value is not valid. Check what you entered.",
    ]);
    const [refused, ...others] = screen.getAllByRole("alert");
    expect(others).toHaveLength(0);
    expect(refused === undefined ? "" : words(refused)).toBe(notDone);
    expect(refused === undefined ? false : before(refused, save())).toBe(true);
    expect(said()).not.toContain("Saved");
    expect(field("Redirect URIs").value).toBe("not an address");
  });
});

describe("Delete", () => {
  test("asks first, naming the app and saying what stops working, and once confirmed the server no longer holds the app and the screen goes back to Apps", async () => {
    const { user, fake, address, app } = await read(confidential);
    const sent = fake.script("DELETE /oauth_apps/{id}");
    expect(rows("Delete app")).toEqual([
      row(
        "Delete this app for good Every token of the app stops working at once, also for other people who allowed it. This cannot be undone.",
        { offers: ["Delete"] },
      ),
    ]);

    await user.click(remove());

    const dialog = await asking("Delete Morning bot?");
    expect(words(dialog)).toContain(
      "Every token of the app stops working at once, also for other people who allowed it. This cannot be undone.",
    );
    expect(buttonsOf(dialog)).toEqual(["Close", "Cancel", "Delete"]);
    expect(sent.seen).toHaveLength(0);

    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      if (address.backs === 0) throw new Error("test: the screen did not go back yet");
    });
    expect(address.backs).toBe(1);
    expect(sent.seen.map((s) => s.url.pathname.split("/").at(-1))).toEqual([app.id]);
    expect((await ownApps(fake)).map((a) => a.name)).toEqual(["Home dashboard", "Old importer"]);
  });

  test("Cancel sends nothing, leaves the reader on the screen, and gives the focus back to the button", async () => {
    const { user, address, requests } = await read(confidential);
    const button = remove();
    await user.click(button);
    const dialog = await asking("Delete Morning bot?");
    const sent = requests.length;

    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    await closed();
    await waitFor(() => {
      if (document.activeElement !== button) throw new Error("test: the focus is not back on the button yet");
    });
    expect(requests.slice(sent)).toEqual([]);
    expect(address.backs).toBe(0);
  });
});

describe("an app the administrators disabled", () => {
  test("a confidential one says so first, and what it can still do; it has no way to issue a token, and can still get a new secret, be changed and be deleted", async () => {
    await read(disabled);

    expect(sections()).toEqual([isDisabled, "Credentials", "App settings", "Delete app"]);
    expect(words(section(isDisabled))).toContain(
      `${stopped} You can still make a new secret, change its settings or delete it.`,
    );
    expect(rows("Credentials")).toEqual([secretRow]);
    expect(screen.queryAllByRole("link")).toHaveLength(0);
    expect(buttons()).toEqual(["Apps", "Copy client ID", "Make a new secret", "Delete", "Save changes"]);
    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("a public one says so without a secret to make, and has no way to issue a token either", async () => {
    await read(open, {
      before: ({ fake }) => {
        fake.script("GET /oauth_apps/{id}", { change: (app) => ({ ...app, disabled: true }) });
      },
    });

    expect(sections()).toEqual([isDisabled, "Credentials", "App settings", "Delete app"]);
    expect(words(section(isDisabled))).toContain(`${stopped} You can still change its settings or delete it.`);
    expect(words(section(isDisabled))).not.toContain("secret");
    expect(rows("Credentials")).toEqual([]);
    expect(screen.queryAllByRole("link")).toHaveLength(0);
    expect(buttons()).toEqual(["Apps", "Copy client ID", "Delete", "Save changes"]);
  });

  test("an app that is not disabled has no such summary", async () => {
    await read(confidential);

    expect(sections()).not.toContain(isDisabled);
  });
});

describe("an address that names no app of the reader's", () => {
  test("says that the app is not among the reader's apps, leads to the entry, and offers nothing else", async () => {
    const on = await drawn(unknown);
    const missing = await screen.findByRole("alert");
    await atRest(on);

    expect(words(missing)).toMatch(notAmong);
    expect(within(missing).getByRole("link", { name: "See your apps" }).getAttribute("href")).toBe("/settings/apps");
    expect(sections()).toEqual([]);
    expect(screen.queryAllByRole("textbox")).toHaveLength(0);
    expect(buttons()).toEqual(["Apps"]);
    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("its way back leads to Apps", async () => {
    const { user, address } = await drawn(unknown);
    await screen.findByRole("alert");

    await user.click(screen.getByRole("button", { name: "Apps" }));

    expect(address.backs).toBe(1);
  });
});
