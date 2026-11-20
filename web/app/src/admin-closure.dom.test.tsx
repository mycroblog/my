// SPDX-License-Identifier: AGPL-3.0-only

// The closure of the service in the app (#3266, FR-808, FR-809, ADR-0088), on the fake API, as what follows a press
// and what time does: a closure announced with its dates in the reader's zone, the export deadline refused before the
// closing date and a short notice marked; its dates changed, and it withdrawn after a question; the closing date
// passing while the screen is open; the check with what holds the closure back; the server closed once its domain is
// typed, and its progress read again while the deliveries go on; who reaches the screen, before and after the
// closure. What each screen shows at rest is the stories' (admin-closure.stories.tsx); announcing on the real server
// is admin-closure.spec.ts's, and closing it is never done there.

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { fakeHost, fakeOrigin, type FakeOptions } from "@my/e2e/fake";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { heldRepeatsOf, main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

const day = 24 * 60 * 60 * 1000;

/** Waits until the app has no read on its way. */
const settled = (app: TestApp) =>
  waitFor(() => {
    if (app.data.queryClient.isFetching() > 0) throw new Error("test: the screen's reads are on their way");
  });

/** The app as hana, the scenario's administrator, at `at` on a server of `options`, once the screen is read. */
async function atClosure(at: string, options: FakeOptions = {}, on = fakeApp({ viewer: "hana", ...options })) {
  const app = await renderApp(at, { on });
  await main().findByRole("heading", { level: 1 });
  await settled(app);
  return app;
}

const button = (name: string) => main().getByRole("button", { name });
/** A section of the screen by its heading. */
const section = (name: string) => main().getByRole("region", { name });
/** What the server and connection status says above every screen; empty when it says nothing. */
const banner = () => screen.queryAllByRole("region", { name: "Server and connection status" }).map(words).join(" ");

/** The question of `name` once it is open. */
const asking = async (name: string) => within(await screen.findByRole("alertdialog", { name }));

/** Waits until no question is open. */
const answered = () =>
  waitFor(() => {
    if (screen.queryAllByRole("alertdialog").length > 0) throw new Error("test: a question is still open");
  });

/** Sets a field of a date and a time to `value` (`2099-03-01T12:00`), as the browser's picker does. */
const setMoment = (label: string, value: string) => {
  fireEvent.change(main().getByLabelText(label), { target: { value } });
};

/** `at` as the form's field of a date and a time holds it on the device's clock. */
const fieldValue = (at: number) => {
  const d = new Date(at);
  const two = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${two(d.getMonth() + 1)}-${two(d.getDate())}T${two(d.getHours())}:${two(d.getMinutes())}`;
};

/** How the screen writes a moment in `timeZone`: "January 19, 2027 at 9:52 AM", each space as one. */
const moment = (at: string | number, timeZone: string) =>
  new Intl.DateTimeFormat("en", { dateStyle: "long", timeStyle: "short", timeZone })
    .format(new Date(at))
    .replace(/\s+/g, " ");

// Announcing.

describe("in the reader's zone", () => {
  // The device on New York's clock, the reader on Tokyo's: the screen is right only on the reader's.
  let zone: string | undefined;
  beforeAll(() => {
    zone = process.env["TZ"];
    process.env["TZ"] = "America/New_York";
  });
  afterAll(() => {
    if (zone === undefined) delete process.env["TZ"];
    else process.env["TZ"] = zone;
  });
  const tokyo = { preferences: { locale: "en", time_zone: "Asia/Tokyo", time_zone_follows_browser: false } } as const;

  test("a closure announced reaches the server at the moments typed on the reader's clock, after a question, and the screen says them on it", async () => {
    const on = fakeApp({ viewer: "hana", ...tokyo });
    const sent = on.fake.script("PUT /admin/closure/announcement");
    const app = await atClosure("/admin/closure/announcement", {}, on);

    setMoment("Closing date", "2099-03-01T12:00");
    setMoment("Export deadline", "2099-04-01T09:30");
    await app.user.click(main().getByRole("textbox", { name: "Message to the users" }));
    await app.user.paste("Thank you, and goodbye.");
    await app.user.click(button("Announce"));
    const question = await asking("Announce the closure?");
    expect(sent.seen).toHaveLength(0);
    await app.user.click(question.getByRole("button", { name: "Announce" }));

    const body = (await sent.arrived()).body;
    expect(Date.parse(body.closes_at)).toBe(Date.UTC(2099, 2, 1, 3, 0));
    expect(Date.parse(body.export_until ?? "")).toBe(Date.UTC(2099, 3, 1, 0, 30));
    expect(body.message).toBe("Thank you, and goodbye.");
    await app.arrived("/admin/closure");
    await waitFor(() => {
      expect(words(section("Announcement"))).toContain(`Closing date${moment(Date.UTC(2099, 2, 1, 3), "Asia/Tokyo")}`);
    });
    expect(words(section("Announcement"))).toContain(
      `Export deadline${moment(Date.UTC(2099, 3, 1, 0, 30), "Asia/Tokyo")}`,
    );
  });

  test("Change the dates opens on the dates in effect on the reader's clock, and saves the one changed", async () => {
    const on = fakeApp({ viewer: "hana", closure: "announced", ...tokyo });
    const announced = await on.fake.serve(new Request(`${fakeOrigin}/api/v1/admin/closure`));
    const closure = (await announced.json()) as { closes_at: string; export_until: string };
    const sent = on.fake.script("PUT /admin/closure/announcement");
    const app = await atClosure("/admin/closure", {}, on);

    await app.user.click(main().getByRole("link", { name: "Change the dates" }));
    await app.arrived("/admin/closure/announcement");
    const closes = main().getByLabelText("Closing date");
    const tokyoField = (at: string) =>
      new Intl.DateTimeFormat("sv", { dateStyle: "short", timeStyle: "short", timeZone: "Asia/Tokyo" })
        .format(new Date(at))
        .replace(" ", "T");
    await waitFor(() => {
      expect(closes).toHaveProperty("value", tokyoField(closure.closes_at));
    });
    expect(main().getByLabelText("Export deadline")).toHaveProperty("value", tokyoField(closure.export_until));

    setMoment("Closing date", "2099-05-01T18:00");
    setMoment("Export deadline", "2099-06-01T18:00");
    await app.user.click(button("Save"));
    await app.user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Save" }));

    const body = (await sent.arrived()).body;
    expect(Date.parse(body.closes_at)).toBe(Date.UTC(2099, 4, 1, 9, 0));
    expect(Date.parse(body.export_until ?? "")).toBe(Date.UTC(2099, 5, 1, 9, 0));
  });
});

test("an export deadline before the closing date is refused on the form before anything is sent, under its field", async () => {
  const on = fakeApp({ viewer: "hana" });
  const sent = on.fake.script("PUT /admin/closure/announcement");
  const app = await atClosure("/admin/closure/announcement", {}, on);

  setMoment("Closing date", "2099-03-01T12:00");
  setMoment("Export deadline", "2099-02-01T12:00");
  await app.user.click(button("Announce"));

  const deadline = main().getByLabelText("Export deadline");
  await waitFor(() => {
    expect(deadline.getAttribute("aria-invalid")).toBe("true");
  });
  const described = (deadline.getAttribute("aria-describedby") ?? "").split(" ");
  expect(described.map((id) => words(document.getElementById(id) ?? document.body))).toContain(
    "Choose a moment on or after the closing date.",
  );
  expect(app.focused()).toBe(deadline);
  expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
  expect(sent.seen).toHaveLength(0);
});

test("a closing date less than 30 days ahead is marked on the form, and one further ahead is not", async () => {
  const app = await atClosure("/admin/closure/announcement");
  const short = () => main().queryAllByText("Less than 30 days' notice.");

  setMoment("Closing date", fieldValue(Date.now() + 60 * day));
  await waitFor(() => {
    expect(main().getByLabelText("Export deadline").getAttribute("min")).toBe(fieldValue(Date.now() + 60 * day));
  });
  expect(short()).toHaveLength(0);

  setMoment("Closing date", fieldValue(Date.now() + 10 * day));
  await waitFor(() => {
    expect(short()).toHaveLength(1);
  });
  expect(app.path()).toBe("/admin/closure/announcement");
});

test("a closure announced at short notice is marked on the closure's screen too", async () => {
  await atClosure("/admin/closure", { closure: "announced", closureAdmin: { shortNotice: true } });

  expect(words(section("Announcement"))).toMatch(/30 days/);
});

test("Withdraw asks first: Cancel keeps the closure, and Withdraw ends it, the banner with it", async () => {
  const on = fakeApp({ viewer: "hana", closure: "announced" });
  const withdrawn = on.fake.script("DELETE /admin/closure/announcement");
  const app = await atClosure("/admin/closure", {}, on);
  expect(banner()).toMatch(/^This server closes on /);

  await app.user.click(button("Withdraw"));
  await app.user.click((await asking("Withdraw the closure?")).getByRole("button", { name: "Cancel" }));
  await answered();
  expect(withdrawn.seen).toHaveLength(0);

  await app.user.click(button("Withdraw"));
  await app.user.click((await asking("Withdraw the closure?")).getByRole("button", { name: "Withdraw" }));
  await withdrawn.arrived();
  await main().findByText("No closure is announced.");
  await waitFor(() => {
    expect(banner()).toBe("");
  });
});

// The closing date.

test("once the closing date passed, the screen says so, offers Check and close first, and the banner speaks in the past", async () => {
  await atClosure("/admin/closure", { closure: "announced", closureAdmin: { due: true } });

  expect(main().getAllByRole("heading", { level: 3 }).map(words)).toEqual(["Close the server", "Announcement"]);
  expect(within(section("Close the server")).getByRole("link", { name: "Check and close" }).getAttribute("href")).toBe(
    "/admin/closure/close",
  );
  expect(words(section("Announcement"))).toContain("StateClosing date passed");
  expect(banner()).toMatch(/^This server's closing date, .+, has passed, and it closes soon\./);
});

test("the closing date passing while the screen is open turns it to Check and close without a reload", async () => {
  const soon = new Date(Date.now() + 1200).toISOString();
  const on = fakeApp({ viewer: "hana", closure: "announced" });
  on.fake.script("GET /admin/closure", { change: (closure) => ({ ...closure, closes_at: soon }) });
  on.fake.script("GET /instance", {
    change: (instance) =>
      instance.closure === undefined ? instance : { ...instance, closure: { ...instance.closure, closes_at: soon } },
  });
  await atClosure("/admin/closure", {}, on);
  expect(words(section("Announcement"))).toContain("StateAnnounced");
  expect(main().queryAllByRole("link", { name: "Check and close" })).toHaveLength(0);

  await waitFor(
    () => {
      expect(main().getAllByRole("link", { name: "Check and close" })).toHaveLength(1);
    },
    { timeout: 3000 },
  );
  expect(words(section("Announcement"))).toContain("StateClosing date passed");
  await waitFor(() => {
    expect(banner()).toMatch(/^This server's closing date, .+, has passed/);
  });
});

// The check, and closing.

test("the check says what holds the closure back with its count and what to do, and before the closing date offers no Close", async () => {
  await atClosure("/admin/closure/close", { closure: "announced", closureAdmin: { due: true, warnings: true } });
  const does = words(main().getByRole("region", { name: "What closing does" }));
  expect(does).toContain("Account moves not ended2Wait for the moves to end: nobody can start one after the closure.");
  expect(does).toContain(
    "Accounts under a retention hold1Export the records under a hold with the command line, and keep them outside the server.",
  );
  expect(main().getAllByRole("button", { name: "Close the server" })).toHaveLength(1);
});

test("before its closing date the check offers no Close, and says when it can", async () => {
  await atClosure("/admin/closure/close", { closure: "announced" });

  expect(main().queryAllByRole("button", { name: "Close the server" })).toHaveLength(0);
  expect(words(section("Closing"))).toMatch(/^ClosingYou can close the server once .+ has passed\.$/);
});

test("Close the server is confirmed by typing the server's domain, sends what was typed, and shows the progress", async () => {
  const on = fakeApp({ viewer: "hana", closure: "announced", closureAdmin: { due: true } });
  const closing = on.fake.script("POST /admin/closure/close");
  const app = await atClosure("/admin/closure/close", {}, on);

  await app.user.click(button("Close the server"));
  const question = await asking(`Close ${fakeHost}?`);
  const field = question.getByRole("textbox", { name: `To confirm, enter ${fakeHost}` });
  const confirm = question.getByRole("button", { name: "Close the server" });
  expect(confirm).toHaveProperty("disabled", true);
  await app.user.type(field, fakeHost.slice(0, -1));
  expect(confirm).toHaveProperty("disabled", true);
  expect(closing.seen).toHaveLength(0);

  await app.user.type(field, fakeHost.slice(-1));
  expect(confirm).toHaveProperty("disabled", false);
  await app.user.click(confirm);

  expect((await closing.arrived()).body).toEqual({ confirm_domain: fakeHost });
  const progress = await main().findByRole("region", { name: "Deleting the accounts, here and on other servers" });
  expect(within(progress).getByRole("meter", { name: "Deliveries done" })).toBeDefined();
});

test("while closed the progress is read again, the failures apart from the meter, until every delivery has ended", async () => {
  const repeats = heldRepeatsOf(10_000);
  const on = fakeApp({ viewer: "hana", closure: "closed" });
  const app = await atClosure("/admin/closure", {}, on);
  const meter = () => main().getByRole("meter", { name: "Deliveries done" });

  const going = on.fake.script("GET /admin/closure", {
    change: (closure) => ({
      ...closure,
      progress: {
        accounts: 1284,
        fanouts_pending: 0,
        deliveries: { pending: 600, succeeded: 400, permanent_failure: 0, skipped: 0, dead: 0 },
      },
    }),
  });
  expect(repeats.tick()).toBeGreaterThan(0);
  await going.arrived();
  await waitFor(() => {
    expect(meter().getAttribute("aria-valuenow")).toBe("40");
  });
  going.end();

  const ended = on.fake.script("GET /admin/closure", {
    change: (closure) => ({
      ...closure,
      state: "delivery_completed" as const,
      progress: {
        accounts: 1284,
        fanouts_pending: 0,
        deliveries: { pending: 0, succeeded: 900, permanent_failure: 70, skipped: 20, dead: 10 },
      },
    }),
  });
  expect(repeats.tick()).toBeGreaterThan(0);
  await ended.arrived();
  await main().findByText("Every delivery has ended.");
  expect(words(main().getByLabelText("Not delivered"))).toBe("Refused70Skipped20Out of attempts10");
  await settled(app);

  expect(repeats.tick()).toBe(0);
  expect(ended.seen).toHaveLength(1);
});

// Who reaches it.

test("a reader without manage_service_closure is refused the closure's screen, and the administration lists it for no one else", async () => {
  const on = fakeApp({ viewer: "hana" });
  on.fake.script("GET /accounts/verify_credentials", {
    change: (account) => ({
      ...account,
      permissions: account.permissions.filter((p) => p !== "manage_service_closure"),
    }),
  });
  const app = await renderApp("/admin/closure", { on });
  await main().findByRole("alert");
  expect(main().queryAllByRole("heading", { name: "Server closure" })).toHaveLength(0);

  await app.navigate("/admin");
  await main().findByRole("navigation", { name: "Administration screens" });
  expect(main().queryAllByRole("link", { name: /^Server closure/ })).toHaveLength(0);
  expect(main().getAllByRole("link", { name: /^Theme/ })).toHaveLength(1);
});

describe("once the server is closed", () => {
  test("a holder of manage_service_closure still reaches the closure's screen, and every other screen says the server closed", async () => {
    const on = fakeApp({ viewer: "hana", closure: "closed" });
    const app = await atClosure("/admin/closure", {}, on);
    expect(main().getByRole("region", { name: "Deleting the accounts, here and on other servers" })).toBeDefined();

    await app.navigate("/lists");
    await main().findByRole("heading", { name: /has closed$/ });
  });

  test("a reader without the permission gets the closed server's screen at the closure's address", async () => {
    const on = fakeApp({ viewer: "hana", closure: "closed" });
    on.fake.script("GET /accounts/verify_credentials", {
      change: (account) => ({
        ...account,
        permissions: account.permissions.filter((p) => p !== "manage_service_closure"),
      }),
    });
    await renderApp("/admin/closure", { on });
    await main().findByRole("heading", { name: /has closed$/ });
    expect(main().queryAllByRole("heading", { name: "Server closure" })).toHaveLength(0);
  });

  test("a reader of no administration gets the closed server's screen at the closure's address", async () => {
    await renderApp("/admin/closure", { viewer: "yuki", closure: "closed" });
    await main().findByRole("heading", { name: /has closed$/ });
    expect(main().queryAllByRole("heading", { name: "Server closure" })).toHaveLength(0);
  });
});
