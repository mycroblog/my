// SPDX-License-Identifier: AGPL-3.0-only
import { type SaveState, useNotice } from "@my/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { saveStateOf } from "./saveState";
import { shellQueryReader } from "../../../lib/query";
import { credentialsQueryOptions, instanceQueryOptions } from "../api/me";
import {
  cachePushSubscription,
  deletePushSubscription,
  type NotificationType,
  type PushSubscriptionInfo,
  pushSubscriptionQueryOptions,
  savePushSubscription,
} from "../api/push";
import { forgetPushPromptDismissed, rememberPushPromptDismissed } from "./pushPromptDismissal";

/** What the notification settings show of Web Push on this device. */
export type PushNotifications =
  /** The server sends no Web Push (the instance has no `push_server_key`), or that or the subscription is not known yet. */
  | { status: "hidden" }
  /** This browser has no Push API (or no Service Worker), so it cannot receive Web Push. */
  | { status: "unsupported" }
  /** The user blocked notifications for this site in the browser; `refused`: in the prompt of a change made here. */
  | { status: "denied"; refused: boolean }
  | {
      status: "available";
      /** Whether this signed-in app has a subscription: the value being saved while `isPending`. */
      enabled: boolean;
      /** Whether the server holds a subscription of this signed-in app, as saved; undefined when it could not be read. */
      subscribed: boolean | undefined;
      /** The types pushed: the value being saved while `isPending`. */
      alerts: readonly NotificationType[];
      setEnabled: (enabled: boolean) => void;
      /** Pushes `types` (one row of the notification settings, or the rows of a column) or stops pushing them. */
      setTypes: (types: readonly NotificationType[], selected: boolean) => void;
      isPending: boolean;
      /** Where the last change of the switch is, for its save mark (web-client/16-look-states-and-wording.md G24). */
      saveState: SaveState;
      /** Where the last change of the row of `types` is, for the save mark of its cell. */
      typesSaveState: (types: readonly NotificationType[]) => SaveState;
      /** Why the last change was not saved, localized; undefined when it was. */
      error: string | undefined;
    };

/** The parts of the browser that Web Push uses: the Push API and the Notifications API. */
export function pushBrowser(): { container: ServiceWorkerContainer; notifications: typeof Notification } | null {
  if (typeof navigator === "undefined" || !("serviceWorker" in navigator)) return null;
  if (typeof PushManager === "undefined" || typeof Notification === "undefined") return null;
  return { container: navigator.serviceWorker, notifications: Notification };
}

/** Decodes unpadded base64url, the encoding of the server's key and the subscription's keys. */
function fromBase64Url(text: string): Uint8Array<ArrayBuffer> {
  const binary = atob(text.replaceAll("-", "+").replaceAll("_", "/"));
  return Uint8Array.from(binary, (c) => c.charCodeAt(0));
}

function sameBytes(a: ArrayBuffer | null, b: Uint8Array): boolean {
  if (a === null || a.byteLength !== b.byteLength) return false;
  const view = new Uint8Array(a);
  return view.every((byte, i) => byte === b[i]);
}

/** Thrown when the user does not allow notifications when asked. */
class PermissionRefused extends Error {}

/**
 * Returns the browser's subscription for the server's key, subscribing when there is none or when it was made
 * with another key (https://developer.mozilla.org/docs/Web/API/PushManager/subscribe). Every message is shown
 * as a notification (`userVisibleOnly`), as the Service Worker does.
 */
async function browserSubscription(container: ServiceWorkerContainer, serverKey: string): Promise<PushSubscription> {
  const registration = await container.ready;
  const key = fromBase64Url(serverKey);
  const existing = await registration.pushManager.getSubscription();
  if (existing !== null) {
    if (sameBytes(existing.options.applicationServerKey, key)) return existing;
    await existing.unsubscribe();
  }
  return registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key });
}

/** The request that registers `subscription` for `alerts`. */
function subscriptionRequest(subscription: PushSubscription, alerts: readonly NotificationType[]) {
  const json = subscription.toJSON();
  return {
    endpoint: subscription.endpoint,
    keys: { p256dh: json.keys?.["p256dh"] ?? "", auth: json.keys?.["auth"] ?? "" },
    alerts: [...alerts],
  };
}

/** A change of Web Push: turning it on with the types, changing the types, or turning it off. */
/** A change of the subscription; `rows` are the types of the cells that changed it, none for the switch. */
type PushChange =
  | { enabled: true; alerts: readonly NotificationType[]; ask: boolean; rows?: readonly NotificationType[] | undefined }
  | { enabled: false };

/**
 * Returns whether and how the notification settings offer Web Push on this device (FR-206, ADR-0109 §1). Turning it on
 * asks the browser for the permission to notify, subscribes with the server's key and registers the subscription for
 * this signed-in app with every type of `offered` (the rows of the notification settings); changing a row registers it
 * again; turning it off deletes the server's subscription and the browser's.
 */
export function usePushNotifications(
  offered: readonly NotificationType[],
  /** Tells that the switch turned it on, in place of reading out the save mark. */
  onTurnedOn?: () => void,
): PushNotifications {
  const api = useApi();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const t = useT();
  const browser = pushBrowser();
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  const serverKey = instance.data?.push_server_key;
  const subscription = useQuery({
    ...pushSubscriptionQueryOptions(api),
    enabled: serverKey !== undefined && browser !== null,
  });
  const accountId = useQuery(credentialsQueryOptions(api)).data?.id;
  const mutation = useMutation({
    mutationKey: ["put", "/push/subscription"] as const,
    mutationFn: async (change: PushChange): Promise<PushSubscriptionInfo | null> => {
      if (browser === null || serverKey === undefined) return null;
      if (!change.enabled) {
        await deletePushSubscription(api);
        const registration = await browser.container.ready;
        await (await registration.pushManager.getSubscription())?.unsubscribe();
        return null;
      }
      if (change.ask && (await browser.notifications.requestPermission()) !== "granted") throw new PermissionRefused();
      const pushSubscription = await browserSubscription(browser.container, serverKey);
      return savePushSubscription(api, subscriptionRequest(pushSubscription, change.alerts));
    },
    onSuccess: (saved, change) => {
      cachePushSubscription(api, queryClient, saved);
      // Whoever turned it on hears of the prompt again when it stops; whoever turned it off said no (ADR-0137 §4).
      if (accountId !== undefined) {
        if (change.enabled) forgetPushPromptDismissed(accountId);
        else rememberPushPromptDismissed(accountId);
      }
      if (change.enabled && change.ask && onTurnedOn !== undefined) onTurnedOn();
      else notice.inPlace(t("client:settings.saved"));
    },
  });

  if (serverKey === undefined) return { status: "hidden" };
  if (browser === null) return { status: "unsupported" };
  if (browser.notifications.permission === "denied") {
    return { status: "denied", refused: mutation.isError && mutation.error instanceof PermissionRefused };
  }
  // The section waits for the subscription of the grant, so that its switch never shows off before it is known.
  if (subscription.isPending) return { status: "hidden" };

  const saved = subscription.data ?? null;
  const pending = mutation.isPending ? mutation.variables : undefined;
  const last = mutation.variables;
  const changedTypes = last?.enabled === true ? last.rows : undefined;
  const enabled = pending?.enabled ?? saved !== null;
  const alerts = pending?.enabled === true ? pending.alerts : (saved?.alerts ?? offered);
  let error: string | undefined;
  if (subscription.isError) error = errorMessage(subscription.error);
  if (mutation.isError) {
    const failure = mutation.error;
    if (failure instanceof PermissionRefused) error = t("client:settings.push.not_allowed");
    else if (failure instanceof ApiError) error = errorMessage(failure);
    else error = t("client:settings.push.failed");
  }
  return {
    status: "available",
    enabled,
    subscribed: subscription.isSuccess ? saved !== null : undefined,
    alerts,
    setEnabled: (on) => {
      mutation.mutate(on ? { enabled: true, alerts: offered, ask: true } : { enabled: false });
    },
    setTypes: (types, selected) => {
      const next = selected ? [...new Set([...alerts, ...types])] : alerts.filter((type) => !types.includes(type));
      mutation.mutate({ enabled: true, alerts: next, ask: false, rows: types });
    },
    isPending: mutation.isPending,
    saveState: changedTypes === undefined ? saveStateOf(mutation) : "idle",
    typesSaveState: (types) =>
      changedTypes !== undefined && types.some((type) => changedTypes.includes(type)) ? saveStateOf(mutation) : "idle",
    error,
  };
}
