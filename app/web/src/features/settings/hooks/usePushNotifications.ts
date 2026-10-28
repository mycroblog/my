// SPDX-License-Identifier: AGPL-3.0-only
import { useAnnouncer } from "@my/ui";
import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { isTauriApp } from "../../../lib/platform";
import { shellQueryReader } from "../../../lib/query";
import { credentialsQueryOptions, instanceQueryOptions, type Permission } from "../api/me";
import {
  cachePushSubscription,
  deletePushSubscription,
  type NotificationType,
  type PushSubscriptionInfo,
  pushSubscriptionQueryOptions,
  savePushSubscription,
} from "../api/push";

/** A choice of the kinds of notifications to push: one checkbox, which stands for one or more types. */
type PushAlertGroup = { id: string; label: ClientMessageKey; types: readonly NotificationType[] };

/**
 * The permissions whose holders get moderator notifications (admin_report, admin_sign_up, admin_appeal,
 * admin_webhook_disabled for manage_webhooks, admin_custom_emoji_request for manage_custom_emojis,
 * admin_service_closure_due for manage_service_closure, and admin_error_group for manage_errors).
 */
const moderatorPermissions: readonly Permission[] = [
  "manage_reports",
  "manage_accounts",
  "manage_webhooks",
  "manage_custom_emojis",
  "manage_service_closure",
  "manage_errors",
];

/** The kinds that every user chooses from, in the order of the notifications' filter. */
const userGroups: readonly PushAlertGroup[] = [
  { id: "mention", label: "client:settings.push.type.mention", types: ["mention"] },
  { id: "follow", label: "client:settings.push.type.follow", types: ["follow"] },
  { id: "follow_request", label: "client:settings.push.type.follow_request", types: ["follow_request"] },
  { id: "follow_accepted", label: "client:settings.push.type.follow_accepted", types: ["follow_accepted"] },
  { id: "boost", label: "client:settings.push.type.boost", types: ["boost"] },
  { id: "favourite", label: "client:settings.push.type.favourite", types: ["favourite"] },
  { id: "reaction", label: "client:settings.push.type.reaction", types: ["reaction"] },
  { id: "quote", label: "client:settings.push.type.quote", types: ["quote"] },
  { id: "service_closure", label: "client:settings.push.type.service_closure", types: ["service_closure"] },
  { id: "account_moved", label: "client:settings.push.type.account_moved", types: ["account_moved"] },
  {
    id: "moderation",
    label: "client:settings.push.type.moderation",
    types: [
      "moderation_notice",
      "appeal_decided",
      "report_resolved",
      "relationships_severed",
      "custom_emoji_request_decided",
    ],
  },
];

/**
 * The supporter notifications (ADR-0072 §16), offered except in the Tauri app, which shows nothing of the supporter
 * plans (§14).
 */
const supporterGroup: PushAlertGroup = {
  id: "supporter",
  label: "client:settings.push.type.supporter",
  types: [
    "supporter_payment_failed",
    "supporter_subscription_ended",
    "supporter_plan_retired",
    "supporter_service_closure",
  ],
};

/** The moderator notifications, offered to a viewer whose roles get them. */
const moderatorGroup: PushAlertGroup = {
  id: "admin",
  label: "client:settings.push.type.admin",
  types: [
    "admin_report",
    "admin_sign_up",
    "admin_appeal",
    "admin_webhook_disabled",
    "admin_custom_emoji_request",
    "admin_account_moved",
    "admin_service_closure_due",
    "admin_error_group",
  ],
};

/** What the preferences show of Web Push on this device. */
export type PushNotifications =
  /** The server sends no Web Push (the instance has no `push_server_key`), or that or the subscription is not known yet. */
  | { status: "hidden" }
  /** This browser has no Push API (or no Service Worker), so it cannot receive Web Push. */
  | { status: "unsupported" }
  /** The user blocked notifications for this site in the browser. */
  | { status: "denied" }
  | {
      status: "available";
      /** Whether this signed-in app has a subscription: the value being saved while `isPending`. */
      enabled: boolean;
      /** The types pushed: the value being saved while `isPending`. */
      alerts: readonly NotificationType[];
      groups: readonly PushAlertGroup[];
      setEnabled: (enabled: boolean) => void;
      setGroup: (group: PushAlertGroup, selected: boolean) => void;
      isPending: boolean;
      /** Why the last change was not saved, localized; undefined when it was. */
      error: string | undefined;
    };

/** The parts of the browser that Web Push uses: the Push API and the Notifications API. */
function pushBrowser(): { container: ServiceWorkerContainer; notifications: typeof Notification } | null {
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
type PushChange = { enabled: true; alerts: readonly NotificationType[]; ask: boolean } | { enabled: false };

/**
 * Returns whether and how the preferences offer Web Push on this device (FR-206), on a page whose loader has loaded
 * the viewer's account (it suspends until then). Turning it on asks the browser for the permission to notify,
 * subscribes with the server's key and registers the subscription for this signed-in app with every offered kind;
 * changing a kind registers it again; turning it off deletes the server's subscription and the browser's.
 */
export function usePushNotifications(): PushNotifications {
  const api = useApi();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const t = useT();
  const browser = pushBrowser();
  const account = useSuspenseQuery(credentialsQueryOptions(api));
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  const serverKey = instance.data?.push_server_key;
  const subscription = useQuery({
    ...pushSubscriptionQueryOptions(api),
    enabled: serverKey !== undefined && browser !== null,
  });
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
    onSuccess: (saved) => {
      cachePushSubscription(api, queryClient, saved);
      announce(t("client:settings.saved"));
    },
  });

  if (serverKey === undefined) return { status: "hidden" };
  if (browser === null) return { status: "unsupported" };
  if (browser.notifications.permission === "denied") return { status: "denied" };
  // The section waits for the subscription of the grant, so that its switch never shows off before it is known.
  if (subscription.isPending) return { status: "hidden" };

  const everyone = isTauriApp() ? userGroups : [...userGroups, supporterGroup];
  const groups = account.data.permissions.some((p) => moderatorPermissions.includes(p))
    ? [...everyone, moderatorGroup]
    : everyone;
  const offered = groups.flatMap((group) => group.types);
  const saved = subscription.data ?? null;
  const pending = mutation.isPending ? mutation.variables : undefined;
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
    alerts,
    groups,
    setEnabled: (on) => {
      mutation.mutate(on ? { enabled: true, alerts: offered, ask: true } : { enabled: false });
    },
    setGroup: (group, selected) => {
      const next = selected
        ? [...new Set([...alerts, ...group.types])]
        : alerts.filter((type) => !group.types.includes(type));
      mutation.mutate({ enabled: true, alerts: next, ask: false });
    },
    isPending: mutation.isPending,
    error,
  };
}
