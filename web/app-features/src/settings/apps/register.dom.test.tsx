// SPDX-License-Identifier: AGPL-3.0-only

// Registering an app, under Apps (#3206, ADR-0090 §1): the form waits for the list of the reader's apps, which says
// whether they may register one; on a server whose administrators turned registration off it says so in the form's
// place, and nothing can be sent; a field left empty or one the server refuses is said under that field, and the
// first field at fault takes the focus, which stays on the button when no field is at fault; and a registered app
// shows, in the form's place and once, its client ID and, for a confidential app, its secret, each with a way to
// copy it (that this view takes the focus is jsdom's, focus.jsdom.test.tsx). A server that turned registration off after the list was read says so when the form is
// sent. That the entry lists the new app, and says that registration is off, is the app's test (web/app,
// settings-apps.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { settingsFindings } from "@my/design-system/test/settings";
import { screen, waitFor, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import {
  alerts,
  before,
  buttons,
  clipboard,
  field,
  hints,
  loading,
  notDone,
  notLoaded,
  notShownAgain,
  ownApps,
  refusingClipboard,
  registrationOff,
  said,
  shown,
  write,
  type AppFieldName,
} from "../../test/apps";
import { atRest, describedBy, focused, focusOn, inOrder, section, sections, words } from "../../test/security";
import { settingsOn, type Access, type Given } from "../../test/settings";
import { AppRegister } from "./register";

/** The screen that registers an app, before its read answered. */
const drawn = (access: Access = {}, given: Given = {}) =>
  settingsOn(access, (address) => <AppRegister {...address} />, given);

/** The screen once the list of the reader's apps answered: the form, or what stands in its place. */
async function read(access: Access = {}, given: Given = {}) {
  const on = await drawn(access, given);
  await screen.findAllByRole("region");
  await atRest(on);
  return on;
}

/** The two types of an app, as the choice names them: the type, then what it means. */
const confidential = "Confidential: can keep a secret, like a server or a bot";
const open = "Public: cannot keep a secret, like an app on a phone or in a browser";

/** The form's own button. */
const register = () => screen.getByRole("button", { name: "Register an app" });
/** The choice of the app's type: its two options by their names, in their order, and whether each is chosen. */
function types() {
  const group = within(screen.getByRole("radiogroup", { name: "Type" }));
  const named = [confidential, open].map((name) => group.getByRole<HTMLInputElement>("radio", { name }));
  if (group.getAllByRole("radio").some((radio, place) => radio !== named[place]))
    throw new Error("test: the choice of the type is not confidential, then public, and nothing else");
  return named.map((radio) => radio.checked);
}

/** Fills the fields a registration needs: a name, a redirect URI and a scope. */
async function fill(user: UserEvent) {
  await write(user, field("Name"), "Reading list");
  await write(user, field("Redirect URIs"), "https://reading.example/callback");
  await write(user, field("Scopes"), "read");
}

/** Waits until the form gave way to what shows the registered app. */
const registered = () => screen.findByRole("region", { name: "“Reading list” is registered" });

describe("until the list of the reader's apps is read", () => {
  test("the screen says that it loads the settings, with no field and nothing to send, and shows the form once the list answered", async () => {
    let held: { release: () => void } | undefined;
    await drawn(
      {},
      {
        before: ({ fake }) => {
          held = fake.script("GET /oauth_apps", { hold: "request" });
        },
      },
    );

    expect((await screen.findByRole("status")).textContent).toBe(loading);
    expect(screen.queryAllByRole("textbox")).toHaveLength(0);
    expect(buttons()).toEqual(["Apps"]);

    held?.release();

    await screen.findByRole("textbox", { name: "Name" });
    expect(said()).toEqual([]);
  });

  test("the screen says that the settings could not be loaded when the list fails, and Try again reads it and shows the form", async () => {
    const { user } = await drawn(
      {},
      {
        before: ({ fake }) => {
          fake.script("GET /oauth_apps", { fail: 500, times: 1 });
        },
      },
    );

    const failed = await screen.findByRole("alert");
    expect(words(failed)).toMatch(notLoaded);
    expect(screen.queryAllByRole("textbox")).toHaveLength(0);
    expect(buttons()).toEqual(["Apps", "Try again"]);

    await user.click(within(failed).getByRole("button", { name: "Try again" }));

    await screen.findByRole("textbox", { name: "Name" });
    expect(alerts()).toEqual([]);
  });
});

describe("the form", () => {
  test("asks for the name and the website, the redirect URIs and the scopes, each with what it takes, then the type, and keeps the rules of a settings screen (ADR-0110 §6)", async () => {
    await read();

    expect(sections()).toEqual(["Name and website", "Authorization", "Type"]);
    expect(screen.getAllByRole("textbox").map((box) => box.getAttribute("name"))).toEqual([
      "name",
      "website",
      "redirect_uris",
      "scope",
    ]);
    for (const name of Object.keys(hints) as AppFieldName[]) {
      expect(describedBy(field(name))).toEqual([hints[name]]);
      expect(field(name).value).toBe("");
      expect(field(name).getAttribute("aria-invalid")).not.toBe("true");
    }
    expect(buttons()).toEqual(["Apps", "Register an app"]);
    expect(said()).toEqual([]);
    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("takes the redirect URIs in a field of three lines, and the others in one", async () => {
    await read();

    const uris = field("Redirect URIs");
    expect(uris instanceof HTMLTextAreaElement).toBe(true);
    expect(uris.getAttribute("rows")).toBe("3");
    for (const name of ["Name", "Website", "Scopes"] as const)
      expect(field(name) instanceof HTMLTextAreaElement).toBe(false);
  });

  test("offers the type as a choice of two, each with what it means, confidential first and chosen, under the sentence that it cannot be changed later", async () => {
    await read();

    expect(types()).toEqual([true, false]);
    expect(words(section("Type"))).toMatch(
      inOrder("Type", "The type decides whether the app has a secret. You cannot change it later.", confidential, open),
    );
  });

  test("its way back leads to Apps", async () => {
    const { user, address } = await read();

    await user.click(screen.getByRole("button", { name: "Apps" }));

    expect(address.backs).toBe(1);
  });
});

test("where the administrators turned registration off, the screen says so in the form's place: no field, no type, nothing to send, and nothing was sent", async () => {
  const { requests } = await read({ oauthApps: "disabled" });

  expect(sections()).toEqual(["Your apps"]);
  expect(words(section("Your apps"))).toMatch(inOrder("Your apps", registrationOff));
  expect(screen.queryAllByRole("textbox")).toHaveLength(0);
  expect(screen.queryAllByRole("radio")).toHaveLength(0);
  expect(buttons()).toEqual(["Apps"]);
  expect(requests.filter((request) => !request.startsWith("GET "))).toEqual([]);
  expect(settingsFindings(document.body)).toEqual([]);
});

describe("a form that cannot be sent as it is", () => {
  test("sent empty, says under the name, the redirect URIs and the scopes what each needs, leaves the website alone, sends nothing, and gives the focus to the first of them, the name", async () => {
    const { user, fake } = await read();
    const sent = fake.script("POST /oauth_apps");

    await user.click(register());

    await waitFor(() => {
      if (field("Name").getAttribute("aria-invalid") !== "true") throw new Error("test: no field is marked yet");
    });
    await focusOn(field("Name"));
    expect(describedBy(field("Name"))).toEqual([hints.Name, "Enter a name."]);
    expect(describedBy(field("Redirect URIs"))).toEqual([hints["Redirect URIs"], "Enter at least one redirect URI."]);
    expect(describedBy(field("Scopes"))).toEqual([hints.Scopes, "Enter at least one scope."]);
    expect(field("Redirect URIs").getAttribute("aria-invalid")).toBe("true");
    expect(field("Scopes").getAttribute("aria-invalid")).toBe("true");
    expect(describedBy(field("Website"))).toEqual([hints.Website]);
    expect(field("Website").getAttribute("aria-invalid")).not.toBe("true");
    expect(sent.seen).toHaveLength(0);
    expect(sections()).toEqual(["Name and website", "Authorization", "Type"]);
  });

  test("with one field left empty, says so under that field alone, which takes the focus", async () => {
    const { user, fake } = await read();
    const sent = fake.script("POST /oauth_apps");
    await write(user, field("Name"), "Reading list");
    await write(user, field("Scopes"), "read");

    await user.click(register());

    await waitFor(() => {
      if (field("Redirect URIs").getAttribute("aria-invalid") !== "true")
        throw new Error("test: the field is not marked yet");
    });
    await focusOn(field("Redirect URIs"));
    expect(describedBy(field("Name"))).toEqual([hints.Name]);
    expect(describedBy(field("Scopes"))).toEqual([hints.Scopes]);
    expect(sent.seen).toHaveLength(0);
  });

  // A refused setting, as the server answers one: its title for the form, the member it names, and what is wrong.
  for (const [name, member] of [
    ["Name", "name"],
    ["Website", "website"],
    ["Redirect URIs", "redirect_uris"],
    ["Scopes", "scope"],
  ] as const)
    test(`a ${name} the server refuses is said under that field in the server's words, after what the field takes; the field takes the focus, and the form says before its button that this could not be done`, async () => {
      const { user, fake, address } = await read();
      fake.script("POST /oauth_apps", {
        fail: {
          type: "about:blank",
          title: notDone,
          status: 422,
          field: member,
          detail: "This value is not valid. Check what you entered.",
        },
      });
      await fill(user);

      await user.click(register());

      await waitFor(() => {
        if (field(name).getAttribute("aria-invalid") !== "true") throw new Error("test: the field is not marked yet");
      });
      await focusOn(field(name));
      expect(describedBy(field(name))).toEqual([hints[name], "This value is not valid. Check what you entered."]);
      for (const other of (Object.keys(hints) as AppFieldName[]).filter((one) => one !== name)) {
        expect(describedBy(field(other))).toEqual([hints[other]]);
        expect(field(other).getAttribute("aria-invalid")).not.toBe("true");
      }
      const [refused, ...others] = screen.getAllByRole("alert");
      expect(others).toHaveLength(0);
      expect(refused === undefined ? "" : words(refused)).toBe(notDone);
      expect(refused === undefined ? false : before(refused, register())).toBe(true);
      // What the reader wrote is kept, on the same screen.
      expect(field("Name").value).toBe("Reading list");
      expect(address.backs).toBe(0);
    });

  test("a redirect URI that is no address is refused by the server itself, under its field", async () => {
    const { user, fake } = await read();
    const sent = fake.script("POST /oauth_apps");
    await fill(user);
    await write(user, field("Redirect URIs"), "not an address");

    await user.click(register());

    await waitFor(() => {
      if (field("Redirect URIs").getAttribute("aria-invalid") !== "true")
        throw new Error("test: the field is not marked yet");
    });
    expect(sent.seen.map((s) => s.body.redirect_uris)).toEqual([["not an address"]]);
    expect(describedBy(field("Redirect URIs"))).toEqual([
      hints["Redirect URIs"],
      "This value is not valid. Check what you entered.",
    ]);
    expect(alerts()).toEqual([notDone]);
    expect(await ownApps(fake)).toHaveLength(3);
  });

  test("a refusal that names no field is said before the button in the server's words, and the form stays as it was written", async () => {
    const { user, fake } = await read();
    fake.script("POST /oauth_apps", {
      fail: { type: "/api/v1/problems/too-many-oauth-apps", title: "You have 25 apps already", status: 409 },
    });
    await fill(user);

    await user.click(register());

    const refused = await screen.findByRole("alert");
    expect(words(refused)).toBe("You have 25 apps already");
    expect(before(refused, register())).toBe(true);
    for (const name of Object.keys(hints) as AppFieldName[])
      expect(field(name).getAttribute("aria-invalid")).not.toBe("true");
    expect(field("Name").value).toBe("Reading list");
    // No field is at fault, so none takes the focus: it stays where the reader pressed, beside what the form says.
    expect(focused()).toBe("button: Register an app");
  });

  for (const [what, fail] of [
    ["too many requests (429)", 429],
    ["a request that got no answer", "network"],
  ] as const)
    test(`${what} marks no field either, and leaves the focus on the button, after the one alert`, async () => {
      const { user, fake } = await read();
      fake.script("POST /oauth_apps", { fail });
      await fill(user);

      await user.click(register());

      const refused = await screen.findByRole("alert");
      expect(alerts()).toHaveLength(1);
      expect(before(refused, register())).toBe(true);
      for (const name of Object.keys(hints) as AppFieldName[])
        expect(field(name).getAttribute("aria-invalid")).not.toBe("true");
      expect(focused()).toBe("button: Register an app");
    });
});

describe("what is sent", () => {
  test("is the name, the website, the redirect URIs one per line without the empty lines and the spaces around them, the scopes a space apart, and the type", async () => {
    const { user, fake } = await read();
    const sent = fake.script("POST /oauth_apps");
    await write(user, field("Name"), "Reading list");
    await write(user, field("Website"), "https://reading.example/");
    await write(user, field("Redirect URIs"), "https://reading.example/callback\n\n  com.example.app:/callback  \n");
    await write(user, field("Scopes"), "  read   write:statuses ");

    await user.click(register());

    await registered();
    expect(sent.seen.map((s) => s.body)).toEqual([
      {
        name: "Reading list",
        website: "https://reading.example/",
        redirect_uris: ["https://reading.example/callback", "com.example.app:/callback"],
        scope: "read write:statuses",
        client_type: "confidential",
      },
    ]);
  });

  test("has no website when the field is left empty, and the type the reader chose", async () => {
    const { user, fake } = await read();
    const sent = fake.script("POST /oauth_apps");
    await fill(user);
    await user.click(screen.getByRole("radio", { name: open }));
    expect(types()).toEqual([false, true]);

    await user.click(register());

    await registered();
    expect(sent.seen.map((s) => s.body)).toEqual([
      {
        name: "Reading list",
        redirect_uris: ["https://reading.example/callback"],
        scope: "read",
        client_type: "public",
      },
    ]);
  });
});

describe("a confidential app, once registered", () => {
  /** A confidential app registered, its client ID and its secret on the screen. */
  async function done() {
    const on = await read();
    await fill(on.user);
    await on.user.click(register());
    await registered();
    return on;
  }

  test("shows in the form's place that it is registered, its client ID, and its secret with the words that it is not shown again, each with its Copy, and Finish", async () => {
    const { fake } = await done();

    expect(sections()).toEqual(["“Reading list” is registered", "Client ID", "Client secret"]);
    expect(words(section("“Reading list” is registered"))).toContain(
      "Its client ID stays on the app's page. Its secret is shown here only this once.",
    );
    expect(shown("Client ID")).toMatch(/^[A-Za-z0-9_-]{22}$/);
    expect(shown("Client secret")).toMatch(/^my_cs_[A-Za-z0-9_-]{43}$/);
    // The secret is what is not shown again; the client ID stays on the app's screen.
    expect(words(section("Client secret"))).toContain(notShownAgain);
    expect(words(section("Client ID"))).not.toContain(notShownAgain);
    // The form is gone with its fields and its way back: Finish is the way on.
    expect(screen.queryAllByRole("textbox")).toHaveLength(0);
    expect(buttons()).toEqual(["Copy client ID", "Copy secret", "Finish"]);
    expect(said()).toEqual([]);
    expect(settingsFindings(document.body)).toEqual([]);
    // The client ID is the one the server keeps.
    expect((await ownApps(fake)).map((app) => [app.name, app.client_id])).toContainEqual([
      "Reading list",
      shown("Client ID"),
    ]);
  });

  for (const [button, list] of [
    ["Copy secret", "Client secret"],
    ["Copy client ID", "Client ID"],
  ] as const)
    test(`${button} puts the whole value on the clipboard, and its section says so`, async () => {
      const { user } = await done();

      await user.click(screen.getByRole("button", { name: button }));

      await waitFor(() => {
        if (said().length === 0) throw new Error("test: the screen says nothing yet");
      });
      expect(said()).toEqual(["Copied"]);
      expect(within(section(list)).getAllByRole("status").map(words)).toEqual(["Copied"]);
      expect(await clipboard().readText()).toBe(shown(list));
    });

  test("a clipboard that refuses says to select the text and copy it", async () => {
    const { user } = await done();
    refusingClipboard();

    await user.click(screen.getByRole("button", { name: "Copy secret" }));

    await waitFor(() => {
      if (said().length === 0) throw new Error("test: the screen says nothing yet");
    });
    expect(said()).toEqual(["Could not copy. Select the text and copy it yourself."]);
    // The secret is still there to select.
    expect(shown("Client secret")).toMatch(/^my_cs_/);
  });

  test("Finish leads to Apps, and not before", async () => {
    const { user, address } = await done();
    expect(address.backs).toBe(0);

    await user.click(screen.getByRole("button", { name: "Finish" }));

    expect(address.backs).toBe(1);
  });
});

test("a public app, once registered, shows its client ID alone, and says that a public app has no secret", async () => {
  const { user } = await read();
  await fill(user);
  await user.click(screen.getByRole("radio", { name: open }));

  await user.click(register());

  await registered();
  expect(sections()).toEqual(["“Reading list” is registered", "Client ID"]);
  expect(words(section("“Reading list” is registered"))).toContain(
    "A public app has no secret. Its client ID stays on the app's page.",
  );
  expect(shown("Client ID")).toMatch(/^[A-Za-z0-9_-]{22}$/);
  expect(words(screen.getByRole("main"))).not.toContain("my_cs_");
  expect(words(screen.getByRole("main"))).not.toContain(notShownAgain);
  expect(buttons()).toEqual(["Copy client ID", "Finish"]);
  expect(settingsFindings(document.body)).toEqual([]);
});

// The list said the reader could register, and the administrators turned registration off since: the server's
// refusal of the registration is how the screen learns it.
test("a registration refused because registration was turned off meanwhile puts the sentence of a server that turned it off in the form's place", async () => {
  const { user, fake } = await read();
  const sent = fake.script("POST /oauth_apps", {
    fail: {
      type: "/api/v1/problems/oauth-app-registration-disabled",
      title: "App registration is turned off",
      status: 403,
    },
  });
  await fill(user);

  await user.click(register());

  await screen.findByRole("region", { name: "Your apps" });
  expect(sections()).toEqual(["Your apps"]);
  expect(words(section("Your apps"))).toMatch(inOrder("Your apps", registrationOff));
  expect(screen.queryAllByRole("textbox")).toHaveLength(0);
  expect(buttons()).toEqual(["Apps"]);
  expect(alerts()).toEqual([]);
  expect(sent.seen).toHaveLength(1);
});
