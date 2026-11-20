// SPDX-License-Identifier: AGPL-3.0-only

// The custom emoji in the app (#3263, FR-111, FR-915, ADR-0051, ADR-0070), on the fake API, as what follows a press.
// The administration (hana, holding `manage_custom_emojis`): an emoji registered with its image and its place in the
// picker; a new image staged behind Replace…, which asks first; hidden from the picker, disabled and enabled, deleted by
// its shortcode typed; another server's copied; the requests reviewed, approved or refused, the Requests tab counting
// those that wait; and the picker read again past the HTTP cache after each write. The reader who may ask for emoji
// (mio, holding `artist`: `create_custom_emojis` and no `manage_custom_emojis`): a request within the month's limit,
// withdrawn; their own emoji changed and deleted; their requests paged with Show more, and one found by its ID on a
// later page. What each screen shows at rest is the stories' (admin-custom-emojis.stories.tsx,
// settings-custom-emojis.stories.tsx).

import { expect, test } from "bun:test";
import type { FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { chooseFiles, imageFile } from "./test/files";
import { fakeApp } from "./test/server";

const admin: FakeOptions = { viewer: "hana" };
/** mio, holding the role that lets them ask for emoji, as the stories give it. */
const artist = (more: FakeOptions["staffServer"] = {}): FakeOptions => ({
  viewer: "mio",
  staffServer: { ...more, roles: { viewer: ["artist"] } },
});

/** Waits until the app has no read on its way. */
const settled = (app: TestApp) =>
  waitFor(() => {
    if (app.data.queryClient.isFetching() > 0) throw new Error("test: the screen's reads are on their way");
  });

const button = (name: string) => main().getByRole("button", { name });

/** The app on the fake of `options` at the address `at` gives for it, once its screen is read. */
async function opened(at: (on: ReturnType<typeof fakeApp>) => string, options: FakeOptions) {
  const on = fakeApp(options);
  const app = await renderApp(at(on), { on });
  await main().findAllByRole("heading", { level: 2 });
  await settled(app);
  return { on, app };
}

const emojiPage = (key: string) => (on: ReturnType<typeof fakeApp>) =>
  `/admin/custom-emojis/${on.fake.staffServer.id("emoji", key)}`;

/** The question of `name` once it is open. */
const asking = async (name: string | RegExp) => within(await screen.findByRole("alertdialog", { name }));

/** Waits until no question is open. */
const answered = () =>
  waitFor(() => {
    if (screen.queryAllByRole("alertdialog").length > 0) throw new Error("test: a question is still open");
  });

/** What the status regions of the screen say, those that say something. */
const said = () =>
  main()
    .queryAllByRole("status")
    .map(words)
    .filter((text) => text !== "");

/** Waits until a status of the screen says `sentence`. */
const saying = (sentence: string) =>
  waitFor(() => {
    if (!said().includes(sentence)) throw new Error(`test: the statuses say ${said().join(" / ")}`);
  });

/** The emoji of the local emoji with `shortcode` as the server holds it now. */
async function held(app: TestApp, id: string) {
  const { data } = await app.data.api.GET("/admin/custom_emojis/{id}", { params: { path: { id } } });
  return data;
}

// The administration.

test("an emoji registered with its image and its place in the picker reaches the server, and the picker is read again", async () => {
  const on = fakeApp(admin);
  const registered = on.fake.script("POST /admin/custom_emojis");
  const app = await renderApp("/admin/custom-emojis/new", { on });
  await main().findByRole("textbox", { name: "Shortcode" });
  await settled(app);
  const picker = on.fake.script("GET /custom_emojis");

  await app.user.type(main().getByRole("textbox", { name: "Shortcode" }), "garden_gnome");
  await chooseFiles(app, button("Choose an image"), imageFile("gnome.png"));
  await app.user.type(main().getByRole("textbox", { name: "Category" }), "Garden");
  await app.user.type(main().getByRole("textbox", { name: "Aliases" }), "gnome dwarf");
  await app.user.selectOptions(main().getByRole("combobox", { name: "Offer it in the picker" }), "Off");
  await app.user.click(button("Register"));

  expect((await registered.arrived()).body).toMatchObject({
    shortcode: "garden_gnome",
    category: "Garden",
    aliases: ["gnome", "dwarf"],
    visible_in_picker: false,
  });
  expect(await main().findByRole("heading", { level: 2, name: ":garden_gnome:" })).toBeDefined();
  // Whether the read goes past the HTTP cache is the request's `cache`, which the fake does not keep.
  await picker.arrived();
});

test("a new image is staged first, and replaces the emoji's only once Replace… is answered", async () => {
  const { on, app } = await opened(emojiPage("wave"), admin);
  const replaced = on.fake.script("PUT /admin/custom_emojis/{id}/image");

  await chooseFiles(app, button("Choose an image"), imageFile("wave-2.png"));
  expect(replaced.seen).toHaveLength(0);
  await app.user.click(await main().findByRole("button", { name: /^Replace…?$/ }));
  const question = await asking("Replace the image of :wave:?");
  expect(words(screen.getByRole("alertdialog"))).toContain(
    "Every post and profile that uses it shows the new image, and other servers fetch it again. The old image is not kept.",
  );
  await app.user.click(question.getByRole("button", { name: "Keep the image" }));
  await answered();
  expect(replaced.seen).toHaveLength(0);
  await app.user.click(main().getByRole("button", { name: /^Replace…?$/ }));
  await app.user.click((await asking("Replace the image of :wave:?")).getByRole("button", { name: "Replace" }));

  expect((await replaced.arrived()).url.pathname).toEndWith("/image");
  await answered();
});

test("an emoji taken out of the picker is saved, and the picker is read again", async () => {
  const { on, app } = await opened(emojiPage("wave"), admin);
  const changed = on.fake.script("PATCH /admin/custom_emojis/{id}");
  const picker = on.fake.script("GET /custom_emojis");

  await app.user.selectOptions(main().getByRole("combobox", { name: "Offer it in the picker" }), "Off");
  await app.user.click(button("Save"));

  expect((await changed.arrived()).body.visible_in_picker).toBe(false);
  await picker.arrived();
  expect((await held(app, on.fake.staffServer.id("emoji", "wave")))?.visible_in_picker).toBe(false);
});

test("an emoji is disabled, then enabled again from its page", async () => {
  const { on, app } = await opened(emojiPage("wave"), admin);
  const disabled = on.fake.script("POST /admin/custom_emojis/{id}/disable");
  const enabled = on.fake.script("POST /admin/custom_emojis/{id}/enable");

  await app.user.click(button("Disable"));
  await disabled.arrived();
  expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
  await app.user.click(await main().findByRole("button", { name: "Enable" }));

  await enabled.arrived();
  expect(await main().findByRole("button", { name: "Disable" })).toBeDefined();
  expect((await held(app, on.fake.staffServer.id("emoji", "wave")))?.state).toBe("enabled");
});

test("an emoji is deleted only with its shortcode typed, and leaves the list", async () => {
  const { on, app } = await opened(emojiPage("wave"), admin);
  const deleted = on.fake.script("DELETE /admin/custom_emojis/{id}");

  expect(main().getAllByRole("button").at(-1)).toBe(button("Delete"));
  await app.user.click(button("Delete"));
  const question = await asking("Delete :wave:?");
  const confirm = question.getByRole("button", { name: "Delete" });
  const typed = question.getByRole("textbox", { name: "To confirm, enter wave" });
  await app.user.type(typed, "wav");
  expect(confirm.getAttribute("aria-disabled") === "true" || confirm.hasAttribute("disabled")).toBe(true);
  expect(deleted.seen).toHaveLength(0);
  await app.user.type(typed, "e");
  await app.user.click(question.getByRole("button", { name: "Delete" }));

  await deleted.arrived();
  await app.arrived("/admin/custom-emojis");
  await waitFor(() => {
    const listed = within(main().getByRole("table", { name: "Custom emoji" }))
      .queryAllByRole("rowheader")
      .map(words);
    if (listed.length === 0 || listed.some((name) => name.startsWith(":wave:")))
      throw new Error(`test: the list reads ${listed.join(" / ")}`);
  });
});

test("another server's emoji is copied to this server under the shortcode given", async () => {
  const { on, app } = await opened(emojiPage("blobfox"), admin);
  const copied = on.fake.script("POST /admin/custom_emojis/{id}/copy");

  const shortcode = main().getByRole("textbox", { name: "Shortcode of the copy" });
  await app.user.clear(shortcode);
  await app.user.type(shortcode, "our_fox");
  await app.user.click(button("Copy"));

  expect((await copied.arrived()).body).toEqual({ shortcode: "our_fox" });
  expect(await main().findByRole("heading", { level: 2, name: ":our_fox:" })).toBeDefined();
});

/** The row of the request for `shortcode` in the table of requests. */
const requestRow = (shortcode: string) => {
  const row = within(main().getByRole("table", { name: "Requests for custom emoji" }))
    .getAllByRole("row")
    .find((r) =>
      within(r)
        .queryAllByRole("rowheader")
        .some((h) => words(h).startsWith(`:${shortcode}:`)),
    );
  if (row === undefined) throw new Error(`test: no request for ${shortcode}`);
  return row;
};

/** The count the Requests tab shows beside its name. */
const waitingOnTab = () => words(main().getByRole("tab", { name: /^Requests/ }));

test("a request approved from the review leaves the waiting ones, and the Requests tab counts one fewer", async () => {
  const { on, app } = await opened(() => "/admin/custom-emojis/requests", admin);
  const approved = on.fake.script("POST /admin/custom_emoji_requests/{id}/approve");
  expect(waitingOnTab()).toMatch(/^Requests\s*3/);

  await app.user.click(within(requestRow("party_cat")).getByRole("button", { name: "Review" }));
  const review = within(await screen.findByRole("dialog"));
  await app.user.click(review.getByRole("button", { name: "Approve" }));

  expect((await approved.arrived()).url.pathname).toEndWith(
    `/admin/custom_emoji_requests/${on.fake.staffServer.id("request", "party")}/approve`,
  );
  await saying("The request for :party_cat: is approved.");
  await waitFor(() => {
    if (!/^Requests\s*2/.test(waitingOnTab())) throw new Error(`test: the tab reads ${waitingOnTab()}`);
  });
  expect(() => requestRow("party_cat")).toThrow();
});

test("a request refused from the review with a reason sends the reason, and leaves the waiting ones", async () => {
  const { on, app } = await opened(() => "/admin/custom-emojis/requests", admin);
  const refused = on.fake.script("POST /admin/custom_emoji_requests/{id}/reject");

  await app.user.click(within(requestRow("sakura")).getByRole("button", { name: "Review" }));
  const review = within(await screen.findByRole("dialog"));
  await app.user.click(review.getByRole("button", { name: "Reject" }));
  const reason = await review.findByRole("textbox", { name: "Reason" });
  expect(refused.seen).toHaveLength(0);
  await app.user.type(reason, "Too close to the flag of a team.");
  await app.user.click(review.getByRole("button", { name: "Reject the request" }));

  expect((await refused.arrived()).body).toEqual({ reason: "Too close to the flag of a team." });
  await waitFor(() => {
    if (!/^Requests\s*2/.test(waitingOnTab())) throw new Error(`test: the tab reads ${waitingOnTab()}`);
  });
});

test("the request of someone who may no longer create emoji is refused by the server on Approve, and says so", async () => {
  const { on, app } = await opened(() => "/admin/custom-emojis/requests", admin);
  const approved = on.fake.script("POST /admin/custom_emoji_requests/{id}/approve");

  await app.user.click(within(requestRow("kai_wave")).getByRole("button", { name: "Review" }));
  await app.user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Approve" }));

  await approved.arrived();
  await waitFor(() => {
    const dialog = screen.getByRole("dialog");
    if (
      !words(dialog).includes(
        "The person who asked may no longer create custom emoji, so this request cannot be approved. You can reject it.",
      )
    )
      throw new Error(`test: the review says ${words(dialog)}`);
  });
  const review = within(screen.getByRole("dialog"));
  expect(review.queryAllByRole("button", { name: "Approve" })).toHaveLength(0);
  expect(review.getByRole("button", { name: "Reject" })).toBeDefined();
  expect(words(requestRow("kai_wave"))).toContain("Waiting for a decision");
});

// The reader who may ask for emoji.

test("the settings offer Custom emoji to a reader who may ask for emoji, and not to one who manages them", async () => {
  await opened(() => "/settings", artist());
  expect(main().getAllByRole("link", { name: /^Custom emoji/ })).toHaveLength(1);
});

test("the settings offer no Custom emoji to a lead, who manages emoji though they may ask for them", async () => {
  await opened(() => "/settings", { viewer: "hana", staffServer: { roles: { viewer: ["lead"] } } });
  expect(main().queryAllByRole("link", { name: /^Custom emoji/ })).toHaveLength(0);
});

test("a request sent with a shortcode and an image joins the reader's requests, waiting, and counts against the month", async () => {
  const on = fakeApp(artist());
  const requested = on.fake.script("POST /custom_emoji_requests");
  const app = await renderApp("/settings/custom-emojis/new", { on });
  await main().findByRole("textbox", { name: "Shortcode" });

  await app.user.type(main().getByRole("textbox", { name: "Shortcode" }), "mio_wink");
  await chooseFiles(app, button("Choose an image"), imageFile("wink.png"));
  await app.user.click(button("Send request"));

  expect((await requested.arrived()).body).toMatchObject({ kind: "create", shortcode: "mio_wink" });
  await app.arrived("/settings/custom-emojis");
  const requests = await main().findByRole("list", { name: "Your requests" });
  await waitFor(() => {
    const first = within(requests).getAllByRole("listitem")[0];
    if (first === undefined || !words(first).startsWith(":mio_wink: Waiting for a decision"))
      throw new Error("test: the request is not first");
  });
  expect(words(main().getByRole("list", { name: "This month" }))).toContain(
    "You asked for 4 of the 5 emoji you may ask for this month.",
  );
});

test("at the month's limit the entry says why in the place of Request an emoji", async () => {
  await opened(() => "/settings/custom-emojis", artist({ emoji: { limit: 3 } }));

  expect(main().queryAllByRole("link", { name: "Request an emoji" })).toHaveLength(0);
  expect(words(main().getByRole("region", { name: "This month" }))).toContain("3 of the 3");
});

test("a waiting request withdrawn reaches the server, and says it is withdrawn", async () => {
  const { on, app } = await opened(() => "/settings/custom-emojis", artist());
  const withdrawn = on.fake.script("POST /custom_emoji_requests/{id}/withdraw");

  await app.user.click(
    within(main().getByRole("group", { name: ":sakura:" })).getByRole("button", { name: "Withdraw" }),
  );

  expect((await withdrawn.arrived()).url.pathname).toEndWith(
    `/custom_emoji_requests/${on.fake.staffServer.id("request", "sakura")}/withdraw`,
  );
  await waitFor(() => {
    const row = within(main().getByRole("list", { name: "Your requests" }))
      .getAllByRole("listitem")
      .map(words)
      .find((item) => item.startsWith(":sakura:"));
    if (row?.startsWith(":sakura: Withdrawn") !== true) throw new Error(`test: the request reads ${row ?? "nothing"}`);
  });
});

test("the reader's own emoji is taken out of the picker on Save, and the picker is read again", async () => {
  const { on, app } = await opened(
    (o) => `/settings/custom-emojis/${o.fake.staffServer.id("emoji", "mio_heart")}`,
    artist(),
  );
  const changed = on.fake.script("PATCH /custom_emojis/mine/{id}");
  const picker = on.fake.script("GET /custom_emojis");

  await app.user.type(main().getByRole("textbox", { name: "Category" }), "Hearts");
  await app.user.selectOptions(main().getByRole("combobox", { name: "Offer it in the picker" }), "No");
  await app.user.click(button("Save"));

  expect((await changed.arrived()).body).toMatchObject({ category: "Hearts", visible_in_picker: false });
  await picker.arrived();
});

test("the reader's own emoji is deleted after the question, and leaves their emoji", async () => {
  const { on, app } = await opened(
    (o) => `/settings/custom-emojis/${o.fake.staffServer.id("emoji", "mio_heart")}`,
    artist(),
  );
  const deleted = on.fake.script("DELETE /custom_emojis/mine/{id}");

  await app.user.click(
    within(main().getByRole("group", { name: ":mio_heart:" })).getByRole("button", { name: "Delete" }),
  );
  const question = await asking("Delete :mio_heart:?");
  expect(words(screen.getByRole("alertdialog"))).toContain(
    "The emoji and its images are deleted for good. This cannot be undone.",
  );
  expect(deleted.seen).toHaveLength(0);
  await app.user.click(question.getByRole("button", { name: "Delete" }));

  await deleted.arrived();
  await app.arrived("/settings/custom-emojis");
  await waitFor(() => {
    if (main().queryAllByRole("link", { name: ":mio_heart:" }).length > 0)
      throw new Error("test: the emoji is still listed");
  });
});

/** Gives mio 20 requests more than the fake's, the newest first: a second page of their requests. */
async function manyRequests(on: ReturnType<typeof fakeApp>) {
  for (let i = 0; i < 20; i += 1)
    await on.data.api.POST("/custom_emoji_requests", {
      body: { kind: "create", shortcode: `extra_${String(i).padStart(2, "0")}`, media_id: "1" },
    });
}

test("the reader's requests past a page come with Show more", async () => {
  const on = fakeApp(artist({ emoji: { limit: "none" } }));
  await manyRequests(on);
  const app = await renderApp("/settings/custom-emojis", { on });
  const list = await main().findByRole("list", { name: "Your requests" });
  await settled(app);

  expect(within(list).getAllByRole("listitem")).toHaveLength(20);
  await app.user.click(main().getByRole("button", { name: "Show more" }));
  await waitFor(() => {
    const items = within(main().getByRole("list", { name: "Your requests" })).getAllByRole("listitem");
    if (items.length !== 25) throw new Error(`test: the requests hold ${String(items.length)}`);
  });
  expect(main().queryAllByRole("button", { name: "Show more" })).toHaveLength(0);
});

test("a request on a later page is withdrawn from the first, found by its ID", async () => {
  const on = fakeApp(artist({ emoji: { limit: "none" } }));
  await manyRequests(on);
  const withdrawn = on.fake.script("POST /custom_emoji_requests/{id}/withdraw");
  const app = await renderApp("/settings/custom-emojis", { on });
  await main().findByRole("list", { name: "Your requests" });
  await settled(app);
  await app.user.click(main().getByRole("button", { name: "Show more" }));

  const sakura = await main().findByRole("group", { name: ":sakura:" });
  await app.user.click(within(sakura).getByRole("button", { name: "Withdraw" }));

  expect((await withdrawn.arrived()).url.pathname).toEndWith(
    `/custom_emoji_requests/${on.fake.staffServer.id("request", "sakura")}/withdraw`,
  );
  await waitFor(() => {
    const row = within(main().getByRole("list", { name: "Your requests" }))
      .getAllByRole("listitem")
      .map(words)
      .find((item) => item.startsWith(":sakura:"));
    if (row?.startsWith(":sakura: Withdrawn") !== true) throw new Error(`test: the request reads ${row ?? "nothing"}`);
  });
});
