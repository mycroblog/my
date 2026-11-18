// SPDX-License-Identifier: AGPL-3.0-only

// Notifications, the entry of the settings (#3201), as the app places it: at /settings/notifications, under the menu's
// General group; what it says after the reader left it (a toast names the kind and the channel, or the condition,
// whose save failed); the ways to it from the notifications (the line that Web Push is on, and the held-back
// notifications); the sound a notification plays for a kind whose sound is on; and the rules of a settings screen,
// which it keeps with every table and section it can have. What each control sends and says is the feature's test
// (app-features, settings/notifications-settings*.dom.test.tsx). The browser's push is the stories' stand-in
// (push.samples.ts).

import { describe, expect, onTestFinished, test } from "bun:test";
import { setNotificationSound, setNotificationSoundKinds } from "@my/app-data";
import { memberNotificationRows, moderatorNotificationRows } from "@my/e2e/fake";
import { settingsFindings } from "@my/design-system/test/settings";
import { screen, waitFor, within } from "@testing-library/react";
import { pushBrowser } from "./push.samples";
import { main, renderApp } from "./test/app";
import { browserWithPush, quietDevice } from "./test/push";
import { fakeApp } from "./test/server";

const at = "/settings/notifications";

/** A server that sends Web Push and email, to a member. */
const server = { push: true, email: true, settings: { notificationRows: memberNotificationRows } } as const;

/** A browser without Web Push, as a simulation is, on a device that keeps nothing from the test before. */
const browser = quietDevice;

/** The title of the screen in place, as its heading says it. */
const heading = () =>
  main()
    .queryAllByRole("heading", { level: 1 })
    .map((title) => title.textContent)
    .join();

/** The tables of the entry, once they are drawn. */
const tables = () => main().findAllByRole("table");

/** The settings' menus outside the screen: the one the frame places beside an entry. */
const beside = () =>
  screen.queryAllByRole("navigation", { name: "Settings" }).filter((menu) => !screen.getByRole("main").contains(menu));

describe("the entry", () => {
  test("is a row of the menu's General group, which leads to its screen", async () => {
    browser();
    const app = await renderApp("/settings", server);
    const menu = main().getByRole("navigation", { name: "Settings" });
    const general = within(menu).getByRole("region", { name: "General" });

    await app.user.click(within(general).getByRole("link", { name: /^Notifications/ }));

    await tables();
    expect(app.path()).toBe(at);
    expect(heading()).toBe("Notifications");
    expect(app.title()).toContain("Notifications");
  });

  test("opens at its address, with the menu beside it marking it", async () => {
    browser();
    const app = await renderApp(at, server);

    expect((await tables()).map((table) => table.getAttribute("aria-label"))).toEqual([
      "From other people",
      "From this server",
    ]);
    expect(heading()).toBe("Notifications");
    expect(app.title()).toContain("Notifications");
    const [menu, ...others] = beside();
    if (menu === undefined) throw new Error("test: no settings menu beside the screen");
    expect(others).toHaveLength(0);
    expect(
      within(menu)
        .getAllByRole("link")
        .filter((link) => link.getAttribute("aria-current") === "page")
        .map((link) => link.getAttribute("href")),
    ).toEqual([at]);
  });

  test("its way back leads to the menu", async () => {
    browser();
    const app = await renderApp(at, server);
    await tables();

    await app.user.click(main().getByRole("button", { name: "Settings" }));

    await main().findByRole("navigation", { name: "Settings" });
    expect(app.path()).toBe("/settings");
  });
});

// What the rules are and how a tree breaks each is the check's own test (design-system, test/settings.dom.test.tsx).
test("the entry with every section and column it can have keeps the rules of a settings screen (ADR-0110 §6)", async () => {
  browserWithPush();
  setNotificationSound(true);
  setNotificationSoundKinds(["mention", "follow"], true);
  await renderApp(at, {
    ...server,
    viewer: "hana",
    pushState: "subscribed",
    settings: { notificationRows: [...memberNotificationRows, ...moderatorNotificationRows] },
    preferences: { email_suppression: { kind: "hard_bounce", state_changed_at: "2026-11-02T09:30:00Z" } },
  });
  await tables();

  // The screen has what the check counts: the summary, this device, three tables and the policy, each a section.
  await waitFor(() => {
    expect(
      main()
        .getAllByRole("table")
        .map((table) => within(table).getAllByRole("columnheader").length),
    ).toEqual([5, 5, 5]);
  });
  expect(main().getAllByRole("region")).toHaveLength(6);
  expect(beside()).toHaveLength(1);
  expect(settingsFindings(document.body)).toEqual([]);
});

test("the entry whose settings could not be read says so under its title, keeps the rules, and Try again reads them", async () => {
  browser();
  const on = fakeApp(server);
  // Every read fails, the route's own before the screen and the screen's after it, until the server is back.
  const failing = on.fake.script("GET /preferences/notifications", { fail: 500 });
  const app = await renderApp(at, { on });

  const failed = await main().findByRole("alert");
  expect(failed.textContent).toContain("Your settings could not be loaded.");
  expect(heading()).toBe("Notifications");
  expect(beside()).toHaveLength(1);
  expect(settingsFindings(document.body)).toEqual([]);
  failing.end();

  await app.user.click(within(failed).getByRole("button", { name: "Try again" }));

  await tables();
  expect(main().queryAllByRole("alert")).toHaveLength(0);
});

// The row that would say so is gone with its screen: the app says it over whatever screen the reader is on.
describe("a change whose save fails after the reader left the screen", () => {
  /** The toasts that say `what` was not saved. */
  const toasts = (what: string) =>
    screen
      .queryAllByRole("status")
      .filter((toast) =>
        toast.textContent.startsWith(`“${what}” could not be saved. Open the settings and change it again.`),
      );

  const changes = [
    [
      "a cell",
      "PATCH /preferences/notifications",
      "Mentions: Email",
      async (user: Awaited<ReturnType<typeof renderApp>>["user"]) => {
        const table = within(main().getByRole("table", { name: "From other people" }));
        await user.click(table.getByRole("checkbox", { name: /^Mentions\s*Email$/ }));
      },
    ],
    [
      "a choice of the policy",
      "PATCH /preferences/notification_policy",
      "People you do not follow",
      async (user: Awaited<ReturnType<typeof renderApp>>["user"]) => {
        await user.selectOptions(main().getByRole("combobox", { name: "People you do not follow" }), "Drop");
      },
    ],
  ] as const;

  for (const [what, route, name, change] of changes)
    test(`${what}: a toast names it, and Open settings leads back to the entry, which shows what the server holds`, async () => {
      browser();
      const on = fakeApp(server);
      const app = await renderApp(at, { on });
      await tables();
      const sent = on.fake.script(route, { hold: "request", fail: 500 });

      await change(app.user);
      await sent.arrived();
      await app.navigate("/lists");
      expect(toasts(name)).toHaveLength(0);
      sent.release();

      const [toast, ...others] = await waitFor(() => {
        if (toasts(name).length === 0) throw new Error(`test: no toast says that ${name} was not saved`);
        return toasts(name);
      });
      if (toast === undefined) throw new Error("test: no toast");
      expect(others).toHaveLength(0);
      expect(app.path()).toBe("/lists");

      await app.user.click(within(toast).getByRole("button", { name: "Open settings" }));

      await tables();
      expect(app.path()).toBe(at);
      expect(toasts(name)).toHaveLength(0);
      const email = within(main().getByRole("table", { name: "From other people" })).getByRole<HTMLInputElement>(
        "checkbox",
        { name: /^Mentions\s*Email$/ },
      );
      expect(email.checked).toBe(false);
      expect(main().getByRole<HTMLSelectElement>("combobox", { name: "People you do not follow" }).value).toBe(
        "accept",
      );
    });

  test("while the reader is still on the screen, the row says so and no toast does", async () => {
    browser();
    const on = fakeApp(server);
    const app = await renderApp(at, { on });
    await tables();
    on.fake.script("PATCH /preferences/notifications", { fail: 500 });

    await app.user.click(
      within(main().getByRole("table", { name: "From other people" })).getByRole("checkbox", {
        name: /^Mentions\s*Email$/,
      }),
    );

    await main().findByRole("button", { name: "Try again" });
    expect(toasts("Mentions: Email")).toHaveLength(0);
  });
});

describe("the ways to the entry from the notifications", () => {
  test("the line that Web Push is on, in the prompt's place, leads to it, where the switch is on", async () => {
    browserWithPush();
    const app = await renderApp("/notifications", server);
    const prompt = await main().findByRole("region", { name: /^Get your notifications on this device/ });

    await app.user.click(within(prompt).getByRole("button", { name: "Turn on" }));
    await main().findByText("Notifications are on for this device.");
    await app.user.click(main().getByRole("link", { name: "Choose which kinds in the settings" }));

    await tables();
    expect(app.path()).toBe(at);
    await waitFor(() => {
      expect(main().getByRole<HTMLInputElement>("switch", { name: "Push notifications" }).checked).toBe(true);
    });
  });

  test("the held-back notifications lead to it, where the policy that holds them back is", async () => {
    browser();
    const app = await renderApp("/notifications/requests", { ...server, held: true });
    await main().findByRole("link", { name: "Bakery Kobo" });

    await app.user.click(main().getByRole("link", { name: "Notification settings" }));

    await tables();
    expect(app.path()).toBe(at);
    expect(main().getAllByRole("region", { name: "Notifications from people you may not know" })).toHaveLength(1);
  });

  test("the entry's own link leads to the held-back notifications", async () => {
    browser();
    const app = await renderApp(at, { ...server, held: true });
    await tables();

    await app.user.click(main().getByRole("link", { name: "Held-back notifications" }));

    await main().findByRole("link", { name: "Bakery Kobo" });
    expect(app.path()).toBe("/notifications/requests");
  });
});

// The feature's test holds the other reasons (settings/notifications-settings-device.dom.test.tsx): what makes a
// browser a tab of Safari on an iPhone is the platform's to know, and the stories' stand-in gives it.
test("in a tab of Safari on an iPhone, the Web Push switch is off and disabled, and says to add the site to the Home Screen first", async () => {
  quietDevice();
  onTestFinished(pushBrowser({ push: "install" })());
  await renderApp(at, server);
  await tables();

  const toggle = main().getByRole<HTMLInputElement>("switch", { name: "Push notifications" });
  expect(toggle.checked).toBe(false);
  expect(toggle.disabled).toBe(true);
  expect(
    (toggle.getAttribute("aria-describedby") ?? "").split(" ").map((id) => document.getElementById(id)?.textContent),
  ).toEqual([
    "On an iPhone or iPad, add this site to the Home Screen and open it from there to get push notifications.",
  ]);
});

// Which kinds sound, and that several at once sound once, is the data's test (app-data, push.test.tsx).
test("a notification that arrives while the app is open plays the sound, for a kind whose sound the reader turned on", async () => {
  const heard = browserWithPush();
  setNotificationSound(true);
  setNotificationSoundKinds(["favourite"], true);
  const app = await renderApp("/lists", server);
  expect(heard.sounds).toBe(0);

  // A favourite of the reader's post, on the stream.
  app.fake.newNotification();

  await waitFor(() => {
    expect(heard.sounds).toBe(1);
  });
});
