// SPDX-License-Identifier: AGPL-3.0-only

// The sanctions of accounts and their appeals (#3260, FR-703, FR-707, FR-708, FR-714; ADR-0058 §9), on the fake API, as
// what follows a press: an action taken on a local and a remote account with its expiry, an action lifted, a ban asked
// with the account's handle typed and never lifted, only overturned through its appeal; an appeal upheld or overturned,
// the ban's upheld with its handle typed; and an account's Management, where the email and the IP addresses are asked
// for only once shown, and sign-in recovery is offered only with manage_login_recovery. What each screen shows at rest
// is the stories' (accounts.stories.tsx).

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

/** The app at the page `page` (`""`, `/actions`, `/manage`) of the fake's account of `username`, read by `viewer`. */
async function atAccount(username: string, { viewer = "marco", page = "" } = {}) {
  const on = fakeApp({ viewer });
  const id = on.fake.moderation.id(`account:${username}`);
  const app = await renderApp(`/moderation/accounts/${id}${page}`, { on });
  await main().findByRole("heading", { level: 1, name: `@${username}` });
  return { app, id };
}

/** The app at the fake's moderation item of `key` (`action:ban`, `appeal:kai`) under `path`, read by marco. */
async function atItem(path: (id: string, account: (username: string) => string) => string, key: string) {
  const on = fakeApp({ viewer: "marco" });
  const id = on.fake.moderation.id(key);
  const app = await renderApp(
    path(id, (username) => on.fake.moderation.id(`account:${username}`)),
    { on },
  );
  await main().findByRole("heading", { level: 1 });
  return { app, id };
}

/** Waits until the screen says `sentence`. */
const says = (sentence: string) =>
  waitFor(() => {
    if (!words(screen.getByRole("main")).includes(sentence))
      throw new Error(`test: the screen does not say ${JSON.stringify(sentence)}`);
  });

/** What the facts labelled `label` say, a term and its value joined with no space. */
const facts = (label: string) => words(main().getByLabelText(label));

/** The confirmation named `name` once it is open, whether it is a dialog or an alert dialog. */
const confirmationElement = (name: string) =>
  waitFor(() => {
    const [found] = [...screen.queryAllByRole("alertdialog", { name }), ...screen.queryAllByRole("dialog", { name })];
    if (found === undefined) throw new Error(`test: no confirmation ${JSON.stringify(name)}`);
    return found;
  });

/** The queries of the confirmation named `name` once it is open. */
const confirmation = async (name: string) => within(await confirmationElement(name));

/** Chooses `option` of the choice `label` of the screen. */
const choose = (app: TestApp, label: string, option: string) =>
  app.user.selectOptions(main().getByRole("combobox", { name: label }), option);

/** Writes `text` into the field `label` of the screen. */
async function write(app: TestApp, label: string, text: string) {
  await app.user.click(main().getByRole("textbox", { name: label }));
  await app.user.paste(text);
}

/** Whether the button is offered for a press: neither disabled nor marked so. */
const pressable = (button: HTMLElement) =>
  !(button as HTMLButtonElement).disabled && button.getAttribute("aria-disabled") !== "true";

/** The names of the options of the choice `label`. */
const options = (label: string) =>
  within(main().getByRole("combobox", { name: label }))
    .getAllByRole("option")
    .map((option) => option.textContent);

/** How many requests the script saw once the screen's reads settled. */
const seenAtRest = async (app: TestApp, script: { seen: readonly unknown[] }) => {
  await waitFor(() => {
    if (app.data.queryClient.isFetching() > 0) throw new Error("test: the screen's reads are on their way");
  });
  return script.seen.length;
};

// Actions taken and lifted.

test("an action on a local account goes to the server with its kind, its expiry, its reason and the account told, and the account's actions show it in effect", async () => {
  const { app, id } = await atAccount("yuki");
  const taken = app.fake.script("POST /admin/accounts/{id}/actions");
  const reason = "Replied to strangers with the same link all evening.";

  await choose(app, "Action", "No posting");
  await choose(app, "Lasts", "7 days");
  await write(app, "Reason the account is told", reason);
  await app.user.click(main().getByRole("button", { name: "Take this action" }));

  const { url, body } = await taken.arrived();
  expect(url.pathname).toEndWith(`/admin/accounts/${id}/actions`);
  expect([body.kind, body.expires_in, body.public_reason, body.notify_target]).toEqual([
    "restrict_posting",
    7 * 24 * 60 * 60,
    reason,
    true,
  ]);
  await says("No posting is in effect now.");
  await waitFor(() => {
    const row = within(main().getByRole("table", { name: "Actions" }))
      .queryAllByRole("row")
      .find((r) => words(r).startsWith("No posting"));
    if (row === undefined || !words(row).includes("In effect"))
      throw new Error("test: the account's actions do not show the new one in effect");
  });
});

test("a remote account is offered only the actions its server can be held to, and one goes to the server with its expiry and without telling the account", async () => {
  const { app } = await atAccount("sora@photos.example");
  const taken = app.fake.script("POST /admin/accounts/{id}/actions");

  expect(options("Action").toSorted()).toEqual(["Limit", "Media always marked as sensitive", "Suspension"]);
  await choose(app, "Action", "Suspension");
  await choose(app, "Lasts", "30 days");
  await app.user.click(main().getByRole("button", { name: "Take this action" }));

  const { body } = await taken.arrived();
  expect([body.kind, body.expires_in, body.notify_target]).toEqual(["suspend", 30 * 24 * 60 * 60, false]);
  await says("Suspension is in effect now.");
});

test("an action in effect is lifted with the moderators' reason, the server told, and is no longer offered to be lifted", async () => {
  const { app, id } = await atItem(
    (action, account) => `/moderation/accounts/${account("quickdeals")}/actions/${action}`,
    "action:quickdeals:limit",
  );
  const lifted = app.fake.script("POST /admin/account_actions/{id}/lift");
  const reason = "The campaign's links are gone from every post.";

  await write(app, "Why it is lifted (for the moderators)", reason);
  await app.user.click(main().getByRole("button", { name: "Lift" }));

  const { url, body } = await lifted.arrived();
  expect(url.pathname).toEndWith(`/admin/account_actions/${id}/lift`);
  expect(body?.reason).toBe(reason);
  await says("Limit is lifted.");
  expect(facts("The action")).toMatch(/State\s*Lifted/);
  expect(main().queryAllByRole("button", { name: "Lift" })).toHaveLength(0);
});

// A ban.

test("a ban is asked with the account's handle typed, says that only an appeal can overturn it, and goes to the server with no expiry and the email blocked", async () => {
  const { app, id } = await atAccount("quickdeals");
  const taken = app.fake.script("POST /admin/accounts/{id}/actions");

  await choose(app, "Action", "Ban");
  // A ban lasts until its account is deleted: it has no expiry to choose.
  expect(main().queryAllByRole("combobox", { name: "Lasts" })).toHaveLength(0);
  await app.user.click(main().getByRole("checkbox", { name: "Also block sign-ups with this account's email address" }));
  await app.user.click(main().getByRole("button", { name: "Ban" }));

  const element = await confirmationElement("Ban @quickdeals?");
  const asking = within(element);
  expect(words(element)).toContain("A ban cannot be lifted: only an appeal can overturn it.");
  const confirm = asking.getByRole("button", { name: "Ban" });
  const phrase = asking.getByRole("textbox");
  expect(pressable(confirm)).toBe(false);
  await app.user.type(phrase, "@quickdeal");
  expect(pressable(confirm)).toBe(false);
  await app.user.type(phrase, "s");
  expect(pressable(confirm)).toBe(true);
  expect(await seenAtRest(app, taken)).toBe(0);
  await app.user.click(confirm);

  const { url, body } = await taken.arrived();
  expect(url.pathname).toEndWith(`/admin/accounts/${id}/actions`);
  expect([body.kind, body.expires_in, body.block_email]).toEqual(["ban", undefined, true]);
  await says("Ban is in effect now.");
  await waitFor(() => {
    if (!/State\s*Banned/.test(facts("The account"))) throw new Error("test: the account is not said to be banned");
  });
});

test("a ban asked and cancelled sends nothing", async () => {
  const { app } = await atAccount("quickdeals");
  const taken = app.fake.script("POST /admin/accounts/{id}/actions");

  await choose(app, "Action", "Ban");
  await app.user.click(main().getByRole("button", { name: "Ban" }));
  const asking = await confirmation("Ban @quickdeals?");
  await app.user.type(asking.getByRole("textbox"), "@quickdeals");
  await app.user.click(asking.getByRole("button", { name: "Cancel" }));

  await waitFor(() => {
    if (screen.queryAllByRole("alertdialog").length + screen.queryAllByRole("dialog").length > 0)
      throw new Error("test: the confirmation is still open");
  });
  expect(await seenAtRest(app, taken)).toBe(0);
});

test("a ban's page says it is permanent and offers no Lift, only the way to its appeal, whose overturning lifts it", async () => {
  const { app, id } = await atItem(
    (action, account) => `/moderation/accounts/${account("cheapfollowers")}/actions/${action}`,
    "action:ban",
  );
  const appeal = app.fake.moderation.id("appeal:ban");
  const overturned = app.fake.script("POST /admin/appeals/{id}/overturn");

  expect(facts("The action")).toMatch(/State\s*Permanent/);
  await says("A ban cannot be lifted. Only an appeal can overturn it, until the account is deleted.");
  expect(main().queryAllByRole("button", { name: "Lift" })).toHaveLength(0);
  await app.user.click(main().getByRole("link", { name: "Open its appeal" }));
  await app.arrived(`/moderation/appeals/${appeal}`);
  await main().findByRole("heading", { level: 1, name: "Appeal of @cheapfollowers" });

  await app.user.click(main().getByRole("button", { name: "Overturn" }));
  await app.user.click(
    (await confirmation("Overturn the original decision?")).getByRole("button", { name: "Overturn" }),
  );

  expect((await overturned.arrived()).url.pathname).toEndWith(`/admin/appeals/${appeal}/overturn`);
  await says("The action is overturned and lifted.");
  const action = await app.data.api.GET("/admin/account_actions/{id}", { params: { path: { id } } });
  expect(action.data?.state).toBe("lifted");
});

// Appeals.

for (const [decision, path, outcome] of [
  ["Uphold", "uphold", "The action is upheld."],
  ["Overturn", "overturn", "The action is overturned and lifted."],
] as const)
  test(`a moderator decides an appeal by "${decision}" with the reason they are told, after a confirmation, the server told`, async () => {
    const { app, id } = await atItem((appeal) => `/moderation/appeals/${appeal}`, "appeal:kai");
    const decided = app.fake.script(`POST /admin/appeals/{id}/${path}`);
    const reason = "We listened to the room's recording.";

    await write(app, "Reason you tell them", reason);
    await app.user.click(main().getByRole("button", { name: decision }));
    const asking = await confirmation(`${decision} the original decision?`);
    expect(decided.seen).toHaveLength(0);
    await app.user.click(asking.getByRole("button", { name: decision }));

    const { url, body } = await decided.arrived();
    expect(url.pathname).toEndWith(`/admin/appeals/${id}/${path}`);
    expect(body?.public_reason).toBe(reason);
    await says(outcome);
    expect(facts("The appeal")).toMatch(new RegExp(`State\\s*${decision === "Uphold" ? "Upheld" : "Overturned"}`));
  });

test("upholding a ban's appeal says the ban becomes final, is confirmed by typing the account's handle, and reaches the server", async () => {
  const { app, id } = await atItem((appeal) => `/moderation/appeals/${appeal}`, "appeal:ban");
  const upheld = app.fake.script("POST /admin/appeals/{id}/uphold");

  await says("Upholding the original decision makes the ban final: the account is deleted on");
  await app.user.click(main().getByRole("button", { name: "Uphold" }));
  const element = await confirmationElement("Uphold the original decision?");
  const asking = within(element);
  expect(words(element)).toMatch(
    /The ban becomes final and the account is deleted on .+; they are told\. This cannot be undone\./,
  );
  const confirm = asking.getByRole("button", { name: "Uphold" });
  expect(pressable(confirm)).toBe(false);
  await app.user.type(asking.getByRole("textbox"), "@cheapfollowers");
  expect(pressable(confirm)).toBe(true);
  await app.user.click(confirm);

  expect((await upheld.arrived()).url.pathname).toEndWith(`/admin/appeals/${id}/uphold`);
  await says("The action is upheld.");
});

// Management.

test("an account's email and IP addresses are asked for only once shown, and are shown with its sign-ins", async () => {
  const { app, id } = await atAccount("kai", { viewer: "hana", page: "/manage" });
  const revealed = app.fake.script("POST /admin/accounts/{id}/reveal");

  expect(await seenAtRest(app, revealed)).toBe(0);
  expect(words(screen.getByRole("main"))).not.toContain("kai@demo.test");
  expect(words(screen.getByRole("main"))).not.toContain("203.0.113.52");
  const [show] = within(main().getByLabelText("Email and IP addresses")).getAllByRole("button", { name: "Show" });
  if (show === undefined) throw new Error("test: nothing to show");
  await app.user.click(show);

  expect((await revealed.arrived()).url.pathname).toEndWith(`/admin/accounts/${id}/reveal`);
  await waitFor(() => {
    const shown = facts("Email and IP addresses");
    if (!shown.includes("kai@demo.test") || !shown.includes("203.0.113.52"))
      throw new Error("test: the email and the IP address are not shown");
  });
  const signIns = within(main().getByRole("table", { name: "Sign-ins" }));
  expect(signIns.getAllByRole("rowheader").map((cell) => words(cell))).toEqual([
    "2001:db8:85a3::8a2e:370:7334",
    "203.0.113.52",
  ]);
});

test("an administrator issues a password reset link after a confirmation, and the screen shows the link once", async () => {
  const { app, id } = await atAccount("kai", { viewer: "hana", page: "/manage" });
  const issued = app.fake.script("POST /admin/accounts/{id}/password_reset_link");

  await app.user.click(main().getByRole("button", { name: "Issue a password reset link…" }));
  const asking = await confirmation("Issue a password reset link for @kai?");
  expect(issued.seen).toHaveLength(0);
  await app.user.click(asking.getByRole("button", { name: "Issue" }));

  expect((await issued.arrived()).url.pathname).toEndWith(`/admin/accounts/${id}/password_reset_link`);
  await says("The link is issued. It is shown only now.");
  expect(words(main().getByRole("list", { name: "Password reset link" }))).toMatch(
    /^https:\/\/demo\.test\/auth\/password\/reset\?token=\S+$/,
  );
});

test("an administrator removes an account's two-factor authentication after a confirmation, and it is no longer offered", async () => {
  const { app, id } = await atAccount("kai", { viewer: "hana", page: "/manage" });
  const removed = app.fake.script("POST /admin/accounts/{id}/two_factor/disable");

  await app.user.click(main().getByRole("button", { name: "Remove two-factor authentication…" }));
  const asking = await confirmation("Remove two-factor authentication of @kai?");
  await app.user.click(asking.getByRole("button", { name: "Remove" }));

  expect((await removed.arrived()).url.pathname).toEndWith(`/admin/accounts/${id}/two_factor/disable`);
  await says("@kai signs in without two-factor authentication now.");
  expect(main().queryAllByRole("button", { name: "Remove two-factor authentication…" })).toHaveLength(0);
});

test("sign-in recovery is offered only to a holder of manage_login_recovery, whatever else the reader may do", async () => {
  const on = fakeApp({ viewer: "hana" });
  on.fake.script("GET /accounts/verify_credentials", {
    change: (account) => ({
      ...account,
      permissions: account.permissions.filter((permission) => permission !== "manage_login_recovery"),
    }),
  });
  const id = on.fake.moderation.id("account:kai");
  await renderApp(`/moderation/accounts/${id}/manage`, { on });

  await main().findByRole("heading", { level: 2, name: "Email and IP addresses" });
  await waitFor(() => {
    if (on.data.queryClient.isFetching() > 0) throw new Error("test: the screen's reads are on their way");
  });
  expect(main().queryAllByRole("button", { name: /^(Issue a password reset link|Remove two-factor)/ })).toHaveLength(0);
});

test("an email address whose delivery stopped is sent email again, the server told", async () => {
  const { app, id } = await atAccount("sam", { viewer: "hana", page: "/manage" });
  const lifted = app.fake.script("POST /admin/accounts/{id}/email_suppression/lift");

  await app.user.click(main().getByRole("button", { name: "Send email again" }));

  expect((await lifted.arrived()).url.pathname).toEndWith(`/admin/accounts/${id}/email_suppression/lift`);
  await waitFor(() => {
    if (main().queryAllByRole("button", { name: "Send email again" }).length > 0)
      throw new Error("test: Send email again is still offered");
  });
});

test("a remote account's posts are fetched again from its server, the server told", async () => {
  const { app, id } = await atAccount("sora@photos.example", { viewer: "hana", page: "/manage" });
  const fetched = app.fake.script("POST /admin/accounts/{id}/refetch_statuses");

  await app.user.click(main().getByRole("button", { name: "Fetch the posts again" }));

  expect((await fetched.arrived()).url.pathname).toEndWith(`/admin/accounts/${id}/refetch_statuses`);
  await says("The posts are fetched again in the background, 100 a minute.");
});

test("a remote account's posts are not offered to be fetched again to a reader without manage_domains", async () => {
  const on = fakeApp({ viewer: "marco" });
  on.fake.script("GET /accounts/verify_credentials", {
    change: (account) => ({
      ...account,
      permissions: account.permissions.filter((permission) => permission !== "manage_domains"),
    }),
  });
  await renderApp(`/moderation/accounts/${on.fake.moderation.id("account:sora@photos.example")}/manage`, { on });

  await main().findByRole("heading", { level: 1, name: "@sora@photos.example" });
  await waitFor(() => {
    if (on.data.queryClient.isFetching() > 0) throw new Error("test: the screen's reads are on their way");
  });
  expect(main().queryAllByRole("button", { name: "Fetch the posts again" })).toHaveLength(0);
});
