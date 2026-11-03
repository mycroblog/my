// SPDX-License-Identifier: AGPL-3.0-only
// The prompt to turn on Web Push on the notifications page through the whole app (web-client/README.md T4; ADR-0137):
// MSW plays the server, and a stand-in for the browser's Push API (jsdom has none) plays the browser.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { clearUserStorage, pushPromptDismissedKey } from "../../lib/storage";
import { viewerFixture } from "../../test/fixtures";
import { apiBase, problem, signedInHandler, withViewer } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { notificationSettingsFixture, pushServerKey } from "./test/handlers";
import { pushBrowser, pushServer } from "./test/push";

const everyUserType = notificationSettingsFixture.rows.flatMap((row) => row.types);

const subscription = {
  id: "01900000-0000-7000-8000-0000000000f1",
  endpoint: "https://push.example.net/send/1",
  alerts: ["mention"],
  server_key: pushServerKey,
} as const;

beforeEach(() => {
  server.use(
    signedInHandler,
    http.get(`${apiBase}/notifications`, ({ request }) => withViewer(request, () => HttpResponse.json({ items: [] }))),
    http.get(`${apiBase}/markers`, ({ request }) => withViewer(request, () => HttpResponse.json({}))),
  );
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

afterEach(() => {
  Reflect.deleteProperty(navigator, "serviceWorker");
});

function promptRegion() {
  return screen.queryByRole("region", { name: "Get notifications on this device" });
}

async function openNotifications() {
  const view = await renderApp({ path: "/notifications" });
  await screen.findByRole("tablist", { name: "Notification types" });
  return view;
}

async function openToPrompt() {
  const view = await openNotifications();
  return { ...view, prompt: await screen.findByRole("region", { name: "Get notifications on this device" }) };
}

/** Lets what the page reads settle, for what must not show. */
async function settled(done: () => boolean = () => true) {
  await waitFor(() => {
    expect(done()).toBe(true);
  });
  await new Promise((resolve) => setTimeout(resolve, 50));
}

function selectedTab() {
  return within(screen.getByRole("tablist", { name: "Notification types" })).getByRole("tab", { selected: true });
}

describe("when the prompt shows (ADR-0137 §1)", () => {
  test("offers Web Push above the tabs without asking the browser or subscribing", async () => {
    const browser = pushBrowser();
    const push = pushServer();

    const { prompt } = await openToPrompt();

    const tabs = screen.getByRole("tablist", { name: "Notification types" });
    expect(prompt.compareDocumentPosition(tabs) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(within(prompt).getByRole("button", { name: "Turn on" })).toBeEnabled();
    expect(within(prompt).getByRole("button", { name: "Close" })).toBeEnabled();
    expect(browser.notifications.requestPermission).not.toHaveBeenCalled();
    expect(browser.pushManager.subscribe).not.toHaveBeenCalled();
    expect(push.puts).toEqual([]);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  test("offers it when the browser already allows notifications", async () => {
    pushBrowser({ permission: "granted" });
    pushServer();

    await openToPrompt();
  });

  test("does not show when the server sends no Web Push", async () => {
    pushBrowser();
    const read = vi.fn();
    server.use(
      http.get(`${apiBase}/preferences/notifications`, () => {
        read();
        return HttpResponse.json(notificationSettingsFixture);
      }),
    );

    await openNotifications();
    await settled();

    expect(promptRegion()).not.toBeInTheDocument();
    expect(read).not.toHaveBeenCalled();
  });

  test("does not show in a browser without the Push API", async () => {
    pushServer();

    await openNotifications();
    await settled();

    expect(promptRegion()).not.toBeInTheDocument();
  });

  test("does not show when the browser blocks notifications", async () => {
    pushBrowser({ permission: "denied" });
    pushServer();

    await openNotifications();
    await settled();

    expect(promptRegion()).not.toBeInTheDocument();
  });

  test("does not show while the subscription is not known, nor when this app has one", async () => {
    pushBrowser();
    const push = pushServer(subscription);

    await openNotifications();
    expect(promptRegion()).not.toBeInTheDocument();
    await settled(() => push.reads > 0);

    expect(promptRegion()).not.toBeInTheDocument();
  });

  test("does not show when the subscription cannot be read", async () => {
    pushBrowser();
    pushServer();
    let reads = 0;
    server.use(
      http.get(`${apiBase}/push/subscription`, () => {
        reads++;
        return problem(500, "about:blank", "Server error");
      }),
    );

    await openNotifications();
    await settled(() => reads > 0);

    expect(promptRegion()).not.toBeInTheDocument();
  });

  test("does not show when the notification settings cannot be read", async () => {
    pushBrowser();
    const push = pushServer();
    let reads = 0;
    server.use(
      http.get(`${apiBase}/preferences/notifications`, () => {
        reads++;
        return problem(500, "about:blank", "Server error");
      }),
    );

    await openNotifications();
    await settled(() => reads > 0 && push.reads > 0);

    expect(promptRegion()).not.toBeInTheDocument();
  });

  test("does not show to an account that said no, and does to another one on the same browser", async () => {
    pushBrowser();
    const push = pushServer();
    pushPromptDismissedKey.write([viewerFixture.id]);

    const first = await openNotifications();
    await settled(() => push.reads > 0);
    expect(promptRegion()).not.toBeInTheDocument();
    first.unmount();

    pushPromptDismissedKey.write(["01900000-0000-7000-8000-0000000000aa"]);
    await openToPrompt();
  });
});

describe("the flow of the prompt (ADR-0137 §2)", () => {
  test("closing it subscribes nothing, moves the focus to the tabs, and is remembered", async () => {
    const browser = pushBrowser();
    const push = pushServer();
    const { user, prompt } = await openToPrompt();

    await user.click(within(prompt).getByRole("button", { name: "Close" }));

    expect(promptRegion()).not.toBeInTheDocument();
    expect(selectedTab()).toHaveFocus();
    expect(pushPromptDismissedKey.read()).toEqual([viewerFixture.id]);
    expect(browser.notifications.requestPermission).not.toHaveBeenCalled();
    expect(browser.pushManager.subscribe).not.toHaveBeenCalled();
    expect(push.puts).toEqual([]);
  });

  test("the session's end keeps that it was closed", async () => {
    pushPromptDismissedKey.write([viewerFixture.id]);

    clearUserStorage();

    expect(pushPromptDismissedKey.read()).toEqual([viewerFixture.id]);
  });

  test("turning it on asks the browser, registers every type, and offers the settings", async () => {
    const browser = pushBrowser();
    const push = pushServer();
    const { user, prompt, router } = await openToPrompt();

    await user.click(within(prompt).getByRole("button", { name: "Turn on" }));

    await waitFor(() => {
      expect(promptRegion()).not.toBeInTheDocument();
    });
    expect(browser.notifications.requestPermission).toHaveBeenCalledOnce();
    expect(push.puts.map((put) => put.alerts)).toEqual([everyUserType]);
    expect(selectedTab()).toHaveFocus();
    expect(await screen.findByText("Push notifications are on for this device.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Choose types" }));

    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/settings/notifications");
    });
  });

  test("when the browser's prompt is closed without a choice, it says why and can be tried again", async () => {
    const browser = pushBrowser({ answer: "default" });
    const push = pushServer();
    const { user, prompt } = await openToPrompt();

    await user.click(within(prompt).getByRole("button", { name: "Turn on" }));

    expect(await within(prompt).findByRole("alert")).toHaveTextContent(/did not allow|not allowed/i);
    expect(browser.pushManager.subscribe).not.toHaveBeenCalled();
    expect(push.puts).toEqual([]);
    expect(pushPromptDismissedKey.read()).toEqual([]);

    browser.notifications.requestPermission.mockImplementation(() => Promise.resolve("granted"));
    await user.click(within(prompt).getByRole("button", { name: "Turn on" }));

    await waitFor(() => {
      expect(promptRegion()).not.toBeInTheDocument();
    });
    expect(push.puts).toHaveLength(1);
  });

  test("when the browser blocks it, it says why, leaves only closing, and is not offered again", async () => {
    const browser = pushBrowser();
    const push = pushServer();
    browser.notifications.requestPermission.mockImplementation(() => {
      browser.notifications.permission = "denied";
      return Promise.resolve("denied");
    });
    const { user, prompt } = await openToPrompt();

    await user.click(within(prompt).getByRole("button", { name: "Turn on" }));

    expect(await screen.findByText(/blocked/i)).toBeInTheDocument();
    const region = promptRegion();
    expect(region).toBeInTheDocument();
    expect(within(region as HTMLElement).queryByRole("button", { name: "Turn on" })).not.toBeInTheDocument();
    const close = within(region as HTMLElement).getByRole("button", { name: "Close" });
    expect(close).toHaveFocus();
    expect(push.puts).toEqual([]);

    await user.click(close);

    expect(promptRegion()).not.toBeInTheDocument();
    expect(selectedTab()).toHaveFocus();
  });

  test("the next visit after blocking shows nothing, closed or not", async () => {
    const browser = pushBrowser();
    pushServer();
    browser.notifications.requestPermission.mockImplementation(() => {
      browser.notifications.permission = "denied";
      return Promise.resolve("denied");
    });
    const first = await openToPrompt();
    await first.user.click(within(first.prompt).getByRole("button", { name: "Turn on" }));
    await screen.findByText(/blocked/i);
    first.unmount();

    await openNotifications();
    await settled();

    expect(promptRegion()).not.toBeInTheDocument();
    expect(pushPromptDismissedKey.read()).toEqual([]);
  });

  test("when subscribing fails, it says why and can be tried again", async () => {
    const browser = pushBrowser();
    const push = pushServer();
    browser.pushManager.subscribe.mockImplementationOnce(() => Promise.reject(new Error("no push service")));
    const { user, prompt } = await openToPrompt();

    await user.click(within(prompt).getByRole("button", { name: "Turn on" }));

    expect(await within(prompt).findByRole("alert")).toBeInTheDocument();
    expect(push.puts).toEqual([]);

    await user.click(within(prompt).getByRole("button", { name: "Turn on" }));

    await waitFor(() => {
      expect(promptRegion()).not.toBeInTheDocument();
    });
    expect(push.puts).toHaveLength(1);
  });

  test("when the server refuses the subscription, it says why and stays", async () => {
    pushBrowser();
    pushServer();
    server.use(http.put(`${apiBase}/push/subscription`, () => problem(500, "about:blank", "Server error")));
    const { user, prompt } = await openToPrompt();

    await user.click(within(prompt).getByRole("button", { name: "Turn on" }));

    expect(await within(prompt).findByRole("alert")).toBeInTheDocument();
    expect(promptRegion()).toBeInTheDocument();
  });
});

describe("what is remembered (ADR-0137 §4)", () => {
  async function pushSwitch(path = "/settings/notifications") {
    const view = await renderApp({ path });
    return { ...view, control: await screen.findByRole("switch", { name: /Notify me on this device/ }) };
  }

  test("turning it off in the settings counts as saying no", async () => {
    pushBrowser({ permission: "granted" });
    pushServer(subscription);
    const { user, control } = await pushSwitch();

    await user.click(control);

    await waitFor(() => {
      expect(pushPromptDismissedKey.read()).toEqual([viewerFixture.id]);
    });
  });

  test("turning it on in the settings forgets it", async () => {
    pushBrowser();
    pushServer();
    pushPromptDismissedKey.write([viewerFixture.id, "01900000-0000-7000-8000-0000000000aa"]);
    const { user, control } = await pushSwitch();

    await user.click(control);

    await waitFor(() => {
      expect(pushPromptDismissedKey.read()).toEqual(["01900000-0000-7000-8000-0000000000aa"]);
    });
  });

  test("turning it on from the prompt leaves other accounts' answers", async () => {
    pushBrowser();
    pushServer();
    pushPromptDismissedKey.write(["01900000-0000-7000-8000-0000000000aa"]);
    const { user, prompt } = await openToPrompt();

    await user.click(within(prompt).getByRole("button", { name: "Turn on" }));

    await waitFor(() => {
      expect(promptRegion()).not.toBeInTheDocument();
    });
    expect(pushPromptDismissedKey.read()).toEqual(["01900000-0000-7000-8000-0000000000aa"]);
  });
});
