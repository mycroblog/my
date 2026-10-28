// SPDX-License-Identifier: AGPL-3.0-only
// The notification settings through the whole app (web-client/README.md T4; FR-206, FR-210, ADR-0109 §1, §2, §5): MSW
// plays the server, and a stand-in for the browser's Push API (jsdom has none) plays the Service Worker registration's
// PushManager and the Notifications API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import type { PushSubscriptionRequest } from "./api/push";
import {
  applyNotificationSettingChanges,
  instanceNotificationRows,
  moderatorNotificationRows,
  notificationSettingsFixture,
  ordinaryNotificationRows,
  pushServerKey,
  supporterNotificationRow,
} from "./test/handlers";
import { instanceFixture, preferencesFixture, type Schemas, viewerFixture } from "../../test/fixtures";
import { apiBase, signedInHandler, withViewer } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { notificationSoundKey } from "../../lib/storage";

function keyBytes(text: string): Uint8Array<ArrayBuffer> {
  return Uint8Array.from(atob(text.replaceAll("-", "+").replaceAll("_", "/")), (c) => c.charCodeAt(0));
}

/** A browser subscription to `endpoint` made with `key`. */
function browserSubscription(endpoint: string, key: string) {
  return {
    endpoint,
    options: { applicationServerKey: keyBytes(key).buffer, userVisibleOnly: true },
    toJSON: () => ({ endpoint, keys: { p256dh: "browser-p256dh", auth: "browser-auth" } }),
    unsubscribe: vi.fn(() => Promise.resolve(true)),
  };
}

/** Plays a browser with the Push API whose user answers the permission prompt with `answer`. */
function pushBrowser({
  permission = "default",
  answer = "granted",
}: { permission?: NotificationPermission; answer?: NotificationPermission } = {}) {
  let current: ReturnType<typeof browserSubscription> | null = null;
  const pushManager = {
    getSubscription: vi.fn(() => Promise.resolve(current)),
    subscribe: vi.fn((options: PushSubscriptionOptionsInit) => {
      current = browserSubscription("https://push.example.net/send/1", pushServerKey);
      expect(options.userVisibleOnly).toBe(true);
      expect(new Uint8Array(options.applicationServerKey as ArrayBuffer)).toEqual(keyBytes(pushServerKey));
      return Promise.resolve(current);
    }),
  };
  const notifications = { permission, requestPermission: vi.fn(() => Promise.resolve(answer)) };
  vi.stubGlobal("PushManager", function PushManager() {});
  vi.stubGlobal("Notification", notifications);
  Object.defineProperty(navigator, "serviceWorker", {
    configurable: true,
    value: { ready: Promise.resolve({ pushManager }) },
  });
  return {
    pushManager,
    notifications,
    /** Starts with a subscription that the browser made with `key`. */
    subscribed(key: string) {
      current = browserSubscription("https://push.example.net/send/old", key);
      return current;
    },
  };
}

/** The server: an instance that sends Web Push, and the grant's subscription with the requests it received. */
function pushServer(initial: Schemas["PushSubscription"] | null = null) {
  let saved = initial;
  const puts: PushSubscriptionRequest[] = [];
  let deletes = 0;
  server.use(
    http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, push_server_key: pushServerKey })),
    http.get(`${apiBase}/push/subscription`, ({ request }) =>
      withViewer(request, () =>
        saved === null
          ? HttpResponse.json(
              { type: "about:blank", title: "Not Found", status: 404 },
              { status: 404, headers: { "Content-Type": "application/problem+json" } },
            )
          : HttpResponse.json(saved),
      ),
    ),
    http.put(`${apiBase}/push/subscription`, async ({ request }) => {
      const body = (await request.clone().json()) as PushSubscriptionRequest;
      puts.push(body);
      saved = {
        id: "01900000-0000-7000-8000-0000000000f1",
        endpoint: body.endpoint,
        alerts: body.alerts,
        server_key: pushServerKey,
      };
      return withViewer(request, () => HttpResponse.json(saved));
    }),
    http.delete(`${apiBase}/push/subscription`, ({ request }) =>
      withViewer(request, () => {
        deletes++;
        saved = null;
        return new HttpResponse(null, { status: 204 });
      }),
    ),
  );
  return {
    puts,
    get deletes() {
      return deletes;
    },
  };
}

/** The notification settings server, which keeps the changes it receives. */
function settingsServer(initial: Schemas["NotificationSettings"] = notificationSettingsFixture) {
  let settings = initial;
  const patches: Schemas["NotificationSettingsUpdate"][] = [];
  server.use(
    http.get(`${apiBase}/preferences/notifications`, ({ request }) =>
      withViewer(request, () => HttpResponse.json(settings)),
    ),
    http.patch(`${apiBase}/preferences/notifications`, async ({ request }) => {
      const body = (await request.clone().json()) as Schemas["NotificationSettingsUpdate"];
      patches.push(body);
      settings = applyNotificationSettingChanges(settings, body.changes);
      return withViewer(request, () => HttpResponse.json(settings));
    }),
  );
  return { patches };
}

/** Every type of the default rows of a user without moderator permissions, in the order of the rows. */
const everyUserType = notificationSettingsFixture.rows.flatMap((row) => row.types);

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

afterEach(() => {
  Reflect.deleteProperty(navigator, "serviceWorker");
  delete (globalThis as { isTauri?: unknown }).isTauri;
});

function table(name: string) {
  return screen.getByRole("table", { name });
}

function columnHeaders(name: string): string[] {
  return within(table(name))
    .getAllByRole("columnheader")
    .map((header) => header.textContent);
}

function rowHeaders(name: string): string[] {
  return within(table(name))
    .getAllByRole("rowheader")
    .map((header) => header.textContent);
}

async function openSettings() {
  const view = await renderApp({ path: "/settings/notifications" });
  expect(await screen.findByRole("heading", { level: 1, name: "Notifications" })).toBeInTheDocument();
  return view;
}

describe("the table", () => {
  test("lists the rows the server offers a user without moderator permissions, in the app and by email", async () => {
    await openSettings();

    expect(screen.getAllByRole("table")).toHaveLength(2);
    expect(columnHeaders("From other people")).toEqual(["Notification", "In the app", "Email"]);
    expect(rowHeaders("From other people")).toEqual([
      "Mentions",
      "New followers",
      "Follow requests",
      "Accepted follow requests",
      "Boosts",
      "Favourites",
      "Emoji reactions",
      "Quotes",
      "Ended polls",
      "Accounts you follow moving",
      "Voice chats",
    ]);
    expect(rowHeaders("From this server")).toEqual(["Notices from the moderators", "Closure of this server"]);
    expect(screen.queryByRole("table", { name: "For moderators" })).not.toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Mentions: In the app" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Mentions: Email" })).not.toBeChecked();
  });

  test("lists the moderator rows the server offers a moderator", async () => {
    settingsServer({ rows: [...ordinaryNotificationRows, ...moderatorNotificationRows, ...instanceNotificationRows] });
    server.use(
      http.get(`${apiBase}/accounts/verify_credentials`, ({ request }) =>
        withViewer(request, () =>
          HttpResponse.json({ ...viewerFixture, permissions: ["manage_reports", "manage_accounts"] }),
        ),
      ),
    );
    await openSettings();

    expect(rowHeaders("For moderators")).toEqual([
      "Reports",
      "Sign-ups awaiting approval",
      "Appeals",
      "Limited accounts moving",
    ]);
    expect(screen.getByRole("checkbox", { name: "Reports: Email" })).toBeChecked();
  });

  test("has no email column on a server that sends no email", async () => {
    server.use(http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, sends_email: false })));
    await openSettings();

    expect(columnHeaders("From other people")).toEqual(["Notification", "In the app"]);
    expect(screen.queryByRole("checkbox", { name: "Mentions: Email" })).not.toBeInTheDocument();
  });

  test("shows the always-on cells checked and not operable", async () => {
    await openSettings();

    const moderation = screen.getByRole("checkbox", { name: "Notices from the moderators: In the app" });
    expect(moderation).toBeChecked();
    expect(moderation).toBeDisabled();
    expect(screen.getByRole("checkbox", { name: "Notices from the moderators: Email" })).toBeEnabled();
    const closure = screen.getByRole("checkbox", { name: "Closure of this server: Email" });
    expect(closure).toBeChecked();
    expect(closure).toBeDisabled();
  });

  test("shows the supporter row in the browser, and not in the Tauri app", async () => {
    settingsServer({ rows: [...notificationSettingsFixture.rows, supporterNotificationRow] });
    const { unmount } = await openSettings();
    expect(rowHeaders("From this server")).toContain("Your supporter plan");
    unmount();

    (globalThis as { isTauri?: unknown }).isTauri = true;
    await openSettings();
    expect(rowHeaders("From this server")).not.toContain("Your supporter plan");
  });

  test("turning a row off in the app saves it, and leaves its email not operable", async () => {
    const api = settingsServer();
    const { user } = await openSettings();
    const inApp = screen.getByRole("checkbox", { name: "Boosts: In the app" });
    const email = screen.getByRole("checkbox", { name: "Boosts: Email" });
    expect(email).toBeEnabled();

    await user.click(inApp);

    expect(inApp).not.toBeChecked();
    expect(email).toBeDisabled();
    await waitFor(() => {
      expect(api.patches).toEqual([{ changes: [{ group: "boost", channel: "in_app", enabled: false }] }]);
    });
    expect((await screen.findAllByText("Saved")).length).toBeGreaterThan(0);
  });

  test("a cell shows that it is saving right after the press, and then that it is saved, in its cell", async () => {
    settingsServer();
    let answer: () => void = () => undefined;
    const answered = new Promise<void>((resolve) => {
      answer = resolve;
    });
    server.use(
      http.patch(
        `${apiBase}/preferences/notifications`,
        async () => {
          await answered;
        },
        { once: true },
      ),
    );
    const { user } = await openSettings();
    const email = screen.getByRole("checkbox", { name: "Mentions: Email" });
    const cellOf = (name: string) => {
      const found = screen.getAllByRole("cell").find((c) => within(c).queryByRole("checkbox", { name }) !== null);
      if (found === undefined) throw new Error(`no cell has the checkbox ${name}`);
      return found;
    };
    const cell = cellOf("Mentions: Email");

    await user.click(email);
    expect(within(cell).getByText("Saving…")).toBeInTheDocument();
    answer();

    expect(await within(cell).findByText("Saved")).toBeInTheDocument();
    expect(within(cell).queryByText("Saving…")).not.toBeInTheDocument();
    // The other cells show nothing.
    expect(cellOf("Boosts: Email")).not.toHaveTextContent("Saved");
  });

  test("checking a row's email saves it", async () => {
    const api = settingsServer();
    const { user } = await openSettings();
    const email = screen.getByRole("checkbox", { name: "Mentions: Email" });

    await user.click(email);

    await waitFor(() => {
      expect(api.patches).toEqual([{ changes: [{ group: "mention", channel: "email", enabled: true }] }]);
    });
    expect(email).toBeChecked();
  });

  test("says why a change was not saved, and shows the saved value again", async () => {
    server.use(
      http.patch(`${apiBase}/preferences/notifications`, ({ request }) =>
        withViewer(request, () =>
          HttpResponse.json(
            { type: "about:blank", title: "Unprocessable Entity", status: 422, detail: "The row cannot be changed." },
            { status: 422, headers: { "Content-Type": "application/problem+json" } },
          ),
        ),
      ),
    );
    const { user } = await openSettings();
    const email = screen.getByRole("checkbox", { name: "Mentions: Email" });

    await user.click(email);

    expect(await screen.findByText("The row cannot be changed.")).toBeInTheDocument();
    expect(email).not.toBeChecked();
  });

  test("names no reason emails do not reach the address on a server that sends no email", async () => {
    server.use(
      http.get(`${apiBase}/preferences`, ({ request }) =>
        withViewer(request, () =>
          HttpResponse.json({
            ...preferencesFixture,
            email_suppression: { kind: "complaint", since: "2026-10-10T09:30:00Z" },
          }),
        ),
      ),
    );
    server.use(http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, sends_email: false })));
    await openSettings();

    expect(screen.queryByRole("region", { name: "Email delivery" })).not.toBeInTheDocument();
  });
});

async function pushSection() {
  return screen.findByRole("region", { name: "Push notifications" });
}

describe("push notifications", () => {
  test("are not offered when the server sends no Web Push", async () => {
    pushBrowser();
    await openSettings();

    expect(screen.queryByRole("region", { name: "Push notifications" })).not.toBeInTheDocument();
    expect(columnHeaders("From other people")).not.toContain("Push");
  });

  test("say why a browser without the Push API cannot have them", async () => {
    pushServer();
    await openSettings();

    const section = await pushSection();
    expect(within(section).getByText(/This browser cannot receive push notifications/)).toBeInTheDocument();
    expect(within(section).queryByRole("switch")).not.toBeInTheDocument();
  });

  test("say how to allow them when the browser blocks notifications for the site", async () => {
    pushBrowser({ permission: "denied" });
    pushServer();
    await openSettings();

    const section = await pushSection();
    expect(within(section).getByText(/Notifications are blocked for this site/)).toBeInTheDocument();
    expect(within(section).queryByRole("switch")).not.toBeInTheDocument();
  });

  test("turning them on asks for the permission, subscribes with the server's key and registers every row", async () => {
    const browser = pushBrowser();
    const api = pushServer();
    const { user } = await openSettings();
    const toggle = within(await pushSection()).getByRole("switch", { name: "Notify me on this device" });
    expect(toggle).not.toBeChecked();
    expect(columnHeaders("From other people")).not.toContain("Push");

    await user.click(toggle);

    await waitFor(() => {
      expect(api.puts).toHaveLength(1);
    });
    expect(browser.notifications.requestPermission).toHaveBeenCalledOnce();
    expect(browser.pushManager.subscribe).toHaveBeenCalledOnce();
    expect(api.puts[0]).toEqual({
      endpoint: "https://push.example.net/send/1",
      keys: { p256dh: "browser-p256dh", auth: "browser-auth" },
      alerts: everyUserType,
    });
    await waitFor(() => {
      expect(toggle).toBeChecked();
    });
    expect(columnHeaders("From other people")).toEqual(["Notification", "In the app", "Push", "Email"]);
    expect(screen.getByRole("checkbox", { name: "Boosts: Push" })).toBeChecked();
  });

  test("unchecking a row registers the subscription without its types", async () => {
    const browser = pushBrowser({ permission: "granted" });
    browser.subscribed(pushServerKey);
    const api = pushServer({
      id: "01900000-0000-7000-8000-0000000000f1",
      endpoint: "https://push.example.net/send/old",
      alerts: ["mention", "voice_room_started", "voice_room_invited"],
      server_key: pushServerKey,
    });
    const { user } = await openSettings();
    expect(within(await pushSection()).getByRole("switch", { name: "Notify me on this device" })).toBeChecked();
    const voice = await screen.findByRole("checkbox", { name: "Voice chats: Push" });
    expect(voice).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Favourites: Push" })).not.toBeChecked();

    await user.click(voice);

    await waitFor(() => {
      expect(api.puts).toEqual([
        {
          endpoint: "https://push.example.net/send/old",
          keys: { p256dh: "browser-p256dh", auth: "browser-auth" },
          alerts: ["mention"],
        },
      ]);
    });
    expect(browser.notifications.requestPermission).not.toHaveBeenCalled();
    expect(browser.pushManager.subscribe).not.toHaveBeenCalled();
    await waitFor(() => {
      expect(voice).not.toBeChecked();
    });
  });

  test("a row off in the app has its push cell not operable", async () => {
    const browser = pushBrowser({ permission: "granted" });
    browser.subscribed(pushServerKey);
    pushServer({
      id: "01900000-0000-7000-8000-0000000000f1",
      endpoint: "https://push.example.net/send/old",
      alerts: ["boost"],
      server_key: pushServerKey,
    });
    settingsServer(
      applyNotificationSettingChanges(notificationSettingsFixture, [
        { group: "boost", channel: "in_app", enabled: false },
      ]),
    );
    await openSettings();

    expect(await screen.findByRole("checkbox", { name: "Boosts: Push" })).toBeDisabled();
    expect(screen.getByRole("checkbox", { name: "Mentions: Push" })).toBeEnabled();
  });

  test("a subscription made with another key is replaced", async () => {
    const browser = pushBrowser({ permission: "granted" });
    const old = browser.subscribed(
      "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4",
    );
    const api = pushServer();
    const { user } = await openSettings();

    await user.click(within(await pushSection()).getByRole("switch", { name: "Notify me on this device" }));

    await waitFor(() => {
      expect(api.puts).toHaveLength(1);
    });
    expect(old.unsubscribe).toHaveBeenCalledOnce();
    expect(api.puts[0]?.endpoint).toBe("https://push.example.net/send/1");
  });

  test("turning them off deletes the server's subscription and the browser's", async () => {
    const browser = pushBrowser({ permission: "granted" });
    const current = browser.subscribed(pushServerKey);
    const api = pushServer({
      id: "01900000-0000-7000-8000-0000000000f1",
      endpoint: "https://push.example.net/send/old",
      alerts: ["mention"],
      server_key: pushServerKey,
    });
    const { user } = await openSettings();
    const toggle = within(await pushSection()).getByRole("switch", { name: "Notify me on this device" });

    await user.click(toggle);

    await waitFor(() => {
      expect(api.deletes).toBe(1);
    });
    await waitFor(() => {
      expect(toggle).not.toBeChecked();
    });
    expect(current.unsubscribe).toHaveBeenCalledOnce();
    expect(columnHeaders("From other people")).not.toContain("Push");
  });

  test("stay off when the user does not allow notifications", async () => {
    const browser = pushBrowser({ answer: "denied" });
    const api = pushServer();
    const { user } = await openSettings();
    const section = await pushSection();
    const toggle = within(section).getByRole("switch", { name: "Notify me on this device" });

    await user.click(toggle);

    expect(await within(section).findByRole("alert")).toHaveTextContent(
      "Push notifications are off because the browser did not allow notifications.",
    );
    expect(toggle).not.toBeChecked();
    expect(browser.pushManager.subscribe).not.toHaveBeenCalled();
    expect(api.puts).toEqual([]);
  });
});

describe("the notification sound", () => {
  test("is off by default, without a sound column", async () => {
    await openSettings();

    const section = screen.getByRole("region", { name: "Notification sound" });
    expect(within(section).getByRole("switch", { name: "Play a sound for new notifications" })).not.toBeChecked();
    expect(columnHeaders("From other people")).not.toContain("Sound");
  });

  test("turned on, shows the sound column, whose rows are kept on this device", async () => {
    const { user } = await openSettings();

    await user.click(screen.getByRole("switch", { name: "Play a sound for new notifications" }));
    expect(columnHeaders("From other people")).toEqual(["Notification", "In the app", "Email", "Sound"]);
    const voice = screen.getByRole("checkbox", { name: "Voice chats: Sound" });
    expect(voice).not.toBeChecked();
    await user.click(voice);

    expect(voice).toBeChecked();
    expect(notificationSoundKey.read()).toEqual({ enabled: true, types: ["voice_room_started", "voice_room_invited"] });
  });

  test("of a row off in the app is not operable", async () => {
    notificationSoundKey.write({ enabled: true, types: [] });
    settingsServer(
      applyNotificationSettingChanges(notificationSettingsFixture, [
        { group: "boost", channel: "in_app", enabled: false },
      ]),
    );
    await openSettings();

    expect(screen.getByRole("checkbox", { name: "Boosts: Sound" })).toBeDisabled();
    expect(screen.getByRole("checkbox", { name: "Mentions: Sound" })).toBeEnabled();
  });

  test("plays once from the button, and says when the browser does not play it", async () => {
    const play = vi
      .spyOn(HTMLMediaElement.prototype, "play")
      .mockImplementationOnce(() => Promise.resolve())
      .mockImplementationOnce(() => Promise.reject(new DOMException("not allowed", "NotAllowedError")));
    const { user } = await openSettings();
    const button = screen.getByRole("button", { name: "Play the sound" });

    await user.click(button);
    expect(play).toHaveBeenCalledOnce();
    expect(screen.queryByText(/The browser did not play the sound/)).not.toBeInTheDocument();

    await user.click(button);
    expect(await screen.findByText(/The browser did not play the sound/)).toBeInTheDocument();
  });
});
