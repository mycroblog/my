// SPDX-License-Identifier: AGPL-3.0-only
// The Web Push subscription of the signed-in grant (FR-206, ADR-0086 §18–§22): the server's, which a 404 says there is
// none of; the kinds of notifications the reader gets (the rows of their notification settings), which a subscription
// pushes all of when it is made from the prompt; and registering this browser's subscription. Whether this account
// closed the prompt in this browser is kept on the device, never sent: closing it, or turning push off, keeps the
// record; turning it on takes it away. The notification sound is the device's too: whether it plays at all, and for
// which kinds, and telling when a notification the stream brings is to sound; playing it is the platform's.
import { ProblemError, type components } from "@mycroblog/mykit";
import {
  askNotificationPermission,
  notificationPermission,
  readDeviceValue,
  subscribePush,
  supportsPush,
  writeDeviceValue,
  type NotificationPermissionState,
  type PushSubscriptionData,
} from "@my/platform";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useSyncExternalStore } from "react";
import { viewerQuery } from "./accounts";
import type { AppData } from "./app-data";
import { inOrder } from "./in-order";
import { instanceQuery } from "./instance";
import { watchNotifications, type NotificationType } from "./notifications";
import { useAppData } from "./provider";

/** The Web Push subscription of the grant. */
export type PushSubscription = components["schemas"]["PushSubscription"];

/** The grant's subscription; null when it has none (404). */
export function pushSubscriptionQuery({ api }: AppData) {
  return {
    queryKey: ["get", "/push/subscription"] as const,
    queryFn: async ({ signal }: { signal: AbortSignal }): Promise<PushSubscription | null> => {
      try {
        const { data } = await api.GET("/push/subscription", { signal });
        return data ?? null;
      } catch (error) {
        if (error instanceof ProblemError && error.status === 404) return null;
        throw error;
      }
    },
  };
}

/** The reader's notification settings: the rows of the kinds they get. */
export function notificationSettingsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/preferences/notifications");
}

/** The grant's subscription: undefined while it is read or when it could not be, null when there is none. */
export function usePushSubscription(): PushSubscription | null | undefined {
  return useQuery(pushSubscriptionQuery(useAppData())).data;
}

/** Every kind of notification the reader gets (the rows of their settings), each once; undefined until read. */
export function useNotificationKinds(): readonly NotificationType[] | undefined {
  const rows = useQuery(notificationSettingsQuery(useAppData())).data?.rows;
  return rows === undefined ? undefined : [...new Set(rows.flatMap((r) => r.types))];
}

/**
 * Registers this browser's subscription for the notifications of `alerts`, and puts what the server answered into the
 * cache; it rejects with the server's problem.
 */
async function putPush(data: AppData, subscription: PushSubscriptionData, alerts: readonly NotificationType[]) {
  const { data: saved } = await data.api.PUT("/push/subscription", {
    body: { endpoint: subscription.endpoint, keys: subscription.keys, alerts: [...alerts] },
  });
  if (saved !== undefined) data.queryClient.setQueryData(pushSubscriptionQuery(data).queryKey, saved);
}

/** The document the writes of the subscription are one of, each sent once the ones before it were answered. */
const subscriptionWrites = "/push/subscription";

/** Registers the subscription (putPush), once the writes of it made before were answered. */
function registerPush(data: AppData, subscription: PushSubscriptionData, alerts: readonly NotificationType[]) {
  return inOrder(data, subscriptionWrites, () => putPush(data, subscription, alerts));
}

/** Registers a subscription for the session of the nearest AppDataProvider (registerPush). */
export function useRegisterPush(): (
  subscription: PushSubscriptionData,
  alerts: readonly NotificationType[],
) => Promise<void> {
  const data = useAppData();
  return (subscription, alerts) => registerPush(data, subscription, alerts);
}

// ---- The prompt closed, by account, on this device -----------------------------------------------------------------

/** The device's key of the accounts that closed the Web Push prompt here; kept on sign-out. */
const closedKey = "my:push-prompt-closed";

const listeners = new Set<() => void>();

function closedAccounts(): readonly string[] {
  try {
    const parsed: unknown = JSON.parse(readDeviceValue(closedKey) ?? "[]");
    return Array.isArray(parsed) ? parsed.filter((id): id is string => typeof id === "string") : [];
  } catch {
    return [];
  }
}

/** Keeps (`closed`) or takes away the record that the account of `id` closed the Web Push prompt on this device. */
export function rememberPushPromptClosed(id: string, closed: boolean): void {
  const others = closedAccounts().filter((x) => x !== id);
  writeDeviceValue(closedKey, JSON.stringify(closed ? [...others, id] : others));
  for (const listener of listeners) listener();
}

/** Whether the account of `id` closed the Web Push prompt on this device. */
export function usePushPromptClosed(id: string | undefined): boolean {
  return useSyncExternalStore(
    (listener) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    () => id !== undefined && closedAccounts().includes(id),
    () => false,
  );
}

// ---- The switch of Web Push, in the settings ------------------------------------------------------------------------

/** Where turning Web Push on ended: on, refused in the browser, the question closed unanswered, or failed. */
export type PushTurnOn = "on" | "blocked" | "closed" | "failed";

/**
 * Turns Web Push on for this device and the account of `accountId`, as the prompt does (ADR-0086 §21): asks the
 * browser's permission when it was never given, subscribes with the server's `key`, registers the subscription for
 * `alerts`, and takes away the record that the prompt was closed. Where it ended is told, never thrown.
 */
export async function turnPushOn(
  data: AppData,
  accountId: string,
  key: string,
  alerts: readonly NotificationType[],
): Promise<PushTurnOn> {
  let permission = notificationPermission();
  if (permission === "default") permission = await askNotificationPermission();
  if (permission === "denied") return "blocked";
  if (permission !== "granted") return "closed";
  try {
    await registerPush(data, await subscribePush(key), alerts);
  } catch {
    return "failed";
  }
  rememberPushPromptClosed(accountId, false);
  return "on";
}

/**
 * Turns Web Push off for the grant: the server's subscription is deleted and the cache holds none, and the record that
 * the account closed the prompt on this device is kept (ADR-0086 §22), so that the prompt does not come back. It rejects
 * with the server's problem.
 */
export async function turnPushOff(data: AppData, accountId: string): Promise<void> {
  await inOrder(data, subscriptionWrites, async () => {
    await data.api.DELETE("/push/subscription");
    data.queryClient.setQueryData(pushSubscriptionQuery(data).queryKey, null);
  });
  rememberPushPromptClosed(accountId, true);
}

/**
 * Changes the kinds this device's subscription pushes to `alerts`, and resolves once the cache holds the subscription
 * as the server answered. The browser's subscription (made with the server's `key`) is registered again with the new
 * kinds. It rejects with the server's problem, or when the browser refuses.
 */
export async function changePushAlerts(data: AppData, key: string, alerts: readonly NotificationType[]): Promise<void> {
  await registerPush(data, await subscribePush(key), alerts);
}

/**
 * Turns the pushes of `kinds` (those of rows of the settings) on or off for this device's subscription, the others
 * staying as they are, and resolves once the cache holds the subscription as the server answered. Each change starts
 * from what the one before it left, so that changes made together all hold. It rejects with the server's problem,
 * when the browser refuses, or when the grant has no subscription.
 */
function changePushKinds(data: AppData, key: string, kinds: readonly NotificationType[], on: boolean): Promise<void> {
  return inOrder(data, subscriptionWrites, async () => {
    const held = data.queryClient.getQueryData<PushSubscription | null>(pushSubscriptionQuery(data).queryKey);
    if (held === undefined || held === null) throw new Error("app-data: no push subscription to change");
    const others = held.alerts.filter((kind) => !kinds.includes(kind));
    await putPush(data, await subscribePush(key), on ? [...others, ...kinds] : others);
  });
}

/** What a screen of the notification settings holds of Web Push, and can do. */
export type PushSwitch = {
  /** The grant's subscription; null when it has none, undefined until read. */
  readonly subscription: PushSubscription | null | undefined;
  /** Whether this browser can receive pushes at all. */
  readonly supported: boolean;
  /** What the reader said to the browser's notifications. */
  readonly permission: NotificationPermissionState;
  readonly turnOn: (alerts: readonly NotificationType[]) => Promise<PushTurnOn>;
  readonly turnOff: () => Promise<void>;
  readonly changeAlerts: (alerts: readonly NotificationType[]) => Promise<void>;
  /** Turns the pushes of `kinds` on or off, the others staying (`changePushKinds`). */
  readonly changeKinds: (kinds: readonly NotificationType[], on: boolean) => Promise<void>;
};

/**
 * The Web Push switch of the signed-in account, from a screen; its writes need the viewer and the server's push key,
 * and reject until both are read.
 */
export function usePushSwitch(): PushSwitch {
  const data = useAppData();
  const viewer = useQuery(viewerQuery(data)).data;
  const key = useQuery(instanceQuery(data)).data?.push_server_key;
  const subscription = usePushSubscription();
  const need = () => {
    if (viewer === undefined || key === undefined) throw new Error("app-data: the push switch is not ready");
    return { id: viewer.id, key };
  };
  return {
    subscription,
    supported: supportsPush(),
    permission: notificationPermission(),
    turnOn: (alerts) => {
      const ready = need();
      return turnPushOn(data, ready.id, ready.key, alerts);
    },
    turnOff: () => turnPushOff(data, need().id),
    changeAlerts: (alerts) => changePushAlerts(data, need().key, alerts),
    changeKinds: (kinds, on) => changePushKinds(data, need().key, kinds, on),
  };
}

// ---- The sound of notifications, on this device ----------------------------------------------------------------------

/** The device's key of the master switch of the notification sound (ADR-0086 §12). */
const soundKey = "my:notification-sound";

const soundListeners = new Set<() => void>();

/** Whether notifications play a sound on this device; off until the reader turns it on. Playing it is not data's. */
export function notificationSoundOn(): boolean {
  return readDeviceValue(soundKey) === "on";
}

/** Turns the notification sound on or off on this device. */
export function setNotificationSound(on: boolean): void {
  writeDeviceValue(soundKey, on ? "on" : null);
  for (const listener of soundListeners) listener();
}

const watchSound = (listener: () => void) => {
  soundListeners.add(listener);
  return () => {
    soundListeners.delete(listener);
  };
};

/** Whether notifications play a sound on this device, following changes of the switch. */
export function useNotificationSound(): boolean {
  return useSyncExternalStore(watchSound, notificationSoundOn, () => false);
}

/** The device's key of the kinds of notifications that sound (the cells of the sound column, ADR-0086 §12). */
const soundKindsKey = "my:notification-sound-kinds";

const noKinds: readonly NotificationType[] = [];

/** The kinds as last read from the device, the same list for the same text kept. */
let soundKindsRead: { text: string | null; kinds: readonly NotificationType[] } = { text: null, kinds: noKinds };

/** The kinds of notifications that sound on this device; none until the reader chooses some. */
function notificationSoundKinds(): readonly NotificationType[] {
  const text = readDeviceValue(soundKindsKey);
  if (text === soundKindsRead.text) return soundKindsRead.kinds;
  let kinds = noKinds;
  try {
    const parsed: unknown = JSON.parse(text ?? "[]");
    // What the device keeps is this app's own, of any version of it: a kind no longer known sounds for nothing.
    if (Array.isArray(parsed)) kinds = parsed.filter((kind): kind is NotificationType => typeof kind === "string");
  } catch {
    kinds = noKinds;
  }
  soundKindsRead = { text, kinds };
  return kinds;
}

/** Turns the sound of the notifications of `kinds` (those of a row of the settings) on or off on this device. */
export function setNotificationSoundKinds(kinds: readonly NotificationType[], on: boolean): void {
  const others = notificationSoundKinds().filter((kind) => !kinds.includes(kind));
  const now = on ? [...others, ...kinds] : others;
  writeDeviceValue(soundKindsKey, now.length === 0 ? null : JSON.stringify(now));
  for (const listener of soundListeners) listener();
}

/** The kinds of notifications that sound on this device, following the reader's changes. */
export function useNotificationSoundKinds(): readonly NotificationType[] {
  return useSyncExternalStore(watchSound, notificationSoundKinds, () => noKinds);
}

/** How long after a notification that sounded the next ones are silent, in ms: a burst sounds once. */
const soundQuietMs = 3000;

/**
 * Calls `play` when the stream brings a notification that sounds on this device (ADR-0086 §12): the sound is on, the
 * notification's kind is among those that sound, and none that sounds arrived in the moments before, so that a burst
 * is one sound. Until the function it returns is called.
 */
export function watchNotificationSound(data: AppData, play: () => void): () => void {
  let last = Number.NEGATIVE_INFINITY;
  return watchNotifications(data, ({ type }) => {
    if (!notificationSoundOn() || !notificationSoundKinds().includes(type)) return;
    const now = Date.now();
    const quiet = now - last >= soundQuietMs;
    // A burst that goes on stays one sound.
    last = now;
    if (quiet) play();
  });
}

/** Calls `play` for the notifications that sound on this device, while the app shows (`watchNotificationSound`). */
export function useNotificationSounds(play: () => void): void {
  const data = useAppData();
  useEffect(() => watchNotificationSound(data, play), [data, play]);
}
