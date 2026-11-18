// SPDX-License-Identifier: AGPL-3.0-only
// The viewer's choices of which notifications they get (FR-206, ADR-0086): the cells of their notification settings,
// per kind and channel, and the policy that holds back or drops the notifications of accounts that do not know them.
// The rows are read in push.ts, with the Web Push subscription and the sound of this device; a write puts what the
// server answered into the cache, and a new policy marks the held-back notifications.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { inOrder } from "./in-order";
import { instanceQuery } from "./instance";
import { preferencesQuery } from "./preferences";
import { notificationSettingsQuery, pushSubscriptionQuery } from "./push";
import { useAppData } from "./provider";

/** The rows of the viewer's notification settings. */
export type NotificationSettings = components["schemas"]["NotificationSettings"];

/** A row of the settings: a kind of notifications, and whether the viewer gets it in the app and by email. */
export type NotificationSettingRow = components["schemas"]["NotificationSettingRow"];

/** The kind of a row of the settings. */
export type NotificationGroup = components["schemas"]["NotificationGroup"];

/** A channel a cell of the settings can change: the app's own notifications, or email. */
export type NotificationChannel = "in_app" | "email";

/** A cell to change: whether the viewer gets the notifications of a row through a channel. */
export type NotificationCellChange = {
  readonly group: NotificationGroup;
  readonly channel: NotificationChannel;
  readonly enabled: boolean;
};

/**
 * Changes the cells of `changes` (the others keep their value), and resolves once the cache holds the settings as the
 * server answered. It rejects with the server's problem (a cell that is fixed, a row the viewer does not get). A
 * change is sent once the ones made before it were answered.
 */
export function changeNotificationSettings(data: AppData, changes: readonly NotificationCellChange[]): Promise<void> {
  return inOrder(data, "/preferences/notifications", async () => {
    const { data: saved } = await data.api.PATCH("/preferences/notifications", { body: { changes: [...changes] } });
    if (saved !== undefined) data.queryClient.setQueryData(notificationSettingsQuery(data).queryKey, saved);
  });
}

/** The rows of the viewer's notification settings, undefined until the server answered. */
export function useNotificationSettings(): NotificationSettings | undefined {
  return useQuery(notificationSettingsQuery(useAppData())).data;
}

/**
 * Where the reads of what the screen of the notification settings shows are: the rows, the policy, the grant's Web
 * Push subscription, the viewer's preferences and the instance. `loading` until the server answered them all, `failed`
 * when it did not answer one, with `retry`, which reads again those that failed.
 */
export function useNotificationSettingsRead(): { status: "loading" | "failed" | "read"; retry: () => void } {
  const data = useAppData();
  const reads = [
    useQuery(notificationSettingsQuery(data)),
    useQuery(notificationPolicyQuery(data)),
    useQuery(pushSubscriptionQuery(data)),
    useQuery(preferencesQuery(data)),
    useQuery(instanceQuery(data)),
  ];
  return {
    status: reads.some((r) => r.status === "error")
      ? "failed"
      : reads.every((r) => r.status === "success")
        ? "read"
        : "loading",
    retry: () => {
      for (const r of reads) if (r.status === "error") void r.refetch();
    },
  };
}

/** Changes cells of the notification settings, from a screen (`changeNotificationSettings`). */
export function useChangeNotificationSettings(): (changes: readonly NotificationCellChange[]) => Promise<void> {
  const data = useAppData();
  return (changes) => changeNotificationSettings(data, changes);
}

// ---- The stopped address -------------------------------------------------------------------------------------------

/** A verification email the server did not send, having sent as many as it sends for now. */
export type VerificationWait = {
  /**
   * How long the server asks to wait before the next one (the `Retry-After` of its 429), in ms; undefined when it
   * did not say.
   */
  readonly waitMs: number | undefined;
};

/** The wait a `Retry-After` header asks for, in seconds or as an HTTP date, in ms from now; undefined for none. */
function retryAfterMs(header: string | null): number | undefined {
  const value = header?.trim() ?? "";
  if (/^\d+$/.test(value)) return Number(value) * 1000;
  const at = value === "" ? Number.NaN : Date.parse(value);
  return Number.isNaN(at) ? undefined : Math.max(at - Date.now(), 0);
}

/**
 * Sends the viewer a new verification email, whose link lifts the stop of email to their address after a hard bounce
 * (`Preferences.email_suppression`): `sent` once it is queued, or the wait the server asks for when it sent as many as
 * it sends for now (a 429). An address that is no longer stopped has nothing to verify: the preferences are read
 * again, and it is `sent`. It rejects with any other problem.
 */
export async function resendVerificationEmail(data: AppData): Promise<"sent" | VerificationWait> {
  try {
    await data.api.POST("/accounts/email/verification");
  } catch (error) {
    if (error instanceof ProblemError && error.status === 429)
      return { waitMs: retryAfterMs(error.headers.get("Retry-After")) };
    if (!(error instanceof ProblemError && error.status === 409)) throw error;
    await data.queryClient.invalidateQueries({ queryKey: preferencesQuery(data).queryKey });
  }
  return "sent";
}

/** Sends a new verification email, from a screen (`resendVerificationEmail`). */
export function useResendVerificationEmail(): () => Promise<"sent" | VerificationWait> {
  const data = useAppData();
  return () => resendVerificationEmail(data);
}

// ---- The policy ----------------------------------------------------------------------------------------------------

/** What the viewer's policy does with the notifications of a source: let them through, hold them back, or drop them. */
export type NotificationPolicyAction = components["schemas"]["NotificationPolicyAction"];

/** The policy: its action for each kind of source. */
export type NotificationPolicy = components["schemas"]["NotificationPolicy"];

/** The viewer's notification policy. */
export function notificationPolicyQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/preferences/notification_policy");
}

/** The viewer's notification policy, undefined until the server answered. */
export function useNotificationPolicy(): NotificationPolicy | undefined {
  return useQuery(notificationPolicyQuery(useAppData())).data;
}

/**
 * Changes the actions of `change` (the others stay), and resolves once the cache holds the policy as the server
 * answered and the notifications it holds back, their summary, and the notifications are read again.
 * It rejects with the server's problem.
 */
export function changeNotificationPolicy(data: AppData, change: Partial<NotificationPolicy>): Promise<void> {
  const { queryClient } = data;
  return inOrder(data, "/preferences/notification_policy", async () => {
    const { data: saved } = await data.api.PATCH("/preferences/notification_policy", { body: change });
    if (saved !== undefined) queryClient.setQueryData(notificationPolicyQuery(data).queryKey, saved);
    await queryClient.invalidateQueries({
      predicate: ({ queryKey }) =>
        typeof queryKey[1] === "string" &&
        (queryKey[1] === "/notifications" || queryKey[1].startsWith("/notifications/requests")),
    });
  });
}

/** Changes the notification policy, from a screen (`changeNotificationPolicy`). */
export function useChangeNotificationPolicy(): (change: Partial<NotificationPolicy>) => Promise<void> {
  const data = useAppData();
  return (change) => changeNotificationPolicy(data, change);
}
