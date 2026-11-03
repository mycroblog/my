// SPDX-License-Identifier: AGPL-3.0-only
// The toast about a new version (ADR-0141), in the whole app (web-client/README.md T4), and the call and the route as
// what decides whether it shows or the page reloads. jsdom has no Service Workers, so the container is a stand-in.
import { AnnouncerProvider, ToastProvider } from "@my/ui";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { AnyRouter } from "@tanstack/react-router";
import { act, screen, waitFor } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { checkServerEpoch, registerServiceWorker, resetUpdatesForTest } from "./lib/pwa";
import { reportCallActive } from "./lib/reloadSafety";
import { updateSeenKey } from "./lib/storage";
import { skipWaitingMessage } from "./lib/sw-protocol";
import { webClientEpoch, webClientEpochHeader } from "./lib/webClientEpoch";
import { renderApp, renderWithProviders } from "./test/render";
import { server } from "./test/msw/server";
import { UpdatePrompt } from "./UpdatePrompt";

const build = "http://localhost/assets/index-this.js";
const message = "A new version of the app is available.";
const requiredMessage = "The app has changed and needs an update to keep working.";

function fakeContainer(hasBuild: boolean) {
  const waiting = {
    postMessage: vi.fn((data: { type: string }, ports?: MessagePort[]) => {
      if (data.type === "HAS_PRECACHED") ports?.[0]?.postMessage({ type: "PRECACHED", has: hasBuild });
    }),
  };
  const registration = Object.assign(new EventTarget(), { waiting, installing: null, update: vi.fn() });
  const container = Object.assign(new EventTarget(), {
    controller: {},
    register: vi.fn(() => Promise.resolve(registration)),
  });
  return { container: container as unknown as ServiceWorkerContainer, register: container.register, waiting };
}

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  resetUpdatesForTest();
});

afterEach(() => {
  resetUpdatesForTest();
  reportCallActive(false);
});

test("a tab that stayed old for a week offers Update, which only reloads", async () => {
  updateSeenKey.write(Date.now() - 8 * 24 * 60 * 60 * 1000);
  const { container, register, waiting } = fakeContainer(false);
  const reload = vi.fn();
  await registerServiceWorker(container, reload, { build, locks: undefined });
  expect(register).toHaveBeenCalledWith("/sw.js", { scope: "/" });

  const { user } = await renderApp({ path: "/local" });
  await screen.findByRole("heading", { level: 1, name: "Local timeline" });
  const region = await screen.findByRole("region", { name: "Messages" });
  await waitFor(() => {
    expect(region).toHaveTextContent(message);
  });

  await user.click(screen.getByRole("button", { name: "Update" }));
  expect(reload).toHaveBeenCalledOnce();
  expect(waiting.postMessage).not.toHaveBeenCalledWith(skipWaitingMessage);
});

test("a new version shows nothing in the first week", async () => {
  const { container, waiting } = fakeContainer(false);
  await registerServiceWorker(container, vi.fn(), { build, locks: undefined });

  await renderApp({ path: "/local" });
  await screen.findByRole("heading", { level: 1, name: "Local timeline" });
  await waitFor(() => {
    expect(waiting.postMessage).toHaveBeenCalled();
  });
  expect(screen.queryByText(message)).not.toBeInTheDocument();
});

function respondWithNewerEpoch() {
  server.use(
    http.get("*/api/v1/instance", () =>
      HttpResponse.json(
        { error: "gone" },
        { status: 410, headers: { [webClientEpochHeader]: String(webClientEpoch + 1) } },
      ),
    ),
  );
}

test("a newer epoch reloads a page for reading by itself", async () => {
  respondWithNewerEpoch();
  const { reloads } = await renderApp({ path: "/local" });
  await waitFor(() => {
    expect(reloads.count).toBeGreaterThan(0);
  });
  expect(reloads.count).toBe(1);
});

test("a newer epoch on a page that did not declare itself safe offers Update instead", async () => {
  respondWithNewerEpoch();
  const { reloads } = await renderApp({ path: "/settings/preferences" });
  const region = await screen.findByRole("region", { name: "Messages" });
  await waitFor(() => {
    expect(region).toHaveTextContent(requiredMessage);
  });
  expect(reloads.count).toBe(0);
});

test("the pages for reading declare themselves safe, and a page with forms does not", async () => {
  const { router } = await renderApp({ path: "/local" });
  const safe = (path: string) => router.matchRoutes(path, {}).at(-1)?.staticData.reloadSafe === true;
  for (const path of [
    "/",
    "/home",
    "/local",
    "/public",
    "/notifications",
    "/tags/cats",
    "/@alice",
    "/@alice/status/1",
  ]) {
    expect(safe(path), path).toBe(true);
  }
  for (const path of ["/settings/profile", "/admin/settings", "/moderation/reports", "/voice"]) {
    expect(safe(path), path).toBe(false);
  }
});

/** Renders the prompt alone: a call is whatever the test reports. */
async function renderPrompt() {
  const client = new QueryClient();
  await renderWithProviders(
    <QueryClientProvider client={client}>
      <AnnouncerProvider>
        <ToastProvider regionLabel="Messages" dismissLabel="Dismiss">
          <UpdatePrompt router={{ state: { matches: [] } } as unknown as AnyRouter} />
        </ToastProvider>
      </AnnouncerProvider>
    </QueryClientProvider>,
  );
}

function setCall(active: boolean) {
  act(() => {
    reportCallActive(active);
  });
}

async function staleTab() {
  updateSeenKey.write(Date.now() - 8 * 24 * 60 * 60 * 1000);
  await registerServiceWorker(fakeContainer(false).container, vi.fn(), { build, locks: undefined });
}

test("during a call a stale tab shows nothing, and offers Update when the call ends", async () => {
  await staleTab();
  setCall(true);
  await renderPrompt();
  await new Promise((resolve) => setTimeout(resolve, 20));
  expect(screen.queryByText(message)).not.toBeInTheDocument();

  setCall(false);
  expect(await screen.findByText(message)).toBeInTheDocument();
});

test("a dismissed toast does not come back in this tab, also not after a call", async () => {
  await staleTab();
  await renderPrompt();
  const user = userEvent.setup();
  await screen.findByText(message);

  await user.click(screen.getByRole("button", { name: "Dismiss" }));
  await waitFor(() => {
    expect(screen.queryByText(message)).not.toBeInTheDocument();
  });

  setCall(true);
  setCall(false);
  await new Promise((resolve) => setTimeout(resolve, 20));
  expect(screen.queryByText(message)).not.toBeInTheDocument();
});

test("a required update shows during a call and does not reload", async () => {
  const reload = vi.fn();
  setCall(true);
  await renderPrompt();
  act(() => {
    checkServerEpoch(String(webClientEpoch + 1), reload);
  });
  expect(await screen.findByText(requiredMessage)).toBeInTheDocument();
  expect(reload).not.toHaveBeenCalled();
});
