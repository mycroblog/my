// SPDX-License-Identifier: AGPL-3.0-only

// What each kind of notification shows (FR-205): its glyph, the sentence of its Web Push notification
// (notifications.kind.*, which reuses the server's words) with its values, and its shape. A notification about a post
// that speaks to the reader (a mention, a quote, a new post of an account they subscribe to) shows the post whole, the
// sentence over it; the others are the line of their kind, with the reader's post they are about as its first words,
// or the account and what the reader may do with it. A kind the client does not know reads as a new notification.

import { AccountName } from "@my/app-components";
import type { NotificationItem, NotificationType } from "@my/app-data";
import { SentenceName, type IconName } from "@my/design-system";
import { Fragment, type ReactNode } from "react";
import type { stringsIn } from "../locale";
import type { MessageId } from "../messages.gen";

type T = ReturnType<typeof stringsIn>["t"];

/** How a notification is drawn: the post whole, or the line of its kind. */
export type Shape = "post" | "line";

const glyphs: Partial<Record<NotificationType, IconName>> = {
  mention: "reply",
  quote: "quote",
  status: "notifications",
  favourite: "favourite",
  boost: "boost",
  reaction: "react",
  follow: "follow",
  follow_request: "followers",
  follow_accepted: "follow",
  account_moved: "open",
  poll_closed: "poll",
  moderation_notice: "moderation",
  report_resolved: "moderation",
  appeal_decided: "moderation",
  hook_appeal_decided: "moderation",
  relationships_severed: "moderation",
  custom_emoji_request_decided: "moderation",
  announcement: "announcements",
  voice_room_started: "room",
  voice_room_invited: "room",
  reply_request: "reply",
  boost_request: "boost",
  quote_request: "quote",
};

/** The glyph of a kind: its own, a moderator's notice's shield, or the bell. */
export const glyphOf = (type: NotificationType): IconName =>
  glyphs[type] ?? (type.startsWith("admin_") ? "moderation" : "notifications");

/** The kinds drawn as the post they are about, when the reader may see it. */
const postKinds: ReadonlySet<NotificationType> = new Set(["mention", "quote", "status"]);

export const shapeOf = (n: NotificationItem): Shape =>
  postKinds.has(n.type) && n.status !== undefined ? "post" : "line";

// The first of the characters no text holds (the private use area), each standing for a part drawn by React inside a
// formatted string.
const firstSlot = 0xe000;
const slotPattern = /([\u{E000}-\u{F8FF}])/u;

/** The string `id` with `values`, and each of `parts` drawn by React in its argument's place. */
function format(
  t: T,
  id: MessageId,
  values: Readonly<Record<string, string | number>>,
  parts: Readonly<Record<string, ReactNode>>,
): ReactNode {
  const names = Object.keys(parts);
  const slots = Object.fromEntries(names.map((name, i) => [name, String.fromCodePoint(firstSlot + i)]));
  return t(id, { ...values, ...slots })
    .split(slotPattern)
    .map((piece, i) => {
      if (i % 2 === 0) return piece;
      const name = names[(piece.codePointAt(0) ?? firstSlot) - firstSlot] ?? "";
      return <Fragment key={i}>{parts[name]}</Fragment>;
    });
}

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
  return format(t, id, message.values, { name });
}
