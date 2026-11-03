// SPDX-License-Identifier: AGPL-3.0-only
// The states of the update flow of ADR-0141: the worker swap under Web Locks and the other tabs (§2), the tab that stays
// old (§3), and the required update (§4). jsdom has no Service Workers or Web Locks, so all of them are stand-ins, and a
// tab of another build is a lock that its tab holds.
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import {
  checkServerEpoch,
  type PendingUpdate,
  registerServiceWorker,
  reloadOnPreloadError,
  resetUpdatesForTest,
  usePendingUpdate,
} from "./pwa";
import { registerReloadBlocker } from "./reloadSafety";
import { preloadReloadKey, requiredReloadKey, updateSeenKey } from "./storage";
import { skipWaitingMessage } from "./sw-protocol";
import { webClientEpoch } from "./webClientEpoch";

const build = "http://localhost/assets/index-new.js";
const day = 24 * 60 * 60 * 1000;

/** A worker that answers whether it precached the page's entry chunk, or nothing with `undefined`. */
function fakeWorker(hasBuild: boolean | undefined) {
  const postMessage = vi.fn((message: { type: string }, ports?: MessagePort[]) => {
    if (message.type === "HAS_PRECACHED" && hasBuild !== undefined) {
      ports?.[0]?.postMessage({ type: "PRECACHED", has: hasBuild });
    }
  });
  return Object.assign(new EventTarget(), { postMessage, state: "installed" });
}

function setup(
  waiting: ReturnType<typeof fakeWorker> | null,
  { controller = {} }: { controller?: object | null } = {},
) {
  const registration = Object.assign(new EventTarget(), {
    waiting,
    installing: null as ReturnType<typeof fakeWorker> | null,
    update: vi.fn(() => Promise.resolve()),
  });
  const container = Object.assign(new EventTarget(), {
    controller,
    register: vi.fn(() => Promise.resolve(registration)),
  });
  return { container: container as unknown as ServiceWorkerContainer, registration, reload: vi.fn() };
}

/** The Web Locks of the origin, where each name of `otherTabs` is held by another tab. */
function fakeLocks(otherTabs: string[] = []) {
  const request = vi.fn(() => new Promise(() => undefined));
  const query = vi.fn(() => Promise.resolve({ held: otherTabs.map((name) => ({ name, mode: "shared" })) }));
  return { locks: { request, query } as unknown as LockManager, request, query };
}

function skipWaitingSent(worker: ReturnType<typeof fakeWorker>): boolean {
  return worker.postMessage.mock.calls.some(([message]) => message === skipWaitingMessage);
}

function offered(): PendingUpdate | null {
  const { result, unmount } = renderHook(() => usePendingUpdate());
  const update = result.current;
  unmount();
  return update;
}

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  resetUpdatesForTest();
});

afterEach(() => {
  vi.useRealTimers();
  resetUpdatesForTest();
});

/** Lets the answers on the message ports and the lock queries arrive. */
async function settled() {
  await act(() => new Promise((resolve) => setTimeout(resolve, 10)));
}

describe("the worker swap", () => {
  test("holds a shared lock named after the build", async () => {
    const { container, reload } = setup(null);
    const { locks, request } = fakeLocks();
    await registerServiceWorker(container, reload, { build, locks });
    expect(request).toHaveBeenCalledWith(`my:build:${build}`, { mode: "shared" }, expect.any(Function));
  });

  test("the first worker is no update", async () => {
    const worker = fakeWorker(true);
    const { container, reload } = setup(worker, { controller: null });
    await registerServiceWorker(container, reload, { build, locks: fakeLocks().locks });
    await settled();
    expect(skipWaitingSent(worker)).toBe(false);
    expect(offered()).toBeNull();
    expect(updateSeenKey.read()).toBeNull();
  });

  test("a page of the waiting worker's build swaps it in, with no toast and no reload", async () => {
    const worker = fakeWorker(true);
    const { container, reload } = setup(worker);
    await registerServiceWorker(container, reload, { build, locks: fakeLocks([`my:build:${build}`]).locks });
    await settled();
    expect(skipWaitingSent(worker)).toBe(true);
    expect(offered()).toBeNull();

    container.dispatchEvent(new Event("controllerchange"));
    expect(reload).not.toHaveBeenCalled();
  });

  test("waits while a tab of another build runs, and swaps when it is gone and the page shows again", async () => {
    const worker = fakeWorker(true);
    const { container, reload, registration } = setup(worker);
    const { locks, query } = fakeLocks(["my:build:http://localhost/assets/index-old.js"]);
    await registerServiceWorker(container, reload, { build, locks });
    await settled();
    expect(skipWaitingSent(worker)).toBe(false);
    expect(offered()).toBeNull();

    query.mockResolvedValue({ held: [] });
    document.dispatchEvent(new Event("visibilitychange"));
    await settled();
    expect(registration.update).toHaveBeenCalled();
    expect(skipWaitingSent(worker)).toBe(true);
  });

  test("ignores the locks of other names", async () => {
    const worker = fakeWorker(true);
    const { container, reload } = setup(worker);
    await registerServiceWorker(container, reload, { build, locks: fakeLocks(["my:other"]).locks });
    await settled();
    expect(skipWaitingSent(worker)).toBe(true);
  });

  test("without Web Locks it leaves the swap to closing every tab", async () => {
    const worker = fakeWorker(true);
    const { container, reload } = setup(worker);
    await registerServiceWorker(container, reload, { build, locks: undefined });
    await settled();
    expect(skipWaitingSent(worker)).toBe(false);
    expect(offered()).toBeNull();
  });

  test("a page of an older build never swaps, since that would delete its own chunks", async () => {
    const worker = fakeWorker(false);
    const { container, reload } = setup(worker);
    await registerServiceWorker(container, reload, { build, locks: fakeLocks().locks });
    await settled();
    expect(skipWaitingSent(worker)).toBe(false);
  });

  test("a worker that does not answer changes nothing", async () => {
    vi.useFakeTimers();
    const worker = fakeWorker(undefined);
    const { container, reload } = setup(worker);
    updateSeenKey.write(Date.now() - 8 * day);
    await registerServiceWorker(container, reload, { build, locks: fakeLocks().locks });
    await vi.advanceTimersByTimeAsync(5000);
    expect(skipWaitingSent(worker)).toBe(false);
    expect(offered()).toBeNull();
  });

  test("a worker that finishes installing is checked too", async () => {
    const { container, reload, registration } = setup(null);
    await registerServiceWorker(container, reload, { build, locks: fakeLocks().locks });
    const worker = fakeWorker(true);
    registration.installing = worker;
    registration.dispatchEvent(new Event("updatefound"));
    registration.installing = null;
    registration.waiting = worker;
    worker.dispatchEvent(new Event("statechange"));
    await settled();
    expect(skipWaitingSent(worker)).toBe(true);
  });
});

describe("a tab that stays old", () => {
  test("shows nothing for a week, then offers a reload, and Update does nothing else", async () => {
    vi.useFakeTimers();
    const start = Date.now();
    const worker = fakeWorker(false);
    const { container, reload } = setup(worker);
    await registerServiceWorker(container, reload, { build, locks: fakeLocks().locks });
    await vi.advanceTimersByTimeAsync(1);
    expect(updateSeenKey.read()).toBe(start);
    expect(offered()).toBeNull();

    await vi.advanceTimersByTimeAsync(7 * day - 1000);
    expect(offered()).toBeNull();

    await vi.advanceTimersByTimeAsync(2000);
    const update = offered();
    expect(update?.kind).toBe("stale");

    update?.apply();
    expect(reload).toHaveBeenCalledOnce();
    expect(skipWaitingSent(worker)).toBe(false);
  });

  test("counts from when another tab first found the update, and a newer worker keeps the time", async () => {
    const seen = Date.now() - 8 * day;
    updateSeenKey.write(seen);
    const { container, reload } = setup(fakeWorker(false));
    await registerServiceWorker(container, reload, { build, locks: fakeLocks().locks });
    await settled();
    expect(offered()?.kind).toBe("stale");
    expect(updateSeenKey.read()).toBe(seen);
  });

  test("a reload that brings the waiting worker's build offers nothing, however long it waited", async () => {
    updateSeenKey.write(Date.now() - 8 * day);
    const { container, reload } = setup(fakeWorker(true));
    await registerServiceWorker(container, reload, { build, locks: fakeLocks(["my:build:old"]).locks });
    await settled();
    expect(offered()).toBeNull();
  });

  test("the swap forgets the time and withdraws the offer", async () => {
    updateSeenKey.write(Date.now() - 8 * day);
    const { container, reload, registration } = setup(fakeWorker(false));
    await registerServiceWorker(container, reload, { build, locks: fakeLocks().locks });
    await settled();
    expect(offered()?.kind).toBe("stale");

    registration.waiting = null;
    container.dispatchEvent(new Event("controllerchange"));
    await settled();
    expect(updateSeenKey.read()).toBeNull();
    expect(offered()).toBeNull();
  });
});

describe("the required update of a newer epoch", () => {
  const newer = String(webClientEpoch + 1);

  test.each([
    ["no header", null],
    ["the same epoch", String(webClientEpoch)],
    ["an older server", String(webClientEpoch - 1)],
    ["a value that is no number", "soon"],
  ])("does nothing with %s", (_name, header) => {
    const reload = vi.fn();
    checkServerEpoch(header, reload);
    expect(reload).not.toHaveBeenCalled();
    expect(offered()).toBeNull();
  });

  test("reloads at once at a safe moment", () => {
    const reload = vi.fn();
    checkServerEpoch(newer, reload);
    expect(reload).toHaveBeenCalledOnce();
    expect(requiredReloadKey.read()).toBe(webClientEpoch + 1);
  });

  test("reloads once per epoch; if that did not help, the toast stays", async () => {
    vi.useFakeTimers();
    requiredReloadKey.write(webClientEpoch + 1);
    const reload = vi.fn();
    checkServerEpoch(newer, reload);
    expect(reload).not.toHaveBeenCalled();
    expect(offered()?.kind).toBe("required");
    await vi.advanceTimersByTimeAsync(60_000);
    expect(reload).not.toHaveBeenCalled();
  });

  test("a still newer epoch reloads again", () => {
    requiredReloadKey.write(webClientEpoch + 1);
    const reload = vi.fn();
    checkServerEpoch(String(webClientEpoch + 2), reload);
    expect(reload).toHaveBeenCalledOnce();
  });

  test("offers Update while it is not safe, and reloads as soon as it is", async () => {
    vi.useFakeTimers();
    let busy = true;
    const remove = registerReloadBlocker(() => busy);
    const reload = vi.fn();
    checkServerEpoch(newer, reload);
    checkServerEpoch(newer, reload);
    const update = offered();
    expect(update?.kind).toBe("required");
    expect(reload).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(30_000);
    expect(reload).not.toHaveBeenCalled();

    busy = false;
    await vi.advanceTimersByTimeAsync(3000);
    expect(reload).toHaveBeenCalledOnce();
    await vi.advanceTimersByTimeAsync(30_000);
    expect(reload).toHaveBeenCalledOnce();
    remove();
  });

  test("the user may reload at once, which counts as the epoch's reload", () => {
    const remove = registerReloadBlocker(() => true);
    const reload = vi.fn();
    checkServerEpoch(newer, reload);
    offered()?.apply();
    expect(reload).toHaveBeenCalledOnce();
    expect(requiredReloadKey.read()).toBe(webClientEpoch + 1);
    remove();
  });

  test("replaces the stale offer", async () => {
    updateSeenKey.write(Date.now() - 8 * day);
    const { container, reload } = setup(fakeWorker(false));
    await registerServiceWorker(container, reload, { build, locks: fakeLocks().locks });
    await settled();
    expect(offered()?.kind).toBe("stale");

    const remove = registerReloadBlocker(() => true);
    checkServerEpoch(newer, vi.fn());
    expect(offered()?.kind).toBe("required");
    remove();
  });
});

describe("a chunk that is gone", () => {
  function fakeWindow() {
    const reload = vi.fn();
    const target = Object.assign(new EventTarget(), { location: { reload } });
    return { window: target as unknown as Window, reload };
  }

  function preloadError(): Event {
    return new Event("vite:preloadError", { cancelable: true });
  }

  test("does not reload while it is not safe: it offers Update, the error goes on, and it reloads when safe", async () => {
    vi.useFakeTimers();
    let busy = true;
    const remove = registerReloadBlocker(() => busy);
    const { window, reload } = fakeWindow();
    const stop = reloadOnPreloadError(window);

    const event = preloadError();
    window.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(false);
    expect(reload).not.toHaveBeenCalled();
    expect(offered()?.kind).toBe("required");

    busy = false;
    await vi.advanceTimersByTimeAsync(3000);
    expect(reload).toHaveBeenCalledOnce();
    // The guard counts from the reload, so that a failure right after it does not reload again.
    expect(preloadReloadKey.read()).toBe(Date.now());
    stop();
    remove();
  });

  test("keeps the guard: a failure right after a reload offers nothing new and lets the error through", () => {
    preloadReloadKey.write(Date.now());
    const { window, reload } = fakeWindow();
    const stop = reloadOnPreloadError(window);
    const event = preloadError();
    window.dispatchEvent(event);
    expect(reload).not.toHaveBeenCalled();
    expect(event.defaultPrevented).toBe(false);
    expect(offered()).toBeNull();
    stop();
  });
});
