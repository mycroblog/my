// SPDX-License-Identifier: AGPL-3.0-only
// The Web Push subscription of the signed-in grant (FR-206, ADR-0086 §18–§22): the server's, which a 404 says there is
// none of; the kinds of notifications the reader gets (the rows of their notification settings), which a subscription
// pushes all of when it is made from the prompt; and registering this browser's subscription. Whether this account
// closed the prompt in this browser is kept on the device, never sent: closing it, or turning push off, keeps the
// record; turning it on takes it away.
import { ProblemError, type components } from "@mycroblog/mykit";
import { readDeviceValue, writeDeviceValue, type PushSubscriptionData } from "@my/platform";
import { useQuery } from "@tanstack/react-query";
import { useSyncExternalStore } from "react";
import type { AppData } from "./app-data";
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
