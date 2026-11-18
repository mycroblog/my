// SPDX-License-Identifier: AGPL-3.0-only
// The Web Push subscription of the signed-in grant (FR-206, ADR-0086 §18–§22): the server's, which a 404 says there is
// none of; the kinds of notifications the reader gets (the rows of their notification settings), which a subscription
// pushes all of when it is made from the prompt; and registering this browser's subscription. Whether this account
// closed the prompt in this browser is kept on the device, never sent: closing it, or turning push off, keeps the
// record; turning it on takes it away.
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
import { useSyncExternalStore } from "react";
import { viewerQuery } from "./accounts";
import type { AppData } from "./app-data";
import { instanceQuery } from "./instance";
import type { NotificationType } from "./notifications";
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
async function registerPush(data: AppData, subscription: PushSubscriptionData, alerts: readonly NotificationType[]) {
  const { data: saved } = await data.api.PUT("/push/subscription", {
    body: { endpoint: subscription.endpoint, keys: subscription.keys, alerts: [...alerts] },
  });
  if (saved !== undefined) data.queryClient.setQueryData(pushSubscriptionQuery(data).queryKey, saved);
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
  await data.api.DELETE("/push/subscription");
  data.queryClient.setQueryData(pushSubscriptionQuery(data).queryKey, null);
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

/** Whether notifications play a sound on this device, following changes of the switch. */
export function useNotificationSound(): boolean {
  return useSyncExternalStore(
    (listener) => {
      soundListeners.add(listener);
      return () => {
        soundListeners.delete(listener);
      };
    },
    notificationSoundOn,
    () => false,
  );
}
