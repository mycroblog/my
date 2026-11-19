// SPDX-License-Identifier: AGPL-3.0-only

// The server's settings in the app (#3262, FR-001, FR-010, FR-705, FR-724, FR-725, FR-922, ADR-0058 §1), on the fake
// API, as what follows a press: Server settings lists its ten pages; a page saves the settings changed on it, and only
// those, in one PATCH, in the units the server keeps (hours, minutes and MiB shown for seconds and bytes), and its texts
// one key at a time; a key the server refuses is marked under its field; every form asks before its changes are left;
// switching federation to the allowlist, limiting sign-in to the login providers and a maximum age of posts say what
// they end, and ask, before they are saved; a one-person server has no sign-up to set; the icon and logo show the
// built-in pictures, a chosen one before it is saved, and a retry that sends only what failed; translation shows this
// month's use against its limit; and each administration entry opens only to its permission. What each screen shows at
// rest is the stories' (admin.stories.tsx); the rules, relays, login providers and OAuth apps are
// admin-connections.dom.test.tsx's. A new picture saved is the E2E spec's (admin-settings.spec.ts): its save sends the
// PNG renditions the browser draws (SetBrandRequest), and the DOM simulation has no canvas to draw them on.

import { expect, test } from "bun:test";
import type { FakeOptions } from "@my/e2e/fake";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { chooseFiles } from "./test/files";
import { fakeApp } from "./test/server";

/** The ten pages of Server settings, in the order its list gives them. */
const pages = [
  "general",
  "branding",
  "registration",
  "content",
  "voice",
  "auto-delete",
  "federation",
  "discovery",
  "hooks",
  "records",
] as const;

/** Waits until the app has no read on its way. */
const settled = (app: TestApp) =>
  waitFor(() => {
    if (app.data.queryClient.isFetching() > 0) throw new Error("test: the screen's reads are on their way");
  });

/**
 * The app at `at` as hana, the scenario's administrator, reads it on a server of `options`, once its form is drawn
 * and read; the saves of the settings counted.
 */
async function atForm(at: string, options: FakeOptions = {}) {
  const on = fakeApp({ viewer: "hana", ...options });
  const patch = on.fake.script("PATCH /admin/settings");
  const app = await renderApp(at, { on });
  await main().findByRole("button", { name: "Save" });
  await settled(app);
  return { app, patch };
}

/** A field of the screen by its name: a text or number field, a select, or a radio by the words it starts with. */
const textbox = (name: string) => main().getByRole("textbox", { name });
const select = (name: string) => main().getByRole("combobox", { name });
const radio = (starts: string) => {
  const [first] = main().getAllByRole("radio", { name: new RegExp(`^${starts}`) });
  if (first === undefined) throw new Error(`test: no choice starts with ${starts}`);
  return first;
};

/** Replaces what the field of `name` holds with `value`, as a reader types it. */
async function typeIn(app: TestApp, name: string, value: string) {
  const field = textbox(name);
  await app.user.clear(field);
  await app.user.type(field, value);
}

/** Presses the form's Save. */
const save = (app: TestApp) => app.user.click(main().getByRole("button", { name: "Save" }));

/** Waits until a status of the page says `sentence`. */
const says = (sentence: string) =>
  waitFor(() => {
    if (!screen.queryAllByRole("status").some((status) => words(status) === sentence))
      throw new Error(`test: no status says ${JSON.stringify(sentence)}`);
  });

/** The value of the setting of `key` as the fake holds it now, read as a client does. */
async function held(app: TestApp, key: string) {
  const { data } = await app.data.api.GET("/admin/settings");
  return data?.items.find((setting) => setting.key === key)?.value;
}

/** What describes `field`: the words of the elements its `aria-describedby` names. */
const describedBy = (field: HTMLElement) =>
  (field.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .flatMap((id) => {
      const part = document.getElementById(id);
      return part === null ? [] : [words(part)];
    })
    .join(" ");

/** The fields of the screen marked as refused. */
const marked = () =>
  main()
    .getAllByRole("textbox")
    .filter((box) => box.getAttribute("aria-invalid") === "true");

/** The shell's link to Home, which leaves the screen in place. */
const home = () => {
  const [link] = screen.getAllByRole("link", { name: "Home" });
  if (link === undefined) throw new Error("test: the shell has no link to Home");
  return link;
};

// Server settings and its pages.

test("Server settings lists its ten pages, each a link to its own address", async () => {
  const app = await renderApp("/admin/settings", { viewer: "hana" });

  const list = await main().findByRole("navigation", { name: "Server settings" });
  await settled(app);
  expect(
    within(list)
      .getAllByRole("link")
      .map((link) => link.getAttribute("href")),
  ).toEqual(pages.map((page) => `/admin/settings/${page}`));
});

type Edit = { type: string; into: string } | { choose: string; in: string } | { radio: string };

/** What a reader changes on a page, what the page showed before, and the values the server is sent for it. */
const changes: readonly {
  page: (typeof pages)[number];
  edits: readonly Edit[];
  shown?: Readonly<Record<string, string>>;
  sent: Readonly<Record<string, string>>;
}[] = [
  {
    page: "registration",
    shown: { "Longest an invite works": "720" },
    edits: [{ type: "48", into: "Longest an invite works" }, { radio: "Nobody" }],
    sent: { member_invite_max_lifetime_seconds: String(48 * 3600), registration_mode: "closed" },
  },
  {
    page: "federation",
    shown: { "Keep copies for": "1440", "Most space for copies": "1024" },
    edits: [
      { type: "30", into: "Keep copies for" },
      { type: "2048", into: "Most space for copies" },
    ],
    sent: { remote_media_cache_ttl_seconds: String(30 * 60), remote_media_cache_max_bytes: String(2048 * 1_048_576) },
  },
  {
    page: "content",
    shown: { "Soonest a post can be scheduled": "5" },
    edits: [
      { type: "15", into: "Soonest a post can be scheduled" },
      { choose: "On", in: "Media needs a description" },
    ],
    sent: { scheduled_post_min_lead_minutes: "15", require_media_description: "true" },
  },
  {
    page: "voice",
    edits: [{ type: "60", into: "Keep who took part" }, { radio: "Only with the permission" }],
    sent: { voice_participants_retention_days: "60", voice_room_starters: "permitted" },
  },
  {
    page: "auto-delete",
    edits: [{ type: "60", into: "Delete posts older than" }],
    sent: { auto_delete_default_min_age_days: "60" },
  },
  {
    page: "discovery",
    edits: [{ type: "mailto:security@demo.example", into: "Security contact" }, { radio: "Refuse training" }],
    sent: { security_contact: "mailto:security@demo.example", ai_usage: "deny_training" },
  },
  {
    page: "hooks",
    edits: [{ type: "45", into: "Keep decisions" }],
    sent: { hook_decisions_retention_days: "45" },
  },
  {
    page: "records",
    edits: [{ type: "30", into: "Appeals can be made for" }],
    sent: { appeal_window_days: "30" },
  },
  {
    page: "general",
    edits: [{ choose: "日本語", in: "Default language" }],
    sent: { default_locale: "ja" },
  },
];

for (const { page, edits, shown = {}, sent } of changes)
  test(`${page} saves the settings changed on it, and only those, in one request in the server's units`, async () => {
    const { app, patch } = await atForm(`/admin/settings/${page}`);
    for (const [name, value] of Object.entries(shown)) expect(textbox(name)).toHaveProperty("value", value);

    for (const edit of edits) {
      if ("type" in edit) await typeIn(app, edit.into, edit.type);
      else if ("choose" in edit) await app.user.selectOptions(select(edit.in), edit.choose);
      else await app.user.click(radio(edit.radio));
    }
    await save(app);

    await says("Saved");
    expect(patch.seen.map((request) => request.body.values)).toEqual([sent]);
    for (const [key, value] of Object.entries(sent)) expect([key, await held(app, key)]).toEqual([key, value]);
  });

test("General saves a text it changed by its own key, apart from the settings", async () => {
  const on = fakeApp({ viewer: "hana" });
  const patch = on.fake.script("PATCH /admin/settings");
  const texts = on.fake.script("PUT /admin/instance_texts/{key}");
  const app = await renderApp("/admin/settings/general", { on });
  await main().findByRole("button", { name: "Save" });
  await settled(app);

  await typeIn(app, "Name", "River town");
  await app.user.selectOptions(select("Default language"), "日本語");
  await save(app);

  await says("Saved");
  expect(texts.seen.map(({ url, body }) => [url.pathname, body.body])).toEqual([
    ["/api/v1/admin/instance_texts/name", "River town"],
  ]);
  expect(patch.seen.map((request) => request.body.values)).toEqual([{ default_locale: "ja" }]);
});

for (const [document, key] of [
  ["terms", "terms"],
  ["privacy-policy", "privacy_policy"],
  ["commerce-disclosure", "commerce_disclosure"],
] as const)
  test(`the document at ${document} is saved as the text ${key}, alone`, async () => {
    const on = fakeApp({ viewer: "hana" });
    const patch = on.fake.script("PATCH /admin/settings");
    const texts = on.fake.script("PUT /admin/instance_texts/{key}");
    const app = await renderApp(`/admin/documents/${document}`, { on });
    await main().findByRole("button", { name: "Save" });
    await settled(app);
    const [field] = main().getAllByRole("textbox");
    if (field === undefined) throw new Error("test: the document has no text field");

    await app.user.clear(field);
    await app.user.type(field, "## Written here");
    await save(app);

    await says("Saved");
    expect(texts.seen.map(({ url, body }) => [url.pathname, body.body])).toEqual([
      [`/api/v1/admin/instance_texts/${key}`, "## Written here"],
    ]);
    expect(patch.seen).toHaveLength(0);
  });

test("a value the server refuses is marked under its field, and nothing is saved", async () => {
  const { app, patch } = await atForm("/admin/settings/discovery");

  await typeIn(app, "Security contact", "http://security.example");
  await app.user.click(radio("Refuse training"));
  await save(app);

  await says("Nothing was saved. Fix what is marked and save again.");
  const field = textbox("Security contact");
  expect(describedBy(field)).toContain("Enter an https: or mailto: address, or leave it empty.");
  expect(marked()).toEqual([field]);
  expect(patch.seen).toHaveLength(1);
  expect([await held(app, "security_contact"), await held(app, "ai_usage")]).toEqual(["", "allow"]);
});

test("a refusal of a setting shown in other units is marked under the field that shows it", async () => {
  const on = fakeApp({ viewer: "hana" });
  on.fake.script("PATCH /admin/settings", {
    fail: {
      type: "about:blank",
      title: "Unprocessable Content",
      status: 422,
      detail: "This value is not valid.",
      field: "values.member_invite_max_lifetime_seconds",
    },
  });
  const app = await renderApp("/admin/settings/registration", { on });
  await main().findByRole("button", { name: "Save" });
  await settled(app);

  await typeIn(app, "Longest an invite works", "48");
  await typeIn(app, "Usable invites per person", "7");
  await save(app);

  await waitFor(() => {
    if (marked().length === 0) throw new Error("test: no field is marked");
  });
  expect(marked()).toEqual([textbox("Longest an invite works")]);
});

// What is asked before a change is left or saved.

/** Each form of the settings, and what a reader changes on it. */
const forms: readonly [string, (app: TestApp) => Promise<void>, FakeOptions?][] = [
  ["/admin/settings/general", (app) => typeIn(app, "Name", "River town")],
  [
    "/admin/settings/branding",
    (app) =>
      app.user.click(
        within(main().getByRole("group", { name: "Logo" })).getByRole("button", { name: "Use the built-in image" }),
      ),
    { logo: true },
  ],
  ["/admin/settings/registration", (app) => typeIn(app, "Usable invites per person", "7")],
  ["/admin/settings/content", (app) => typeIn(app, "Longest post", "400")],
  ["/admin/settings/voice", (app) => typeIn(app, "Keep who took part", "60")],
  ["/admin/settings/auto-delete", (app) => typeIn(app, "Delete posts older than", "60")],
  ["/admin/settings/federation", (app) => typeIn(app, "Keep copies for", "30")],
  ["/admin/settings/discovery", (app) => typeIn(app, "Security contact", "mailto:a@demo.example")],
  ["/admin/settings/hooks", (app) => typeIn(app, "Keep decisions", "45")],
  ["/admin/settings/records", (app) => typeIn(app, "Audit log", "800")],
  ["/admin/translation", (app) => typeIn(app, "Keep translations", "9")],
  ["/admin/documents/terms", (app) => typeIn(app, "Terms of service", "## New terms")],
];

for (const [at, change, options] of forms)
  test(`the form at ${at} asks before its changes are left, and leaves on Leave`, async () => {
    const { app } = await atForm(at, options);

    await change(app);
    await app.user.click(home());

    const asking = within(await screen.findByRole("alertdialog", { name: "Leave without saving?" }));
    expect(app.path()).toBe(at);
    await app.user.click(asking.getByRole("button", { name: "Leave" }));
    await app.arrived("/");
  });

test("Keep editing stays on the form with its changes, and a form left unchanged is left without a question", async () => {
  const { app } = await atForm("/admin/translation");
  await typeIn(app, "Keep translations", "9");
  await app.user.click(home());

  await app.user.click(
    within(await screen.findByRole("alertdialog", { name: "Leave without saving?" })).getByRole("button", {
      name: "Keep editing",
    }),
  );

  await waitFor(() => {
    if (screen.queryAllByRole("alertdialog").length > 0) throw new Error("test: the question is still open");
  });
  expect(app.path()).toBe("/admin/translation");
  expect(textbox("Keep translations")).toHaveProperty("value", "9");

  cleanup();
  const unchanged = await atForm("/admin/settings/records");
  await unchanged.app.user.click(home());
  await unchanged.app.arrived("/");
  expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
});

test("the allowlist says before it is chosen that it ends follows for good, and Save asks with what it ends", async () => {
  const on = fakeApp({ viewer: "hana" });
  const patch = on.fake.script("PATCH /admin/settings");
  const impact = on.fake.script("GET /admin/settings/federation_scope/impact");
  const app = await renderApp("/admin/settings/federation", { on });
  await main().findByRole("button", { name: "Save" });
  await settled(app);
  const allowlist = radio("Allowed servers only");
  expect(words(allowlist.closest("label") ?? allowlist)).toContain(
    "Ends the follows with every other server. This cannot be undone.",
  );

  await app.user.click(allowlist);
  await save(app);
  const first = within(await screen.findByRole("alertdialog", { name: "Federate only with allowed servers?" }));
  expect(impact.seen.length).toBeGreaterThan(0);
  expect(words(screen.getByRole("alertdialog"))).toMatch(
    /37 follows with 5 other servers end, and 12 accounts here lose some\. Most are with mastodon\.example, photos\.example, misskey\.example\..*This cannot be undone\./,
  );
  await app.user.click(first.getByRole("button", { name: "Cancel" }));
  expect(patch.seen).toHaveLength(0);

  await save(app);
  await app.user.click(
    within(await screen.findByRole("alertdialog", { name: "Federate only with allowed servers?" })).getByRole(
      "button",
      { name: "Switch" },
    ),
  );

  await says("Saved");
  expect(patch.seen.map((request) => request.body.values)).toEqual([{ federation_scope: "allowlist" }]);
});

test("limiting sign-in to the login providers says who it keeps out, and asks, before it is saved", async () => {
  const { app, patch } = await atForm("/admin/settings/registration");

  await app.user.selectOptions(select("Sign in only with login providers"), "On");
  await save(app);

  const asking = await screen.findByRole("alertdialog", { name: "Limit sign-in to login providers?" });
  expect(words(asking)).toContain(
    "While it applies, 14 active people without a linked provider cannot sign in again once their sessions end. You are one of them.",
  );
  expect(patch.seen).toHaveLength(0);
  await app.user.click(within(asking).getByRole("button", { name: "Limit sign-in" }));

  await says("Saved");
  expect(patch.seen.map((request) => request.body.values)).toEqual([{ login_providers_only: "true" }]);
});

test("a maximum age of everyone's posts says what it deletes for good, and asks, before it is saved", async () => {
  const { app, patch } = await atForm("/admin/settings/auto-delete");

  await typeIn(app, "Keep posts at most", "400");
  await save(app);

  const asking = await screen.findByRole("alertdialog", { name: "Delete old posts of everyone?" });
  expect(words(asking)).toContain(
    "Posts of this server older than 400 days are deleted, starting 30 days after each person is told. This cannot be undone.",
  );
  await app.user.click(within(asking).getByRole("button", { name: "Cancel" }));
  expect(patch.seen).toHaveLength(0);
  await save(app);
  await app.user.click(
    within(await screen.findByRole("alertdialog", { name: "Delete old posts of everyone?" })).getByRole("button", {
      name: "Save and delete",
    }),
  );

  await says("Saved");
  expect(patch.seen.map((request) => request.body.values)).toEqual([{ auto_delete_max_age_days: "400" }]);
});

test("each way of signing up says what it does before it is chosen, and a mode is saved without a question", async () => {
  const { app, patch } = await atForm("/admin/settings/registration");
  const group = within(main().getByRole("radiogroup", { name: "Who can sign up" }));
  expect(group.getAllByRole("radio").map((choice) => words(choice.closest("label") ?? choice))).toEqual([
    // Each choice's words, then what it does: the label holds both, the second its description.
    "AnyoneAnyone can create an account and use it at once.",
    "After a moderator approvesA sign-up waits for a moderator to approve it, unless it comes with an invite.",
    "Only with an inviteA sign-up needs an invite from a moderator or a member.",
    "NobodyOnly the administrators can create accounts.",
  ]);

  await app.user.click(radio("Only with an invite"));
  await save(app);

  await says("Saved");
  expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
  expect(patch.seen.map((request) => request.body.values)).toEqual([{ registration_mode: "invite" }]);
});

test("a one-person server shows no way of signing up nor member invites, and says why", async () => {
  await atForm("/admin/settings/registration", { singleUser: true });

  expect(main().queryAllByRole("radiogroup", { name: "Who can sign up" })).toHaveLength(0);
  expect(main().queryAllByRole("combobox", { name: "Who can create invites" })).toHaveLength(0);
  expect(main().queryAllByRole("textbox", { name: "Usable invites per person" })).toHaveLength(0);
  expect(
    main().getAllByText("This is a one-person server: nobody else can sign up. Turn it off below to let others join."),
  ).toHaveLength(2);
  expect(select("One-person server")).toBeDefined();
});

/** The words of the options of `box`, a select. */
const optionsOf = (box: HTMLElement) => (box instanceof HTMLSelectElement ? [...box.options].map((o) => o.text) : []);

test("each yes-or-no setting is one control that is On or Off, on every page", async () => {
  for (const at of [...pages.map((page) => `/admin/settings/${page}`), "/admin/translation"]) {
    await atForm(at);
    const yesNo = main()
      .queryAllByRole("combobox")
      .filter((box) => optionsOf(box).some((option) => /^(On|Yes)$/.test(option)));
    expect([at, yesNo.map(optionsOf).filter((options) => options.join() !== "On,Off")]).toEqual([at, []]);
    expect([
      at,
      main()
        .queryAllByRole("radio")
        .filter((choice) => /^(On|Off|Yes|No)$/.test(words(choice.closest("label") ?? choice))),
    ]).toEqual([at, []]);
    expect([at, main().queryAllByRole("switch")]).toEqual([at, []]);
    cleanup();
  }
});

// The icon and logo.

/** The group of the brand picture `name` (Icon, Logo, Logo for dark backgrounds). */
const brand = (name: string) => within(main().getByRole("group", { name }));

/** The addresses of the pictures the group of `name` shows, which its words describe. */
const pictures = (name: string) =>
  brand(name)
    .queryAllByRole("presentation", { hidden: true })
    .flatMap((image) => (image instanceof HTMLImageElement ? [image.getAttribute("src")] : []));

/** A square SVG. */
const squareSvg = () =>
  new File(
    [`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><rect width="100" height="100"/></svg>`],
    "mark.svg",
    { type: "image/svg+xml" },
  );

test("a server with no pictures of its own shows the built-in ones, with nothing to put back", async () => {
  await atForm("/admin/settings/branding");

  for (const name of ["Icon", "Logo", "Logo for dark backgrounds"]) {
    expect([name, pictures(name).length]).toEqual([name, 1]);
    expect([name, brand(name).queryAllByRole("button", { name: "Use the built-in image" })]).toEqual([name, []]);
  }
});

test("a picture of the server's own offers the built-in one back, and the built-in icon offers nothing", async () => {
  await atForm("/admin/settings/branding", { logo: true });

  expect(brand("Icon").queryAllByRole("button", { name: "Use the built-in image" })).toHaveLength(0);
  expect(brand("Logo").getAllByRole("button", { name: "Use the built-in image" })).toHaveLength(1);
  expect(brand("Logo for dark backgrounds").getAllByRole("button", { name: "Use the built-in image" })).toHaveLength(1);
});

test("an icon chosen is checked by the server and shown before it is saved, with the color behind it", async () => {
  const on = fakeApp({ viewer: "hana" });
  const checked = on.fake.script("POST /admin/instance/brand/sanitize");
  const app = await renderApp("/admin/settings/branding", { on });
  await main().findByRole("button", { name: "Save" });
  await settled(app);
  const before = pictures("Icon");

  await chooseFiles(app, brand("Icon").getByRole("button", { name: "Choose an SVG file" }), squareSvg());

  await waitFor(() => {
    if (brand("Icon").queryAllByRole("button", { name: "Use the built-in image" }).length === 0)
      throw new Error("test: the icon is not shown as chosen");
  });
  expect(checked.seen).toHaveLength(1);
  expect(pictures("Icon")).toHaveLength(1);
  expect(pictures("Icon")).not.toEqual(before);
  expect(textbox("Color behind the icon")).toHaveProperty("value", "#ffffff");
});

test("a save of two pictures whose second failed sends only that one again", async () => {
  const on = fakeApp({ viewer: "hana", logo: true });
  const all = on.fake.script("DELETE /admin/instance/brand/{kind}");
  const first = on.fake.script("DELETE /admin/instance/brand/{kind}", { hold: "request", times: 1 });
  const second = on.fake.script("DELETE /admin/instance/brand/{kind}", { fail: 500, times: 1 });
  const app = await renderApp("/admin/settings/branding", { on });
  await main().findByRole("button", { name: "Save" });
  await settled(app);
  await app.user.click(brand("Logo").getByRole("button", { name: "Use the built-in image" }));
  await app.user.click(brand("Logo for dark backgrounds").getByRole("button", { name: "Use the built-in image" }));

  await save(app);
  await first.arrived();
  first.release();
  await waitFor(() => {
    if (screen.queryAllByRole("alert").length === 0) throw new Error("test: the page says nothing of the failure");
  });
  expect(second.seen).toHaveLength(2);
  await save(app);

  await says("Saved");
  const [done, failed, again, ...more] = all.seen.map(({ url }) => url.pathname);
  expect(new Set([done, failed])).toEqual(
    new Set(["/api/v1/admin/instance/brand/logo", "/api/v1/admin/instance/brand/logo_dark"]),
  );
  expect([again, more]).toEqual([failed, []]);
});

// Translation.

test("translation shows this month's characters against the server's limit, and follows a new limit once saved", async () => {
  const { app, patch } = await atForm("/admin/translation", { translation: { provider: "answers" } });
  const month = within(main().getByRole("list", { name: "This month" }));
  expect(month.getAllByRole("listitem").map(words)).toContain("128,400 of 500,000 characters for the whole server");

  await typeIn(app, "Characters for the whole server a month", "600000");
  await app.user.selectOptions(select("Readers can use their browser's translation"), "On");
  await save(app);

  await says("Saved");
  expect(patch.seen.map((request) => request.body.values)).toEqual([
    { translation_server_monthly_characters: "600000", translation_browser: "true" },
  ]);
  await waitFor(() => {
    const items = within(main().getByRole("list", { name: "This month" }))
      .getAllByRole("listitem")
      .map(words);
    if (!items.includes("128,400 of 600,000 characters for the whole server"))
      throw new Error(`test: this month reads ${items.join(" / ")}`);
  });
});

// Who each entry opens to.

/** The addresses the root's list of administration screens leads to. */
const listed = () =>
  main()
    .queryAllByRole("navigation", { name: "Administration screens" })
    .flatMap((nav) =>
      within(nav)
        .queryAllByRole("link")
        .map((link) => link.getAttribute("href")),
    );

const everyEntry = [
  "/admin/settings",
  "/admin/documents",
  "/admin/rules",
  "/admin/translation",
  "/admin/relays",
  "/admin/login-providers",
  "/admin/oauth-apps",
  "/admin/finance",
  "/admin/supporters",
];

for (const [permission, entries, address, read] of [
  [
    "manage_settings",
    ["/admin/settings", "/admin/documents", "/admin/translation", "/admin/relays"],
    "/admin/settings/general",
    "GET /admin/settings",
  ],
  ["manage_rules", ["/admin/rules"], "/admin/rules", "GET /admin/rules"],
  ["manage_login_providers", ["/admin/login-providers"], "/admin/login-providers", "GET /admin/login_providers"],
  ["manage_oauth_apps", ["/admin/oauth-apps"], "/admin/oauth-apps", "GET /admin/oauth_clients"],
] as const)
  test(`without ${permission}, the root lists none of its screens and its address opens none, asking the server nothing`, async () => {
    const on = fakeApp({ viewer: "hana" });
    on.fake.script("GET /accounts/verify_credentials", {
      change: (account) => ({ ...account, permissions: account.permissions.filter((p) => p !== permission) }),
    });
    const asked = on.fake.script(read);
    const app = await renderApp("/admin", { on });
    await main().findByRole("navigation", { name: "Administration screens" });
    await settled(app);
    expect(listed()).toEqual(everyEntry.filter((entry) => !entries.some((gone) => gone === entry)));

    await app.navigate(address);

    expect(await main().findByText("This address leads to no screen open to you.", { exact: false })).toBeDefined();
    await settled(app);
    expect(asked.seen).toHaveLength(0);
  });

test("an administrator with every permission is listed every screen", async () => {
  const app = await renderApp("/admin", { viewer: "hana" });
  await main().findByRole("navigation", { name: "Administration screens" });
  await settled(app);
  expect(listed()).toEqual(everyEntry);
});
