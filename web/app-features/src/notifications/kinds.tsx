// SPDX-License-Identifier: AGPL-3.0-only

// What each kind of notification shows (FR-205): its glyph, the sentence of its Web Push notification
// (notifications.kind.*, which reuses the server's words) with its values, and its shape. A notification about a post
// that speaks to the reader (a mention, a quote, a new post of an account they subscribe to) shows the post whole, the
// sentence over it; the others are the line of their kind, with the post they are about as its first words (the
// reader's, or a new one on an event's board), or the account and what the reader may do with it. A kind the client does not know reads as a new notification.

import { AccountName } from "@my/app-components";
import type { NotificationItem, NotificationType } from "@my/app-data";
import { SentenceName, type IconName } from "@my/design-system";
import type { ReactNode } from "react";
import type { stringsIn } from "../locale";
import type { MessageId } from "../messages.gen";
import { formatParts } from "../slots";

type T = ReturnType<typeof stringsIn>["t"];

/** How a notification is drawn: the post whole, or the line of its kind. */
export type Shape = "post" | "line";

/** The glyph of every kind the client knows, by what it tells. */
const glyphs: Readonly<Record<NotificationType, IconName>> = {
  follow: "follow",
  follow_request: "followers",
  follow_accepted: "follow",
  mention: "reply",
  boost: "boost",
  favourite: "favourite",
  reaction: "react",
  quote: "quote",
  poll_closed: "poll",
  moderation_notice: "moderation",
  report_resolved: "moderation",
  admin_report: "report",
  admin_sign_up: "signUp",
  relationships_severed: "severed",
  appeal_decided: "moderation",
  admin_appeal: "appeal",
  admin_webhook_disabled: "webhookOff",
  admin_hook_failing: "reviewFailing",
  admin_domain_list_failing: "listFailing",
  admin_custom_emoji_request: "emojiCustom",
  custom_emoji_request_decided: "emojiCustom",
  supporter_payment_failed: "paymentFailed",
  supporter_subscription_ended: "supportEnded",
  supporter_plan_retired: "planRetired",
  account_moved: "moved",
  admin_account_moved: "moved",
  service_closure: "closure",
  admin_service_closure_due: "closure",
  supporter_service_closure: "closure",
  voice_room_started: "room",
  voice_room_invited: "room",
  admin_error_group: "errors",
  status: "subscribed",
  announcement: "announcements",
  admin_hook_appeal: "appeal",
  hook_appeal_decided: "moderation",
  auto_delete_limit: "autoDelete",
  scheduled_status_failed: "scheduleFailed",
  event_invited: "events",
  event_schedule_confirmed: "eventConfirmed",
  event_changed: "eventChanged",
  event_canceled: "eventCancelled",
  event_reminder: "eventReminder",
  event_started: "eventStarted",
  event_room_unavailable: "roomUnavailable",
  event_board_posted: "board",
  reply_request: "reply",
  boost_request: "boost",
  quote_request: "quote",
  interaction_accepted: "accepted",
  interaction_rejected: "rejected",
};

/** The glyph of a kind; the bell for a kind newer than the client, which the server may already send. */
export const glyphOf = (type: NotificationType): IconName =>
  (glyphs as Readonly<Record<string, IconName | undefined>>)[type] ?? "notifications";

/** The kinds drawn as the post they are about, when the reader may see it. */
const postKinds: ReadonlySet<NotificationType> = new Set(["mention", "quote", "status"]);

export const shapeOf = (n: NotificationItem): Shape =>
  postKinds.has(n.type) && n.status !== undefined ? "post" : "line";

const day = 86_400_000;

/** The message of a kind, and the values it reads besides the account's name. */
function messageOf(
  n: NotificationItem,
  instance: string,
  untitled: string,
): { id: MessageId; values: Record<string, string | number> } {
  const kind = (name: string) => `notifications.kind.${name}` as MessageId;
  const title = n.event?.title || untitled;
  switch (n.type) {
    case "announcement":
      return { id: kind("announcement"), values: { instance, title: n.announcement?.title ?? "" } };
    case "auto_delete_limit": {
      const limit = n.auto_delete_limit;
      const left = limit
        ? Math.max(0, Math.ceil((Date.parse(limit.enforce_from) - Date.parse(n.created_at)) / day))
        : 0;
      return { id: kind("auto_delete_limit"), values: { instance, days: limit?.max_age_days ?? 0, daysLeft: left } };
    }
    case "scheduled_status_failed":
      return {
        id: kind("scheduled_status_failed"),
        values: { instance, reason: n.scheduled_status_failure?.reason ?? "other" },
      };
    case "admin_error_group":
      return { id: kind(`admin_error_group_${n.error_group?.change ?? "created"}`), values: { instance } };
    case "service_closure":
      return { id: kind(`service_closure_${n.service_closure?.change ?? "announced"}`), values: { instance } };
    case "event_reminder": {
      const ms = n.event ? Date.parse(n.event.starts_at) - Date.parse(n.created_at) : 0;
      const [unit, size] = ms >= day ? ["day", day] : ms >= 3_600_000 ? ["hour", 3_600_000] : ["minute", 60_000];
      return { id: kind("event_reminder"), values: { title, unit, count: Math.max(1, Math.round(ms / size)) } };
    }
    default:
      if (n.type.startsWith("event_")) return { id: kind(n.type), values: { title } };
      return { id: kind(n.type), values: { instance } };
  }
}

/**
 * The sentence of a notification, the account's name drawn strong and isolated with its custom emoji; a kind with no
 * message of the catalog says that there is a new notification.
 */
export function sentenceOf(n: NotificationItem, t: T, instance: string, known: (id: string) => boolean): ReactNode {
  const untitled = t("notifications.untitled_event");
  const message = messageOf(n, instance, untitled);
  const id = known(message.id) ? message.id : ("notifications.kind.other" as MessageId);
  const name = n.account ? (
    <SentenceName>
      <AccountName account={n.account} />
    </SentenceName>
  ) : (
    ""
  );
  return formatParts(t, id, message.values, { name });
}
