// SPDX-License-Identifier: AGPL-3.0-only

// The webhooks in the app (#3263, FR-722, ADR-0060), on the fake API, as what follows a press: one set up with the
// events the reader may choose, its secret shown once at the top with Copy and Done; its events changed; a new secret
// made and shown once; a test sent and a delivery sent again, each a delivery that waits. A community lead (hana
// holding `lead` alone, without `manage_domains`) is offered no server's event, and a webhook receiving one is
// read-only for them, with one line why, while its test, its state, its secret and Delete stay theirs. What each
// screen shows at rest is the stories' (admin-webhooks.stories.tsx).

import { expect, test } from "bun:test";
import type { FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

const admin: FakeOptions = { viewer: "hana" };
const lead: FakeOptions = { viewer: "hana", staffServer: { roles: { viewer: ["lead"] } } };

/** Waits until the app has no read on its way. */
const settled = (app: TestApp) =>
  waitFor(() => {
    if (app.data.queryClient.isFetching() > 0) throw new Error("test: the screen's reads are on their way");
  });

const button = (name: string) => main().getByRole("button", { name });

/** The app on the fake of `options` at the address `at` gives for it, once its screen is read. */
async function opened(
  at: (on: ReturnType<typeof fakeApp>) => string,
  options: FakeOptions,
  script?: (on: ReturnType<typeof fakeApp>) => void,
) {
  const on = fakeApp(options);
  script?.(on);
  const app = await renderApp(at(on), { on });
  await main().findAllByRole("heading", { level: 2 });
  await settled(app);
  return { on, app };
}

const webhook = (key: string) => (on: ReturnType<typeof fakeApp>) =>
  `/admin/webhooks/${on.fake.staffServer.id("webhook", key)}`;

/** The titles of the sections of the screen, in their order. */
const sections = () =>
  main()
    .queryAllByRole("region")
    .map((region) => words(within(region).getAllByRole("heading")[0] ?? region));

/** The row of the deliveries' table whose header starts with `event`. */
const delivery = (event: string) => {
  const row = within(main().getByRole("table", { name: "Deliveries" }))
    .getAllByRole("row")
    .find((r) =>
      within(r)
        .queryAllByRole("rowheader")
        .some((header) => words(header).startsWith(event)),
    );
  if (row === undefined) throw new Error(`test: no delivery of ${event}`);
  return row;
};

/** The events' checkboxes the form offers, by their label. */
const offeredEvents = () =>
  main()
    .queryAllByRole("checkbox")
    .map((box) => words(box.closest("label") ?? box));

test("a new webhook is set up with the events chosen, and its secret is shown once at the top with Copy and Done", async () => {
  const on = fakeApp(admin);
  const created = on.fake.script("POST /admin/webhooks");
  const app = await renderApp("/admin/webhooks/new", { on });
  await main().findByRole("textbox", { name: "Address" });

  await app.user.type(main().getByRole("textbox", { name: "Address" }), "https://hooks.example.com/in");
  await app.user.type(main().getByRole("textbox", { name: "What it is for" }), "Reports to the team");
  await app.user.click(main().getByText("A report is filed", { exact: true }));
  await app.user.click(button("Set up"));

  expect((await created.arrived()).body).toEqual({
    url: "https://hooks.example.com/in",
    description: "Reports to the team",
    events: ["report.created"],
  });
  // The fake makes the secret as the server does; the answer is a 201, which a script does not change.
  const shown = within(await main().findByRole("region", { name: "The webhook is set up" }));
  expect(words(main().getByRole("region", { name: "The webhook is set up" }))).toMatch(
    /^The webhook is set up\s*It is shown only now\. .*whsec_[A-Za-z0-9+/]{43}=\s*Copy the secret\s*Done$/,
  );
  expect(shown.getByRole("button", { name: "Copy the secret" })).toBeDefined();
  await app.user.click(shown.getByRole("button", { name: "Done" }));
  await waitFor(() => {
    if (/whsec_/.test(words(screen.getByRole("main")))) throw new Error("test: the secret is still shown");
  });
  expect(await main().findByRole("heading", { level: 2, name: "hooks.example.com" })).toBeDefined();
});

test("the administrator's form offers the servers' events, and none of the errors', which their roles leave out", async () => {
  await opened(() => "/admin/webhooks/new", admin);

  expect(sections()).toContain("Other servers");
  expect(offeredEvents()).toContain("A server is moderated");
  expect(offeredEvents()).toHaveLength(15);
});

test("the lead's form offers no event of a permission they lack: no server's and no error's", async () => {
  await opened(() => "/admin/webhooks/new", lead);

  expect(sections()).not.toContain("Other servers");
  expect(offeredEvents()).not.toContain("A server is moderated");
  expect(offeredEvents()).not.toContain("A server's moderation is lifted");
  expect(offeredEvents()).toHaveLength(13);
});

test("a webhook's events changed are saved", async () => {
  const { on, app } = await opened(webhook("alerts"), admin);
  const changed = on.fake.script("PATCH /admin/webhooks/{id}");

  await app.user.click(main().getByText("A report is reopened", { exact: true }));
  await app.user.click(button("Save"));

  const sent = await changed.arrived();
  expect((sent.body.events ?? []).map(String).toSorted()).toEqual(
    ["appeal.created", "registration.pending", "report.created", "report.reopened", "report.resolved"].toSorted(),
  );
});

test("a webhook receiving an event the lead may not handle is read-only for them, and says why in one line", async () => {
  await opened(webhook("chat"), lead);

  expect(main().queryAllByRole("textbox")).toHaveLength(0);
  expect(main().queryAllByRole("checkbox")).toHaveLength(0);
  expect(main().queryAllByRole("button", { name: "Save" })).toHaveLength(0);
  expect(words(main().getByRole("region", { name: "Where it posts" }))).toMatch(
    /Events A server is moderated and A local account posts publicly\s*It receives events that your roles do not let you change, so only someone who may handle all of them can change it\. You can still test it, turn it off and on, give it a new secret, or delete it\.$/,
  );
  for (const name of ["Send a test", "Turn off", "Make a new secret", "Delete"])
    expect(main().getAllByRole("button", { name })).toHaveLength(1);
});

test("the lead still sends a test, turns off, makes a new secret and deletes a webhook they cannot change", async () => {
  const { on, app } = await opened(webhook("chat"), lead);
  const tested = on.fake.script("POST /admin/webhooks/{id}/test");
  const disabled = on.fake.script("POST /admin/webhooks/{id}/disable");
  const rotated = on.fake.script("POST /admin/webhooks/{id}/rotate_secret");
  const deleted = on.fake.script("DELETE /admin/webhooks/{id}");

  await app.user.click(button("Send a test"));
  expect((await tested.arrived()).url.pathname).toEndWith("/test");
  await app.user.click(button("Turn off"));
  expect((await disabled.arrived()).url.pathname).toEndWith("/disable");
  await app.user.click(button("Make a new secret"));
  await app.user.click(
    within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Make a new secret" }),
  );
  await rotated.arrived();
  await app.user.click(await main().findByRole("button", { name: "Done" }));
  await app.user.click(button("Delete"));
  const question = within(await screen.findByRole("alertdialog", { name: "Delete the webhook to chat.example.org?" }));
  await app.user.type(
    question.getByRole("textbox", { name: "To confirm, enter chat.example.org" }),
    "chat.example.org",
  );
  await app.user.click(question.getByRole("button", { name: "Delete" }));
  expect((await deleted.arrived()).url.pathname).toEndWith(
    `/admin/webhooks/${on.fake.staffServer.id("webhook", "chat")}`,
  );
});

test("a new secret is shown once at the top with Copy and Done, and only after the question", async () => {
  const { on, app } = await opened(webhook("alerts"), admin, (o) => {
    o.fake.script("POST /admin/webhooks/{id}/rotate_secret", { change: () => ({ secret: "whsec_the-new-one" }) });
  });
  const rotated = on.fake.script("POST /admin/webhooks/{id}/rotate_secret");

  await app.user.click(button("Make a new secret"));
  const question = await screen.findByRole("alertdialog");
  expect(rotated.seen).toHaveLength(0);
  await app.user.click(within(question).getByRole("button", { name: "Make a new secret" }));

  await rotated.arrived();
  const top = await waitFor(() => {
    const first = main().getAllByRole("region")[0];
    if (first === undefined || !words(first).includes("whsec_the-new-one"))
      throw new Error("test: the first section does not show the secret");
    return within(first);
  });
  expect(top.getByRole("heading", { name: "The webhook has a new secret" })).toBeDefined();
  expect(top.getByRole("button", { name: "Copy the secret" })).toBeDefined();
  await app.user.click(top.getByRole("button", { name: "Done" }));
  await waitFor(() => {
    if (words(screen.getByRole("main")).includes("whsec_the-new-one"))
      throw new Error("test: the secret is still shown");
  });
});

test("a test sent adds a delivery that waits to the deliveries", async () => {
  const { on, app } = await opened(webhook("alerts"), admin);
  const tested = on.fake.script("POST /admin/webhooks/{id}/test");

  await app.user.click(button("Send a test"));

  await tested.arrived();
  await waitFor(() => {
    const rows = within(main().getByRole("table", { name: "Deliveries" })).getAllByRole("row");
    if (rows.length !== 6) throw new Error(`test: the deliveries hold ${String(rows.length - 1)} rows`);
  });
  const first = within(main().getByRole("table", { name: "Deliveries" })).getAllByRole("row")[1];
  expect(first === undefined ? "" : words(first)).toMatch(/^Test message.*Waiting/);
});

test("a failing delivery sent again reaches the server, and waits", async () => {
  const { on, app } = await opened(webhook("alerts"), admin);
  const redelivered = on.fake.script("POST /admin/webhooks/{id}/deliveries/{delivery_id}/redeliver");

  expect(words(delivery("A registration awaits approval"))).toContain("Failing");
  await app.user.click(within(delivery("A registration awaits approval")).getByRole("button", { name: "Send again" }));

  await redelivered.arrived();
  await waitFor(() => {
    if (!words(delivery("A registration awaits approval")).includes("Waiting"))
      throw new Error("test: the delivery does not wait");
  });
});

test("the list marks a failing webhook and one the server turned off", async () => {
  await opened(() => "/admin/webhooks", admin);

  const rows = within(main().getByRole("table", { name: "Webhooks" }))
    .getAllByRole("row")
    .slice(1)
    .map(words);
  expect(rows).toHaveLength(4);
  expect(rows.find((row) => row.startsWith("chat.example.org"))).toMatch(/Failing$/);
  expect(rows.find((row) => row.startsWith("archive.example.com"))).toMatch(/Turned off by the server$/);
});

test("a webhook the server turned off offers Turn on and no test, and is turned on without a question", async () => {
  const { on, app } = await opened(webhook("archive"), admin);
  const enabled = on.fake.script("POST /admin/webhooks/{id}/enable");

  expect(main().getByRole("region", { name: "Turned off by the server" })).toBeDefined();
  expect(main().queryAllByRole("button", { name: "Send a test" })).toHaveLength(0);
  await app.user.click(button("Turn on"));

  expect((await enabled.arrived()).url.pathname).toEndWith("/enable");
  expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
});

test("a webhook is deleted only with its host typed, and leaves the list", async () => {
  const { on, app } = await opened(webhook("old"), admin);
  const deleted = on.fake.script("DELETE /admin/webhooks/{id}");

  await app.user.click(button("Delete"));
  const question = within(await screen.findByRole("alertdialog", { name: "Delete the webhook to 10.0.0.8:8080?" }));
  const confirm = question.getByRole("button", { name: "Delete" });
  const host = question.getByRole("textbox", { name: "To confirm, enter 10.0.0.8:8080" });
  await app.user.type(host, "10.0.0.8");
  expect(confirm.getAttribute("aria-disabled") === "true" || confirm.hasAttribute("disabled")).toBe(true);
  expect(deleted.seen).toHaveLength(0);
  await app.user.type(host, ":8080");
  await app.user.click(question.getByRole("button", { name: "Delete" }));

  await deleted.arrived();
  await app.arrived("/admin/webhooks");
  await waitFor(() => {
    const rows = within(main().getByRole("table", { name: "Webhooks" })).getAllByRole("row");
    if (rows.length !== 4) throw new Error(`test: the list holds ${String(rows.length - 1)} webhooks`);
  });
});
