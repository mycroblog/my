// SPDX-License-Identifier: AGPL-3.0-only

// The announcements as the administrator writes them (#3263, FR-726, ADR-0125), on the fake API, as what follows a
// press: one written with a title and a Markdown text is published after a question, and a reader then sees it among
// the server's announcements and counted above the home timeline; one is saved as a draft, which offers no Withdraw;
// a draft's page shows it as its readers' page will; a draft is published from its page, or deleted after a question;
// one showing is revised, and withdrawn after a question, and offers no Delete; its window is read in the reader's
// time zone. What each screen shows at rest is the stories' (admin-announcements.stories.tsx).

import { afterAll, beforeAll, expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { paste } from "./test/composer";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

/** Waits until the app has no read on its way. */
const settled = (app: TestApp) =>
  waitFor(() => {
    if (app.data.queryClient.isFetching() > 0) throw new Error("test: the screen's reads are on their way");
  });

const button = (name: string) => main().getByRole("button", { name });

/** The question of `name` once it is open. */
const asking = async (name: string | RegExp) => within(await screen.findByRole("alertdialog", { name }));

/** Writes the title and the text of the form on the screen. */
async function write(app: TestApp, title: string, text: string) {
  await app.user.click(main().getByRole("textbox", { name: "Title" }));
  await app.user.paste(title);
  await paste(app, main().getByRole("textbox", { name: "Text" }), text);
}

/** The app as hana, the administrator, at the address `at` gives for the fake, once its form is read. */
async function atAnnouncement(
  at: (on: ReturnType<typeof fakeApp>) => string,
  more: Parameters<typeof fakeApp>[0] = {},
) {
  const on = fakeApp({ viewer: "hana", ...more });
  const app = await renderApp(at(on), { on });
  await main().findAllByRole("heading", { level: 2 });
  await settled(app);
  return { on, app };
}

// The device's zone is UTC, the reader's another: a window read on the device's clock is nine hours off.
let zone: string | undefined;
beforeAll(() => {
  zone = process.env.TZ;
  process.env.TZ = "UTC";
});
afterAll(() => {
  if (zone === undefined) delete process.env.TZ;
  else process.env.TZ = zone;
});

test("an announcement published from the screen after the question reaches every reader: listed and counted", async () => {
  const title = "The garden opens on Sunday";
  // The line stands above the timeline whatever it holds: a server with the scenario's people and announcements and
  // no post draws home at once.
  const on = fakeApp({
    viewer: "hana",
    seed: { people: scenario.people, announcements: scenario.announcements },
  });
  const app = await renderApp("/", { on });
  const before = await main().findByRole("link", { name: /^\d+ unread announcements?$/ });
  const count = Number(/^\d+/.exec(before.getAttribute("aria-label") ?? words(before))?.[0] ?? "-1");
  const created = on.fake.script("POST /admin/announcements");

  await app.navigate("/admin/announcements/new");
  await main().findByRole("textbox", { name: "Title" });
  await write(app, title, "Bring **seeds** to share.");
  await app.user.click(button("Publish"));
  const question = await asking(/^Publish/);
  expect(created.seen).toHaveLength(0);
  await app.user.click(question.getByRole("button", { name: "Publish" }));

  const sent = await created.arrived();
  expect(sent.body).toMatchObject({ title, text: "Bring **seeds** to share.", publish: true });
  expect(await main().findByRole("heading", { level: 2, name: title })).toBeDefined();

  await app.navigate("/announcements");
  const entry = await main().findByRole("article", { name: title });
  await app.user.click(within(entry).getByRole("link", { name: title }));
  expect(await main().findByRole("heading", { level: 1, name: title })).toBeDefined();
  // The Markdown is the server's to render (the fake shows it as written): the reader reads its words.
  expect(words(screen.getByRole("main"))).toContain("seeds** to share.");
  await app.navigate("/");
  expect(await main().findByRole("link", { name: `${String(count + 1)} unread announcements` })).toBeDefined();
});

test("an announcement saved as a draft is not published, and its page offers Publish and no Withdraw", async () => {
  const on = fakeApp({ viewer: "hana" });
  const created = on.fake.script("POST /admin/announcements");
  const app = await renderApp("/admin/announcements/new", { on });
  await main().findByRole("textbox", { name: "Title" });

  await write(app, "Notes for the moderators", "Read the new guide.");
  await app.user.click(button("Save as draft"));

  expect((await created.arrived()).body).toMatchObject({ title: "Notes for the moderators", publish: false });
  expect(await main().findByRole("heading", { level: 2, name: "Notes for the moderators" })).toBeDefined();
  await settled(app);
  expect(words(main().getByRole("region", { name: "Where it stands" }))).toContain("Draft");
  expect(button("Publish")).toBeDefined();
  expect(main().queryAllByRole("button", { name: /^Withdraw/ })).toHaveLength(0);
  expect(on.fake.staffServer.shownAnnouncements().map((a) => a.title)).not.toContain("Notes for the moderators");
});

test("a draft's page shows it as its readers' page will, as if published now", async () => {
  const { app } = await atAnnouncement((o) => `/admin/announcements/${o.fake.staffServer.id("announcement", "draft")}`);
  await settled(app);

  const seen = main().getByRole("region", { name: "What readers see" });
  expect(words(seen)).toContain("As last saved, if it were published now.");
  const entry = within(seen).getByRole("article");
  expect(words(entry)).toContain("Moderation report for the autumn");
  expect(words(entry)).toContain("42 reports, 3 suspended accounts.");
  expect(words(entry)).toMatch(/Showing since \w+ \d+, \d{4}/);
});

test("a draft published from its page after the question shows to the readers", async () => {
  const { on, app } = await atAnnouncement(
    (o) => `/admin/announcements/${o.fake.staffServer.id("announcement", "draft")}`,
  );
  const published = on.fake.script("POST /admin/announcements/{id}/publish");

  expect(main().queryAllByRole("button", { name: /^Withdraw/ })).toHaveLength(0);
  await app.user.click(button("Publish"));
  await app.user.click((await asking(/^Publish/)).getByRole("button", { name: "Publish" }));

  expect((await published.arrived()).url.pathname).toEndWith("/publish");
  await waitFor(() => {
    if (!on.fake.staffServer.shownAnnouncements().some((a) => a.title === "Moderation report for the autumn"))
      throw new Error("test: the readers do not see it");
  });
  await app.navigate("/announcements");
  expect(await main().findByRole("article", { name: "Moderation report for the autumn" })).toBeDefined();
});

test("a draft is deleted only once the question is answered, and the list then lacks it", async () => {
  const { on, app } = await atAnnouncement(
    (o) => `/admin/announcements/${o.fake.staffServer.id("announcement", "draft")}`,
  );
  const deleted = on.fake.script("DELETE /admin/announcements/{id}");

  expect(main().getAllByRole("button").at(-1)).toBe(button("Delete"));
  await app.user.click(button("Delete"));
  expect(words(await screen.findByRole("alertdialog", { name: "Delete this draft?" }))).toContain(
    "Its title and text are gone. This cannot be undone.",
  );
  await app.user.click((await asking("Delete this draft?")).getByRole("button", { name: "Keep it" }));
  await waitFor(() => {
    if (screen.queryAllByRole("alertdialog").length > 0) throw new Error("test: the question is still open");
  });
  expect(deleted.seen).toHaveLength(0);

  await app.user.click(button("Delete"));
  await app.user.click((await asking("Delete this draft?")).getByRole("button", { name: "Delete" }));

  expect((await deleted.arrived()).url.pathname).toEndWith(
    `/admin/announcements/${on.fake.staffServer.id("announcement", "draft")}`,
  );
  await app.arrived("/admin/announcements");
  await main().findByText("Welcome to the new web client");
  await settled(app);
  expect(main().queryAllByText("Moderation report for the autumn")).toHaveLength(0);
});

test("an announcement showing now is revised on Save, and what readers see says the new words", async () => {
  const { on, app } = await atAnnouncement(
    (o) => `/admin/announcements/${o.fake.staffServer.id("announcement", "showing")}`,
  );
  const revised = on.fake.script("PUT /admin/announcements/{id}");

  const title = main().getByRole("textbox", { name: "Title" });
  await app.user.clear(title);
  await app.user.paste("Welcome to the rebuilt web client");
  await app.user.click(button("Save"));

  expect((await revised.arrived()).body.title).toBe("Welcome to the rebuilt web client");
  await waitFor(() => {
    if (!words(main().getByRole("region", { name: "What readers see" })).includes("Welcome to the rebuilt web client"))
      throw new Error("test: what readers see still says the old title");
  });
});

test("an announcement is withdrawn only once the question is answered, and its page then offers nothing to change", async () => {
  const { on, app } = await atAnnouncement(
    (o) => `/admin/announcements/${o.fake.staffServer.id("announcement", "showing")}`,
  );
  const withdrawn = on.fake.script("POST /admin/announcements/{id}/withdraw");

  expect(main().queryAllByRole("button", { name: "Delete" })).toHaveLength(0);
  expect(main().getAllByRole("button").at(-1)).toBe(button("Withdraw"));
  await app.user.click(button("Withdraw"));
  const first = await asking("Withdraw this announcement?");
  expect(words(screen.getByRole("alertdialog"))).toContain("Nobody sees it again. This cannot be undone.");
  await app.user.click(first.getByRole("button", { name: "Keep it" }));
  await waitFor(() => {
    if (screen.queryAllByRole("alertdialog").length > 0) throw new Error("test: the question is still open");
  });
  expect(withdrawn.seen).toHaveLength(0);

  await app.user.click(button("Withdraw"));
  await app.user.click((await asking("Withdraw this announcement?")).getByRole("button", { name: "Withdraw" }));

  expect((await withdrawn.arrived()).url.pathname).toEndWith("/withdraw");
  await waitFor(() => {
    if (main().queryAllByRole("textbox").length > 0) throw new Error("test: the form is still there");
  });
  expect(words(main().getByRole("region", { name: "Where it stands" }))).toContain("Withdrawn");
  expect(main().queryAllByRole("button", { name: /^(Save|Publish|Withdraw)$/ })).toHaveLength(0);
});

test("the window is written in the reader's time zone, not the device's", async () => {
  const on = fakeApp({
    viewer: "hana",
    preferences: { time_zone: "Asia/Tokyo", time_zone_follows_browser: false, locale: "en" },
  });
  const created = on.fake.script("POST /admin/announcements");
  const app = await renderApp("/admin/announcements/new", { on });
  await main().findByRole("textbox", { name: "Title" });
  await settled(app);

  await write(app, "Planned maintenance", "Down for an hour.");
  const starts = main().getByLabelText("Starts");
  const ends = main().getByLabelText("Ends");
  await app.user.clear(starts);
  await app.user.type(starts, "2099-02-03T09:00");
  await app.user.clear(ends);
  await app.user.type(ends, "2099-02-03T10:00");
  await app.user.click(button("Save as draft"));

  const sent = (await created.arrived()).body;
  expect(Date.parse(sent.starts_at ?? "")).toBe(Date.parse("2099-02-03T00:00:00Z"));
  expect(Date.parse(sent.ends_at ?? "")).toBe(Date.parse("2099-02-03T01:00:00Z"));
});
