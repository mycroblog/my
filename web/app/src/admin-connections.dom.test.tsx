// SPDX-License-Identifier: AGPL-3.0-only

// The rules, relays, login providers and OAuth apps in the app (#3262, FR-705, FR-922, ADR-0058 §1, ADR-0090,
// ADR-0106), on the fake API, as what follows a press: a rule is written with its explanation, changed and moved, and
// retired last, after a question, while a retired rule only reads; a relay is joined, left after a question, and
// joined again, and an address the server refuses is marked; a login provider is added and shows its callback URL, is
// changed with a new secret, enabled, disabled after a question, and deleted by its name typed; an OAuth app is
// disabled after a question and enabled, and a host is denied after a question and lifted. What each screen shows at
// rest is the stories' (admin.stories.tsx); the settings are admin.dom.test.tsx's.

import { expect, test } from "bun:test";
import type { FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

/** Waits until the app has no read on its way. */
const settled = (app: TestApp) =>
  waitFor(() => {
    if (app.data.queryClient.isFetching() > 0) throw new Error("test: the screen's reads are on their way");
  });

/** The app as hana, the scenario's administrator, at the address `at` gives for the fake, once it is read. */
async function atAdmin(at: (fake: ReturnType<typeof fakeApp>["fake"]) => string, options: FakeOptions = {}) {
  const on = fakeApp({ viewer: "hana", ...options });
  const app = await renderApp(at(on.fake), { on });
  await main().findByRole("heading", { level: 1 });
  await settled(app);
  return app;
}

const button = (name: string) => main().getByRole("button", { name });
const textbox = (name: string) => main().getByRole("textbox", { name });

/** Replaces what the field `field` holds with `value`, as a reader types it. */
async function typeIn(app: TestApp, field: HTMLElement, value: string) {
  await app.user.clear(field);
  await app.user.type(field, value);
}

/** The question of `name` once it is open. */
const asking = async (name: string) => within(await screen.findByRole("alertdialog", { name }));

/** Waits until no question is open. */
const answered = () =>
  waitFor(() => {
    if (screen.queryAllByRole("alertdialog").length > 0) throw new Error("test: a question is still open");
  });

/** Waits until the list of `name` holds an item that says `sentence`, and returns the items' words. */
const listSays = (name: string, sentence: string | RegExp) =>
  waitFor(() => {
    const items = within(main().getByRole("list", { name })).getAllByRole("listitem").map(words);
    if (!items.some((item) => (typeof sentence === "string" ? item === sentence : sentence.test(item))))
      throw new Error(`test: ${name} reads ${items.join(" / ")}`);
    return items;
  });

// Rules.

test("a new rule with its explanation reaches the server and joins the rules in effect last", async () => {
  const on = fakeApp({ viewer: "hana" });
  const written = on.fake.script("POST /admin/rules");
  const app = await renderApp("/admin/rules/new", { on });
  await main().findByRole("textbox", { name: "Rule" });

  await typeIn(app, textbox("Rule"), "Say hello to newcomers.");
  await typeIn(app, textbox("Explanation"), "A welcome goes a long way.");
  await app.user.click(button("Add a rule"));

  expect((await written.arrived()).body).toEqual({
    text: "Say hello to newcomers.",
    hint: "A welcome goes a long way.",
  });
  await app.arrived("/admin/rules");
  const items = await listSays("In effect", "5. Say hello to newcomers. A welcome goes a long way.");
  expect(items.at(-1)).toBe("5. Say hello to newcomers. A welcome goes a long way.");
});

test("a rule reworded and moved to the first place is saved, and the rules show it first", async () => {
  const on = fakeApp({ viewer: "hana" });
  const id = on.fake.admin.ruleId(1);
  const reworded = on.fake.script("PUT /admin/rules/{id}");
  const moved = on.fake.script("POST /admin/rules/{id}/move");
  const app = await renderApp(`/admin/rules/${id}`, { on });
  await main().findByRole("textbox", { name: "Rule" });
  await settled(app);

  await typeIn(app, textbox("Rule"), "Credit the photographer of every photo you share.");
  await app.user.selectOptions(main().getByRole("combobox", { name: "Place" }), "1st");
  await app.user.click(button("Save"));

  expect((await reworded.arrived()).body.text).toBe("Credit the photographer of every photo you share.");
  expect((await moved.arrived()).body.position).toBe(1);
  await app.navigate("/admin/rules");
  const items = await listSays("In effect", "1. Credit the photographer of every photo you share.");
  expect(items[0]).toBe("1. Credit the photographer of every photo you share.");
});

test("Retire comes last on a rule's page, and retires the rule for good only once the question is answered", async () => {
  const on = fakeApp({ viewer: "hana" });
  const id = on.fake.admin.ruleId(1);
  const retired = on.fake.script("POST /admin/rules/{id}/retire");
  const app = await renderApp(`/admin/rules/${id}`, { on });
  await main().findByRole("button", { name: "Retire" });
  await settled(app);
  const buttons = main().getAllByRole("button");
  expect(buttons.at(-1)).toBe(button("Retire"));

  await app.user.click(button("Retire"));
  const first = await asking("Retire this rule?");
  expect(words(screen.getByRole("alertdialog"))).toContain(
    "“Credit the photographer when you share a photo.” leaves the rules for good. This cannot be undone.",
  );
  await app.user.click(first.getByRole("button", { name: "Cancel" }));
  await answered();
  expect(retired.seen).toHaveLength(0);

  await app.user.click(button("Retire"));
  await app.user.click((await asking("Retire this rule?")).getByRole("button", { name: "Retire" }));

  expect((await retired.arrived()).url.pathname).toEndWith(`/admin/rules/${id}/retire`);
  await app.navigate("/admin/rules");
  await listSays("Retired", "Credit the photographer when you share a photo.");
  expect(
    within(main().getByRole("list", { name: "In effect" }))
      .getAllByRole("listitem")
      .map(words),
  ).not.toContain("2. Credit the photographer when you share a photo.");
});

test("a retired rule's page says what it said and when it was retired, with nothing to change", async () => {
  await atAdmin((fake) => `/admin/rules/${fake.admin.ruleId(4)}`);

  const facts = within(main().getByRole("list", { name: "Wording" }))
    .getAllByRole("listitem")
    .map(words);
  expect(facts[0]).toBe("No posts about other servers' drama.");
  expect(facts[1]).toMatch(/^Retired on /);
  expect(main().queryAllByRole("textbox")).toHaveLength(0);
  expect(main().queryAllByRole("button", { name: /^(Save|Retire)$/ })).toHaveLength(0);
});

// Relays.

test("a relay joined by its address is asked to accept, and its page says it waits", async () => {
  const on = fakeApp({ viewer: "hana" });
  const joined = on.fake.script("POST /admin/relays");
  const app = await renderApp("/admin/relays/new", { on });
  await main().findByRole("textbox", { name: "URL" });

  await typeIn(app, textbox("URL"), "https://relay.new.example/inbox");
  await app.user.click(button("Join"));

  expect((await joined.arrived()).body).toEqual({ url: "https://relay.new.example/inbox", receive: true });
  expect(await main().findByRole("heading", { level: 1, name: "relay.new.example" })).toBeDefined();
  await listSays("Relay", /^Awaiting the relay's answer /);
});

for (const [what, url] of [
  ["an address that is not https", "http://relay.new.example/inbox"],
  ["the address of a relay the server has already", "https://relay.fedi.example/inbox"],
] as const)
  test(`${what} is marked under the relay's field, and nothing is joined`, async () => {
    const on = fakeApp({ viewer: "hana" });
    const app = await renderApp("/admin/relays/new", { on });
    await main().findByRole("textbox", { name: "URL" });
    const before = (await app.data.api.GET("/admin/relays")).data?.items.length;

    await typeIn(app, textbox("URL"), url);
    await app.user.click(button("Join"));

    await waitFor(() => {
      if (textbox("URL").getAttribute("aria-invalid") !== "true") throw new Error("test: the address is not marked");
    });
    expect(app.path()).toBe("/admin/relays/new");
    expect((await app.data.api.GET("/admin/relays")).data?.items.length).toBe(before);
  });

test("a joined relay is left once the question is answered, and can then be joined again", async () => {
  const on = fakeApp({ viewer: "hana" });
  const id = on.fake.admin.relayId(0);
  const left = on.fake.script("DELETE /admin/relays/{id}/follow");
  const app = await renderApp(`/admin/relays/${id}`, { on });
  await main().findByRole("button", { name: "Leave" });
  await settled(app);

  await app.user.click(button("Leave"));
  const question = await asking("Leave this relay?");
  expect(words(screen.getByRole("alertdialog"))).toContain(
    "This server stops sending to relay.fedi.example and taking in what it relays. You can join it again later.",
  );
  await app.user.click(question.getByRole("button", { name: "Leave" }));

  expect((await left.arrived()).url.pathname).toEndWith(`/admin/relays/${id}/follow`);
  expect(await main().findByRole("button", { name: "Join again" })).toBeDefined();
});

test("a relay that rejected the server is joined again, and its page says it waits", async () => {
  const on = fakeApp({ viewer: "hana" });
  const id = on.fake.admin.relayId(2);
  const rejoined = on.fake.script("POST /admin/relays/{id}/follow");
  const app = await renderApp(`/admin/relays/${id}`, { on });
  await main().findByRole("button", { name: "Join again" });
  await settled(app);

  await app.user.click(button("Join again"));

  expect((await rejoined.arrived()).url.pathname).toEndWith(`/admin/relays/${id}/follow`);
  expect(await main().findByRole("button", { name: "Leave" })).toBeDefined();
  await listSays("Relay", /^Awaiting the relay's answer /);
});

// Login providers.

test("an OpenID Connect provider added reaches the server, and its page gives the callback URL to register", async () => {
  const on = fakeApp({ viewer: "hana" });
  const added = on.fake.script("POST /admin/login_providers");
  const app = await renderApp("/admin/login-providers/new", { on });
  await main().findByRole("textbox", { name: "Name" });

  await typeIn(app, textbox("Name"), "River SSO");
  await typeIn(app, textbox("Issuer"), "https://sso.river.example");
  await typeIn(app, textbox("Client ID"), "river-client");
  await typeIn(app, main().getByLabelText("Client secret"), "river-secret");
  await app.user.click(button("Add a login provider"));

  expect((await added.arrived()).body).toMatchObject({
    kind: "oidc",
    display_name: "River SSO",
    issuer: "https://sso.river.example",
    client_id: "river-client",
    client_secret: "river-secret",
  });
  await main().findByRole("heading", { level: 1, name: "River SSO" });
  const id = app.path().split("/").at(-1) ?? "";
  const callback = within(await main().findByRole("list", { name: "Callback URL" })).getAllByRole("listitem");
  expect(callback.map(words)).toEqual([`https://demo.test/auth/external/${id}/callback`]);
});

test("a provider's page gives the callback URL the server holds for it", async () => {
  const on = fakeApp({ viewer: "hana" });
  const id = on.fake.admin.providerId(2);
  const app = await renderApp(`/admin/login-providers/${id}`, { on });
  const list = await main().findByRole("list", { name: "Callback URL" });

  const { data } = await app.data.api.GET("/admin/login_providers/{id}", { params: { path: { id } } });
  expect(
    within(list)
      .getAllByRole("listitem")
      .map((item) => words(item)),
  ).toEqual([data?.callback_url ?? "none held"]);
  expect(button("Copy the URL")).toBeDefined();
});

test("a provider renamed with a new secret saves the change, and sends the secret to replace the old one", async () => {
  const on = fakeApp({ viewer: "hana" });
  const id = on.fake.admin.providerId(2);
  const changed = on.fake.script("PATCH /admin/login_providers/{id}");
  const rotated = on.fake.script("POST /admin/login_providers/{id}/rotate_secret");
  const app = await renderApp(`/admin/login-providers/${id}`, { on });
  await main().findByRole("textbox", { name: "Name" });
  await settled(app);

  await typeIn(app, textbox("Name"), "Town SSO");
  await typeIn(app, main().getByLabelText("Client secret"), "a-new-secret");
  await app.user.click(button("Save"));

  expect((await changed.arrived()).body.display_name).toBe("Town SSO");
  expect((await rotated.arrived()).body).toEqual({ client_secret: "a-new-secret" });
  expect(await main().findByRole("heading", { level: 1, name: "Town SSO" })).toBeDefined();
});

test("a disabled provider is enabled on Save, without a question", async () => {
  const on = fakeApp({ viewer: "hana" });
  const id = on.fake.admin.providerId(0);
  const enabled = on.fake.script("POST /admin/login_providers/{id}/enable");
  const app = await renderApp(`/admin/login-providers/${id}`, { on });
  await main().findByRole("combobox", { name: "Sign-in with it" });
  await settled(app);

  await app.user.selectOptions(main().getByRole("combobox", { name: "Sign-in with it" }), "On");
  await app.user.click(button("Save"));

  expect((await enabled.arrived()).url.pathname).toEndWith(`/admin/login_providers/${id}/enable`);
  expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
});

test("an enabled provider is disabled only once the question is answered", async () => {
  const on = fakeApp({ viewer: "hana" });
  const id = on.fake.admin.providerId(2);
  const disabled = on.fake.script("POST /admin/login_providers/{id}/disable");
  const app = await renderApp(`/admin/login-providers/${id}`, { on });
  await main().findByRole("combobox", { name: "Sign-in with it" });
  await settled(app);

  await app.user.selectOptions(main().getByRole("combobox", { name: "Sign-in with it" }), "Off");
  await app.user.click(button("Save"));
  const question = await asking("Disable this provider?");
  expect(disabled.seen).toHaveLength(0);
  await app.user.click(question.getByRole("button", { name: "Save" }));

  expect((await disabled.arrived()).url.pathname).toEndWith(`/admin/login_providers/${id}/disable`);
  await answered();
  const { data } = await app.data.api.GET("/admin/login_providers/{id}", { params: { path: { id } } });
  expect(data?.state).toBe("disabled");
});

test("a provider is deleted only with its name typed, and leaves the list", async () => {
  const on = fakeApp({ viewer: "hana" });
  const id = on.fake.admin.providerId(2);
  const deleted = on.fake.script("DELETE /admin/login_providers/{id}");
  const app = await renderApp(`/admin/login-providers/${id}`, { on });
  await main().findByRole("button", { name: "Delete" });
  await settled(app);

  await app.user.click(button("Delete"));
  const question = await asking("Delete this login provider?");
  const confirm = question.getByRole("button", { name: "Delete" });
  const name = question.getByRole("textbox", { name: "To confirm, enter Town Workspace" });
  expect(confirm.getAttribute("aria-disabled") === "true" || confirm.hasAttribute("disabled")).toBe(true);
  await app.user.type(name, "Town Work");
  expect(confirm.getAttribute("aria-disabled") === "true" || confirm.hasAttribute("disabled")).toBe(true);
  await app.user.clear(name);
  await app.user.type(name, "Town Workspace");
  await app.user.click(question.getByRole("button", { name: "Delete" }));

  expect((await deleted.arrived()).url.pathname).toEndWith(`/admin/login_providers/${id}`);
  await app.arrived("/admin/login-providers");
  await listSays("Login providers", /^GitHub /);
  expect(
    within(main().getByRole("list", { name: "Login providers" }))
      .getAllByRole("listitem")
      .map(words)
      .filter((item) => item.startsWith("Town Workspace")),
  ).toEqual([]);
});

test("the providers' list says who limiting sign-in to them would keep out, while no provider is enabled", async () => {
  await atAdmin(() => "/admin/login-providers", { admin: { providersOnlyWaiting: true } });

  const limit = main().getByRole("region", { name: "Sign-in will be limited to login providers" });
  expect(words(limit)).toContain(
    "It starts once a provider is enabled. 14 active people have no linked provider and cannot sign in again once their sessions end.",
  );
});

// OAuth apps and denied hosts.

/** The row of the app `name` in the list of the apps. */
const appRow = (name: string) => within(main().getByRole("group", { name }));

test("an app is disabled only once the question is answered, and can be enabled again", async () => {
  const on = fakeApp({ viewer: "hana" });
  const disabled = on.fake.script("POST /admin/oauth_clients/{id}/disable");
  const enabled = on.fake.script("POST /admin/oauth_clients/{id}/enable");
  const app = await renderApp("/admin/oauth-apps", { on });
  await main().findByRole("group", { name: "Pocket Reader" });
  await settled(app);

  await app.user.click(appRow("Pocket Reader").getByRole("button", { name: "Disable" }));
  const question = await asking("Disable this app?");
  expect(words(screen.getByRole("alertdialog"))).toContain("Every token of Pocket Reader is revoked at once");
  expect(disabled.seen).toHaveLength(0);
  await app.user.click(question.getByRole("button", { name: "Disable" }));

  await disabled.arrived();
  const enable = await appRow("Pocket Reader").findByRole("button", { name: "Enable" });
  await listSays("Apps", /^Pocket Reader Disabled /);
  await app.user.click(enable);

  await enabled.arrived();
  expect(await appRow("Pocket Reader").findByRole("button", { name: "Disable" })).toBeDefined();
  const appOf = (path: string | undefined) => path?.replace(/\/(enable|disable)$/, "");
  expect(appOf(enabled.seen[0]?.url.pathname)).toBe(appOf(disabled.seen[0]?.url.pathname));
});

test("the server's own app offers neither Disable nor Enable", async () => {
  await atAdmin(() => "/admin/oauth-apps");

  const own = within(main().getByRole("list", { name: "Apps" }))
    .getAllByRole("listitem")
    .filter((item) => words(item).startsWith("mycroblog Enabled This server's own"));
  expect(own).toHaveLength(1);
  expect(own.flatMap((item) => within(item).queryAllByRole("button"))).toEqual([]);
});

test("a host is denied only once the question is answered, and its denial can be lifted", async () => {
  const on = fakeApp({ viewer: "hana" });
  const denied = on.fake.script("POST /admin/oauth_client_host_denials");
  const lifted = on.fake.script("DELETE /admin/oauth_client_host_denials/{host}");
  const app = await renderApp("/admin/oauth-apps/hosts", { on });
  await main().findByRole("textbox", { name: "Host" });
  await settled(app);

  await typeIn(app, textbox("Host"), "bad.example");
  await app.user.click(button("Deny"));
  const question = await asking("Deny this host?");
  expect(words(screen.getByRole("alertdialog"))).toContain(
    "Apps on bad.example can no longer sign in, and their tokens are revoked at once.",
  );
  expect(denied.seen).toHaveLength(0);
  await app.user.click(question.getByRole("button", { name: "Deny" }));

  expect((await denied.arrived()).body.host).toBe("bad.example");
  await listSays("Denied", /^bad\.example Denied on /);
  await app.user.click(
    within(await main().findByRole("group", { name: "bad.example" })).getByRole("button", { name: "Lift" }),
  );

  expect((await lifted.arrived()).url.pathname).toEndWith("/admin/oauth_client_host_denials/bad.example");
  await waitFor(() => {
    if (main().queryAllByRole("group", { name: "bad.example" }).length > 0)
      throw new Error("test: the host is still denied");
  });
});
