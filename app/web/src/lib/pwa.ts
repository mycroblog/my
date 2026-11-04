// SPDX-License-Identifier: AGPL-3.0-only
// The page's side of the Service Worker (ADR-0028, app/web/sw/sw.ts): registering it, swapping in a waiting
// worker when no tab needs the old one, offering an update to a tab that stayed old for long and when this version
// stops working, clearing the caches at sign-out, and reloading when a deploy removed the chunks of the running version.
import { useSyncExternalStore } from "react";
import { isSafeToReload } from "./reloadSafety";
import { preloadReloadKey, requiredReloadKey, updateSeenKey } from "./storage";
import {
  cachesClearedMessage,
  clearCachesMessage,
  hasPrecachedMessage,
  isMessage,
  precachedMessageType,
  skipWaitingMessage,
} from "./sw-protocol";
import { webClientEpoch } from "./webClientEpoch";

/**
 * What the shell offers: `stale` is a tab that kept an old build for long (ADR-0028 §11), `required` a version that no
 * longer works (§4). `apply` reloads the page, which runs the new build whichever worker controls it.
 */
export type PendingUpdate = { kind: "stale" | "required"; apply: () => void };

let pendingUpdate: PendingUpdate | null = null;
const listeners = new Set<() => void>();

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function getPendingUpdate(): PendingUpdate | null {
  return pendingUpdate;
}

function setPendingUpdate(update: PendingUpdate | null): void {
  pendingUpdate = update;
  for (const listener of listeners) listener();
}

/** The URL of the worker, which vite-plugin-pwa builds into dist/sw.js and `my serve` serves at the root. */
const scriptURL = "/sw.js";

/** The part of the Trusted Types API (https://developer.mozilla.org/docs/Web/API/Trusted_Types_API) used here. */
type TrustedTypes = {
  createPolicy: (name: string, rules: { createScriptURL: (url: string) => string }) => ScriptURLPolicy;
};
type ScriptURLPolicy = { createScriptURL: (url: string) => unknown };

/** The policy of each Trusted Types factory, made on first use: the CSP allows its name only once. */
const policies = new WeakMap<TrustedTypes, ScriptURLPolicy>();

/**
 * The worker's URL as a TrustedScriptURL where the browser has Trusted Types: the shell's CSP sets
 * `require-trusted-types-for 'script'` and allows only the policies `my-service-worker` (this one) and
 * `my-server-sanitized-html` (@my/ui PostContent) with `trusted-types` (ADR-0019), and register() is a
 * sink for script URLs. The policy passes the one fixed URL and nothing else.
 */
function trustedScriptURL(): string {
  const factory = (globalThis as { trustedTypes?: TrustedTypes }).trustedTypes;
  if (factory === undefined) return scriptURL;
  let policy = policies.get(factory);
  if (policy === undefined) {
    policy = factory.createPolicy("my-service-worker", {
      createScriptURL(url) {
        if (url !== scriptURL) throw new TypeError(`not the Service Worker's URL: ${url}`);
        return url;
      },
    });
    policies.set(factory, policy);
  }
  // register() accepts the TrustedScriptURL in place of the string its DOM type names.
  return policy.createScriptURL(scriptURL) as string;
}

/** How long a tab keeps an old build before it offers the update: one toast per tab per week, whatever the deploys (ADR-0028 §11). */
const staleAfterMs = 7 * 24 * 60 * 60 * 1000;

/** How long the page waits for the waiting worker's answer. */
const workerAnswerTimeoutMs = 3000;

/** The name of the shared lock of each tab's build: the locks of the origin say which builds still run. */
const buildLockPrefix = "my:build:";

/** Whether `worker` precached the file at `url`; null when it does not answer. */
function workerHasPrecached(worker: ServiceWorker, url: string): Promise<boolean | null> {
  return new Promise((resolve) => {
    const channel = new MessageChannel();
    const timer = setTimeout(() => {
      done(null);
    }, workerAnswerTimeoutMs);
    function done(answer: boolean | null) {
      clearTimeout(timer);
      channel.port1.close();
      resolve(answer);
    }
    channel.port1.onmessage = (event: MessageEvent<unknown>) => {
      if (isMessage(event.data, precachedMessageType)) done((event.data as { has?: unknown }).has === true);
    };
    worker.postMessage(hasPrecachedMessage(url), [channel.port2]);
  });
}

/** Whether a tab of a build other than `ownLock` runs on this origin. */
async function otherBuildRuns(locks: LockManager, ownLock: string): Promise<boolean> {
  const { held = [] } = await locks.query();
  return held.some(({ name }) => name?.startsWith(buildLockPrefix) === true && name !== ownLock);
}

export type BuildIdentity = {
  /** The URL of this build's entry chunk (`import.meta.url` of the entry). */
  build: string;
  /** `navigator.locks`; a browser without Web Locks swaps workers when all its tabs close. */
  locks: LockManager | undefined;
};

/**
 * Registers the worker at /sw.js for the whole origin
 * (https://developer.mozilla.org/docs/Web/API/ServiceWorkerContainer/register). The first worker takes over at once
 * (it claims the page). A later version installs in the background and waits, and nothing shows (ADR-0028 §9):
 * - On a page of the worker's build, it is told to take over without a reload, once no tab of another build runs:
 *   activating deletes the chunks those tabs load (§2). It never reloads a page, so another tab's Update keeps working.
 * - On a page of an older build, usePendingUpdate() offers a reload once the update has waited for a week (§3).
 * The browser looks for a new version when the page loads; the page also asks whenever it becomes visible again,
 * since an open app may never navigate, and that is also when it notices that the other tabs have closed.
 */
export async function registerServiceWorker(
  container: ServiceWorkerContainer,
  reload: () => void,
  { build, locks }: BuildIdentity,
): Promise<void> {
  const ownLock = buildLockPrefix + build;
  // Held for as long as the tab lives; the browser releases it with the tab.
  locks?.request(ownLock, { mode: "shared" }, () => new Promise<void>(() => undefined)).catch(() => undefined);

  const registration = await container.register(trustedScriptURL(), { scope: "/" });
  let staleTimer: ReturnType<typeof setTimeout> | undefined;

  const withdrawStale = () => {
    if (pendingUpdate?.kind === "stale") setPendingUpdate(null);
  };

  const reconcile = async () => {
    clearTimeout(staleTimer);
    const waiting = registration.waiting;
    if (waiting === null) {
      updateSeenKey.write(null);
      withdrawStale();
      return;
    }
    // Without a controller this is the first worker, not an update.
    if (container.controller === null) return;
    // A newer deploy replacing the waiting worker keeps the time, or frequent deploys would never reach the week.
    const seenAt = updateSeenKey.read() ?? Date.now();
    updateSeenKey.write(seenAt);

    const sameBuild = await workerHasPrecached(waiting, build);
    if (registration.waiting !== waiting || sameBuild === null) return;
    if (sameBuild) {
      withdrawStale();
      if (locks !== undefined && !(await otherBuildRuns(locks, ownLock))) waiting.postMessage(skipWaitingMessage);
      return;
    }
    const remaining = seenAt + staleAfterMs - Date.now();
    if (remaining > 0) {
      staleTimer = setTimeout(run, remaining);
    } else if (pendingUpdate === null) {
      setPendingUpdate({ kind: "stale", apply: reload });
    }
  };
  function run() {
    reconcile().catch((error: unknown) => {
      console.error("the waiting Service Worker could not be checked", error);
    });
  }

  run();
  registration.addEventListener("updatefound", () => {
    const worker = registration.installing;
    if (worker === null) return;
    worker.addEventListener("statechange", () => {
      if (worker.state === "installed") run();
    });
  });
  container.addEventListener("controllerchange", run);
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState !== "visible") return;
    registration.update().catch(() => undefined);
    run();
  });
}

/** What the shell offers now, or null: nothing shows while the running version keeps working. */
export function usePendingUpdate(): PendingUpdate | null {
  return useSyncExternalStore(subscribe, getPendingUpdate, () => null);
}

/** How often a required update looks for the safe moment. */
const safeMomentPollMs = 3000;

let safeMomentPoll: ReturnType<typeof setInterval> | undefined;

/**
 * This version no longer works (ADR-0028 §12): offers Update at once, and `load` runs by itself at the first safe moment
 * (lib/reloadSafety) unless `mayLoad` is false. A call or a write in flight is never cut.
 */
function offerRequiredUpdate(load: () => void, mayLoad: boolean): void {
  if (pendingUpdate?.kind !== "required") setPendingUpdate({ kind: "required", apply: load });
  if (!mayLoad || safeMomentPoll !== undefined) return;
  safeMomentPoll = setInterval(() => {
    if (!isSafeToReload()) return;
    clearInterval(safeMomentPoll);
    safeMomentPoll = undefined;
    load();
  }, safeMomentPollMs);
}

/**
 * Compares the server's `webClientEpoch`, from a response of the client API, with this build's. Only a larger value
 * asks for an update: a smaller one is a rollback, or an old server in a rolling deploy. A tab reloads by itself once
 * for each epoch; if that does not help (an old shell in a rolling deploy), the toast stays.
 */
export function checkServerEpoch(header: string | null, reload: () => void): void {
  if (header === null) return;
  const epoch = Number(header);
  if (!Number.isInteger(epoch) || epoch <= webClientEpoch) return;
  const reloaded = requiredReloadKey.read();
  const mayLoad = reloaded === null || reloaded < epoch;
  const load = () => {
    requiredReloadKey.write(epoch);
    reload();
  };
  if (mayLoad && isSafeToReload()) load();
  else offerRequiredUpdate(load, mayLoad);
}

/** Forgets what this module holds, as a new page would: tests share the module. */
export function resetUpdatesForTest(): void {
  clearInterval(safeMomentPoll);
  safeMomentPoll = undefined;
  pendingUpdate = null;
}

/** How long sign-out waits for the worker to confirm that it cleared its caches. */
const clearTimeoutMs = 3000;

/**
 * Deletes every cache of the origin, the precache included, so that nothing stays on the device after
 * sign-out (ADR-0028); lib/auth/userData calls it when the session ends. The worker deletes its media cache with its expiration metadata; the page then
 * deletes whatever is left, which also covers a page without a controlling worker. The app keeps working
 * online; it works offline again once a new version of the worker precaches the build.
 */
export async function clearCaches(): Promise<void> {
  const controller = "serviceWorker" in navigator ? navigator.serviceWorker.controller : null;
  if (controller !== null) await askWorkerToClear(controller);
  if (!("caches" in globalThis)) return;
  const names = await caches.keys();
  await Promise.all(names.map((name) => caches.delete(name)));
}

function askWorkerToClear(worker: ServiceWorker): Promise<void> {
  return new Promise((resolve) => {
    const channel = new MessageChannel();
    const timer = setTimeout(done, clearTimeoutMs);
    function done() {
      clearTimeout(timer);
      channel.port1.close();
      resolve();
    }
    channel.port1.onmessage = (event: MessageEvent) => {
      if (isMessage(event.data, cachesClearedMessage.type)) done();
    };
    worker.postMessage(clearCachesMessage, [channel.port2]);
  });
}

/** A second failure within this time is not reloaded again, so that a broken deploy cannot loop. */
const reloadGuardMs = 10_000;

/**
 * Reloads the page when a chunk of the running version fails to load, which happens after a deploy removed it
 * (https://vite.dev/guide/build#load-error-handling). When a reload would lose something (lib/reloadSafety) it offers
 * the required update instead and the error reaches the error boundaries, as it also does if the reload does not help.
 */
export function reloadOnPreloadError(target: Window): () => void {
  const onPreloadError = (event: Event) => {
    // The time of the last reload is kept in this tab's sessionStorage (lib/storage), which a reload keeps.
    const now = Date.now();
    const last = preloadReloadKey.read();
    preloadReloadKey.write(now);
    if (last !== null && now - last < reloadGuardMs) return;
    const load = () => {
      preloadReloadKey.write(Date.now());
      target.location.reload();
    };
    if (isSafeToReload()) {
      event.preventDefault();
      load();
    } else {
      offerRequiredUpdate(load, true);
    }
  };
  target.addEventListener("vite:preloadError", onPreloadError);
  return () => {
    target.removeEventListener("vite:preloadError", onPreloadError);
  };
}
