// SPDX-License-Identifier: AGPL-3.0-only
// What a notification's row shows, as plain values for the slots of @my/ui's PostCard (web-client/README.md A4):
// the hook decides what to show and formats it; the component only places it. It returns no JSX.
import {
  type CustomEmojiImage,
  type ImageSources,
  type NotificationKind,
  sanitizedHtmlText,
  serverSanitizedHtml,
} from "@my/ui";
import { useIntl } from "react-intl";
import type { ClientMessageKey } from "../../../lib/i18n";
import { handleOf } from "../../../lib/handle";
import {
  accountStatusPath,
  adminErrorGroupPath,
  adminErrorsPath,
  moderationAppealPath,
  moderationReportPath,
  followListPath,
  profilePath,
} from "../../../lib/paths";
import { type TimeView, useTimeView } from "../../../lib/time";
import { useViewer } from "../../session";
import { statusPath } from "../../status";
import type {
  ModerationAction,
  Notification,
  NotificationAccount,
  NotificationStatus,
  NotificationType,
  SupporterEndReason,
} from "../api/notifications";
import type { NotificationGroup } from "./groupNotifications";
import { imageSources } from "../../../lib/images";
import { isTauriApp } from "../../../lib/platform";

/** An account in a row: its name in the UI (the username when it has no display name), profile page, and avatar URL. */
/** An account a row names: its display name with the custom emoji of its profile, its profile page, and its avatar. */
export type NotificationActor = {
  id: string;
  name: string;
  emojis: readonly CustomEmojiImage[];
  href: string;
  avatar: ImageSources | undefined;
};

/** A link from a row to the page where the user follows up what it tells. */
type NotificationLink = { href: string; label: ClientMessageKey };

/** What a `moderation_notice` tells of the action on the user's account, formatted for the row. */
export type ModerationNoticeView = {
  /** What the action does, and why, in the user's language as the server sends them. */
  kind: string;
  category: string;
  /** The moderators' explanation, their own text; null when they gave none. */
  reason: string | null;
  /** When the action ends by itself, as a date and time in the user's locale; null when it lasts until lifted. */
  until: { dateTime: string; text: string } | null;
  /**
   * For an `appeal_decided`: what the moderators decided on the user's appeal, and their explanation (their own
   * text, null when they gave none). Null for a `moderation_notice`, and while the appeal is pending.
   */
  decision: { sentence: ClientMessageKey; reason: string | null } | null;
};

/** What a supporter notification tells of the user's supporter subscription, formatted for the row. */
export type SupporterNoticeView = {
  /** The plan's name, written by the administrators in any language. */
  plan: string;
  /** The line under it: when the grace period ends, why the subscription ended, or what a retirement does; null for none. */
  detail:
    | { kind: "grace"; date: { dateTime: string; text: string } }
    | { kind: "ended"; reason: SupporterEndReason }
    | { kind: "retired" }
    | null;
};

/** The sentences of what became of the user's follow when an account they followed moved; each takes `{target}`. */
export type MoveOutcomeKey =
  | "client:notifications.account_moved.followed"
  | "client:notifications.account_moved.requested"
  | "client:notifications.account_moved.blocking"
  | "client:notifications.account_moved.blocked_by"
  | "client:notifications.account_moved.domain_muted";

/** What an `account_moved` tells of the account that the one the user followed moved to, formatted for the row. */
type MoveNoticeView = {
  /** The new account's handle (`@name@domain`), for `{target}` in the sentences of the row. */
  handle: string;
  /** The new account's profile page. */
  href: string;
  /** What became of the user's follow, as a sentence of `{target}`; null when the server did not say. */
  outcome: MoveOutcomeKey | null;
};

export type NotificationView = {
  /** The kind the row's mark shows on the avatar's corner (P3): the type at a glance, before the sentence is read. */
  kind: NotificationKind;
  /** The post to reply to from the row: the post of a mention, while the user may see it; null otherwise. */
  replyTo: NotificationStatus | null;
  /**
   * The sentence that says what happened; `{name}` in it is the actor's name, for the types that have an
   * actor, and `{count}` the number of the other actors of a group.
   */
  sentence: ClientMessageKey;
  /** The account that did it, the newest of a group; null for the types without an actor. */
  actor: NotificationActor | null;
  /**
   * The account that `{name}` names in a row without an actor, shown without an avatar: the account that a
   * `report_resolved`'s report is about. Null otherwise.
   */
  subject: NotificationActor | null;
  /**
   * The other accounts of a group, newest first, each once: the people who also favourited or boosted the
   * post. Empty for a row of one notification.
   */
  others: readonly NotificationActor[];
  /** When it happened, relative to now as in the timelines (web-client/16-look-states-and-wording.md G8). */
  time: TimeView;
  /**
   * The post it is about, as a quote that links to the post's page: its text without markup (the quote is
   * a link and cannot hold the post's own links), or only the author's content warning, which the post's
   * page asks to open; null for the types that are not about a post, and when the user may not see it.
   */
  post: { href: string; text: string; lang: string | undefined; contentWarning: string | null } | null;
  /** The account to offer a follow back to: the follower of a follow notification, else null. */
  followBack: NotificationAccount | null;
  /**
   * What a `relationships_severed` tells, for `{domain}`, `{followers}`, and `{following}` in its sentence:
   * the suspended server (null for an account, which is the actor), and how many followers and follows the
   * user lost; null for the other types.
   */
  severance: { domain: string | null; followers: number; following: number } | null;
  /** What a `moderation_notice` or an `appeal_decided` tells of the action; null for the other types. */
  notice: ModerationNoticeView | null;
  /**
   * The emoji of a `reaction`, for `{emoji}` in its sentence: the Unicode emoji, or the `:shortcode:` of a
   * custom emoji; null for the other types.
   */
  reaction: string | null;
  /**
   * The custom emoji request that an `admin_custom_emoji_request` or a `custom_emoji_request_decided` tells of:
   * its `:shortcode:`, for `{emoji}` in the sentence, and the administrator's reason of a rejection (their own text,
   * null when they gave none or for the other states); null for the other types and when the request is gone.
   */
  emojiRequest: { emoji: string; reason: string | null } | null;
  /** The subscription that a supporter notification tells of; null for the other types and when it is gone. */
  supporter: SupporterNoticeView | null;
  /** The account that an `account_moved` tells of the move to; null for the other types and when it is gone. */
  move: MoveNoticeView | null;
  /**
   * The pages the row leads to besides the actor's profile and the post, where the user follows up what it
   * tells: the account status, the report, the registrations, or the user's follow lists.
   */
  links: readonly NotificationLink[];
};

/** The mark of each type: the people's actions by what they did, the server's notices by who sends them. */
const kinds: Readonly<Record<NotificationType, NotificationKind>> = {
  follow: "follow",
  follow_request: "follow",
  follow_accepted: "follow",
  mention: "mention",
  boost: "boost",
  favourite: "favourite",
  reaction: "reaction",
  quote: "quote",
  poll_closed: "poll",
  moderation_notice: "staff",
  report_resolved: "staff",
  admin_report: "staff",
  admin_sign_up: "staff",
  relationships_severed: "staff",
  appeal_decided: "staff",
  admin_appeal: "staff",
  admin_webhook_disabled: "staff",
  admin_custom_emoji_request: "staff",
  custom_emoji_request_decided: "staff",
  supporter_payment_failed: "notice",
  supporter_subscription_ended: "notice",
  supporter_plan_retired: "notice",
  account_moved: "notice",
  admin_account_moved: "staff",
  service_closure: "notice",
  admin_service_closure_due: "staff",
  supporter_service_closure: "notice",
  voice_room_started: "voice",
  voice_room_invited: "voice",
  admin_error_group: "staff",
  status: "mention",
};

const sentences: Readonly<Record<NotificationType, ClientMessageKey>> = {
  follow: "client:notifications.type.follow",
  follow_request: "client:notifications.type.follow_request",
  follow_accepted: "client:notifications.type.follow_accepted",
  mention: "client:notifications.type.mention",
  boost: "client:notifications.type.boost",
  favourite: "client:notifications.type.favourite",
  reaction: "client:notifications.type.reaction",
  quote: "client:notifications.type.quote",
  poll_closed: "client:notifications.type.poll_closed",
  moderation_notice: "client:notifications.type.moderation_notice",
  report_resolved: "client:notifications.type.report_resolved",
  admin_report: "client:notifications.type.admin_report",
  admin_sign_up: "client:notifications.type.admin_sign_up",
  relationships_severed: "client:notifications.type.relationships_severed",
  appeal_decided: "client:notifications.type.appeal_decided",
  admin_appeal: "client:notifications.type.admin_appeal",
  admin_webhook_disabled: "client:notifications.type.admin_webhook_disabled",
  admin_custom_emoji_request: "client:notifications.type.admin_custom_emoji_request",
  custom_emoji_request_decided: "client:notifications.type.custom_emoji_request_decided",
  supporter_payment_failed: "client:notifications.type.supporter_payment_failed",
  supporter_subscription_ended: "client:notifications.type.supporter_subscription_ended",
  supporter_plan_retired: "client:notifications.type.supporter_plan_retired",
  account_moved: "client:notifications.type.account_moved",
  admin_account_moved: "client:notifications.type.admin_account_moved",
  service_closure: "client:notifications.type.service_closure",
  admin_service_closure_due: "client:notifications.type.admin_service_closure_due",
  supporter_service_closure: "client:notifications.type.supporter_service_closure",
  voice_room_started: "client:notifications.type.voice_room_started",
  voice_room_invited: "client:notifications.type.voice_room_invited",
  admin_error_group: "client:notifications.type.admin_error_group",
  status: "client:notifications.type.status",
};

/** The sentences of a `service_closure` that tells of a change of the dates or of a withdrawal. */
const closureSentences: Readonly<Record<string, ClientMessageKey>> = {
  rescheduled: "client:notifications.type.service_closure_rescheduled",
  withdrawn: "client:notifications.type.service_closure_withdrawn",
};

/** The sentence of a `service_closure` by what it tells of (ADR-0088 §2); null for the other types. */
function closureSentenceOf(notification: Notification): ClientMessageKey | null {
  const change = notification.type === "service_closure" ? notification.service_closure?.change : undefined;
  return change === undefined ? null : (closureSentences[change] ?? null);
}

/** The sentences of an `admin_error_group` by what it tells of (ADR-0103 §8). */
const errorGroupSentences: Readonly<Record<string, ClientMessageKey>> = {
  created: "client:notifications.type.admin_error_group_created",
  regressed: "client:notifications.type.admin_error_group_regressed",
  others: "client:notifications.type.admin_error_group_others",
};

/** The sentence of an `admin_error_group` by what it tells of; null for the other types, which keep their own. */
function errorGroupSentenceOf(notification: Notification): ClientMessageKey | null {
  const change = notification.type === "admin_error_group" ? notification.error_group?.change : undefined;
  return change === undefined ? null : (errorGroupSentences[change] ?? null);
}

/**
 * What a row shows of the supporter subscription that a supporter notification tells of (ADR-0072 §16), as it is
 * now: the plan's name (the administrators' text), and for a failed payment that is still overdue, when the grace
 * period ends; for an end, why; for a retired plan, what happens to the subscription. Null when it is gone.
 */
function supporterOf(notification: Notification, formatDate: (value: string) => string): SupporterNoticeView | null {
  const subscription = notification.supporter_subscription;
  if (subscription === undefined) return null;
  const plan = subscription.plan_name;
  switch (notification.type) {
    case "supporter_payment_failed": {
      const ends = subscription.state === "past_due" ? subscription.grace_ends_at : undefined;
      return {
        plan,
        detail: ends === undefined ? null : { kind: "grace", date: { dateTime: ends, text: formatDate(ends) } },
      };
    }
    case "supporter_subscription_ended":
      return {
        plan,
        detail: subscription.end_reason === undefined ? null : { kind: "ended", reason: subscription.end_reason },
      };
    case "supporter_plan_retired":
      return { plan, detail: { kind: "retired" } };
    case "supporter_service_closure":
      return { plan, detail: null };
    default:
      return null;
  }
}

/** The sentences of what became of the user's follow when an account they followed moved. */
const moveOutcomeSentences: Readonly<
  Record<NonNullable<NonNullable<Notification["account_move"]>["outcome"]>, MoveOutcomeKey>
> = {
  followed: "client:notifications.account_moved.followed",
  requested: "client:notifications.account_moved.requested",
  blocking: "client:notifications.account_moved.blocking",
  blocked_by: "client:notifications.account_moved.blocked_by",
  domain_muted: "client:notifications.account_moved.domain_muted",
};

/** The new account of an `account_moved` and what became of the user's follow; null without the account. */
function moveOf(notification: Notification): MoveNoticeView | null {
  const move = notification.account_move;
  if (notification.type !== "account_moved" || move?.target === undefined) return null;
  return {
    handle: handleOf(move.target.acct),
    href: profilePath(move.target.acct),
    outcome: move.outcome === undefined ? null : moveOutcomeSentences[move.outcome],
  };
}

/** The sentences of the custom emoji requests, by what happened to the request and its kind. */
const emojiRequestSentences = {
  submitted: {
    create: "client:notifications.custom_emoji_request.submitted.create",
    image: "client:notifications.custom_emoji_request.submitted.image",
  },
  approved: {
    create: "client:notifications.custom_emoji_request.approved.create",
    image: "client:notifications.custom_emoji_request.approved.image",
  },
  rejected: {
    create: "client:notifications.custom_emoji_request.rejected.create",
    image: "client:notifications.custom_emoji_request.rejected.image",
  },
} as const satisfies Record<string, Record<"create" | "image", ClientMessageKey>>;

/**
 * The sentence and the emoji of a custom emoji request's notification; without its request (or for a decision in
 * another state), no sentence, so that the row says the type's sentence without the emoji.
 */
function emojiRequestOf(
  notification: Notification,
): Pick<NotificationView, "emojiRequest"> & { sentence: ClientMessageKey | null } {
  const request = notification.custom_emoji_request;
  if (
    request === undefined ||
    (notification.type !== "admin_custom_emoji_request" && notification.type !== "custom_emoji_request_decided")
  ) {
    return { sentence: null, emojiRequest: null };
  }
  const outcome =
    notification.type === "admin_custom_emoji_request"
      ? "submitted"
      : request.state === "approved" || request.state === "rejected"
        ? request.state
        : null;
  if (outcome === null) return { sentence: null, emojiRequest: null };
  const reason =
    request.state === "rejected" && request.reason !== undefined && request.reason !== "" ? request.reason : null;
  return {
    sentence: emojiRequestSentences[outcome][request.kind],
    emojiRequest: { emoji: `:${request.shortcode}:`, reason },
  };
}

/** The sentences of a group of the types that rows group (groupNotifications). */
const groupSentences: Readonly<Partial<Record<NotificationType, ClientMessageKey>>> = {
  boost: "client:notifications.group.boost",
  favourite: "client:notifications.group.favourite",
};

function actorOf(account: NotificationAccount): NotificationActor {
  return {
    id: account.id,
    name: account.display_name || account.username,
    emojis: account.emojis,
    href: profilePath(account.acct),
    avatar: imageSources(account.avatar),
  };
}

function postOf(status: NotificationStatus): NonNullable<NotificationView["post"]> {
  const contentWarning = status.spoiler_text === "" ? null : status.spoiler_text;
  return {
    href: statusPath(status),
    // Behind a content warning, the text stays on the post's page. Status.content is the server-sanitized
    // HTML of the post (api/openapi.yaml), parsed by @my/ui under the app's Trusted Types.
    text: contentWarning === null ? sanitizedHtmlText(serverSanitizedHtml(status.content)) : "",
    lang: status.language === "und" ? undefined : status.language,
    contentWarning,
  };
}

/**
 * The sentence and the values of a `relationships_severed`: the server or the account that the moderators
 * suspended, with the counts; without its severance, the sentence that says only that follows ended.
 */
function severanceOf(notification: Notification): Pick<NotificationView, "sentence" | "severance"> {
  const severance = notification.relationship_severance;
  const fallback = { sentence: sentences.relationships_severed, severance: null };
  if (severance === undefined) return fallback;
  const counts = { followers: severance.followers_lost, following: severance.following_lost };
  if (severance.kind === "domain_suspension") {
    return {
      sentence: "client:notifications.severed.domain",
      severance: { domain: severance.domain ?? "", ...counts },
    };
  }
  if (notification.account === undefined) return fallback;
  return { sentence: "client:notifications.severed.account", severance: { domain: null, ...counts } };
}

/** The sentences of the decisions on an appeal. */
const decisionSentences: Readonly<Record<"upheld" | "overturned", ClientMessageKey>> = {
  upheld: "client:notifications.appeal.upheld",
  overturned: "client:notifications.appeal.overturned",
};

/**
 * What a row shows of the action that a `moderation_notice` or an `appeal_decided` tells of; `formatDate` formats
 * its end.
 */
function noticeOf(action: ModerationAction, formatDate: (value: string) => string): ModerationNoticeView {
  const { appeal } = action;
  return {
    decision:
      appeal === undefined || appeal.state === "pending"
        ? null
        : {
            sentence: decisionSentences[appeal.state],
            reason:
              appeal.decision_reason === undefined || appeal.decision_reason === "" ? null : appeal.decision_reason,
          },
    kind: action.kind_label,
    category: action.reason_category_label,
    reason: action.public_reason === undefined || action.public_reason === "" ? null : action.public_reason,
    until:
      action.expires_at === undefined ? null : { dateTime: action.expires_at, text: formatDate(action.expires_at) },
  };
}

/** The pages that a row of `notification` links to (NotificationView.links); `ownAcct` is null until the viewer loads. */
function linksOf(notification: Notification, ownAcct: string | null): NotificationLink[] {
  switch (notification.type) {
    case "moderation_notice":
    case "appeal_decided":
      return [{ href: accountStatusPath, label: "client:notifications.link.account_status" }];
    case "admin_appeal":
      return notification.appeal_id === undefined
        ? []
        : [{ href: moderationAppealPath(notification.appeal_id), label: "client:notifications.link.appeal" }];
    case "admin_report":
      return notification.report_id === undefined
        ? []
        : [{ href: moderationReportPath(notification.report_id), label: "client:notifications.link.report" }];
    case "admin_sign_up":
      return [{ href: "/moderation/registrations", label: "client:notifications.link.registrations" }];
    case "admin_webhook_disabled":
      return [{ href: "/admin/webhooks", label: "client:notifications.link.webhooks" }];
    // A new or regressed group leads to its page; the groups beyond the hour's limit, to the list.
    case "admin_error_group": {
      const groupId = notification.error_group?.group_id;
      return groupId === undefined
        ? [{ href: adminErrorsPath, label: "client:notifications.link.errors" }]
        : [{ href: adminErrorGroupPath(groupId), label: "client:notifications.link.error_group" }];
    }
    case "admin_custom_emoji_request":
      return [{ href: "/admin/custom-emojis?tab=requests", label: "client:notifications.link.custom_emoji_requests" }];
    case "custom_emoji_request_decided":
      return [{ href: "/settings/custom-emojis", label: "client:notifications.link.own_custom_emojis" }];
    // The Tauri app shows no way to subscribe or to pay, and no link to the supporter page (ADR-0072 §14).
    case "supporter_payment_failed":
      return isTauriApp()
        ? []
        : [{ href: "/settings/supporter", label: "client:notifications.link.supporter_payment" }];
    case "supporter_subscription_ended":
    case "supporter_plan_retired":
    case "supporter_service_closure":
      return isTauriApp() ? [] : [{ href: "/settings/supporter", label: "client:notifications.link.supporter" }];
    // The page of the room, which says when it ended (ADR-0100 §9).
    case "voice_room_started":
    case "voice_room_invited":
      return notification.voice_room_id === undefined
        ? []
        : [{ href: `/voice/${notification.voice_room_id}`, label: "client:notifications.link.voice_room" }];
    case "relationships_severed":
      return ownAcct === null
        ? []
        : [
            { href: followListPath(ownAcct, "followers"), label: "client:notifications.link.followers" },
            { href: followListPath(ownAcct, "following"), label: "client:notifications.link.following" },
          ];
    default:
      return [];
  }
}

/** The emoji of a `reaction` as its sentence shows it (NotificationView.reaction). */
function reactionOf(notification: Notification): string | null {
  const { reaction } = notification;
  if (notification.type !== "reaction" || reaction === undefined) return null;
  return reaction.shortcode === undefined ? reaction.name : `:${reaction.shortcode}:`;
}

/** Returns what the row of `group` shows: one notification, or the favourites or the boosts of one post. */
export function useNotificationView(group: NotificationGroup): NotificationView {
  const [notification] = group.notifications;
  const intl = useIntl();
  const viewer = useViewer();
  const time = useTimeView(notification.created_at);
  const { account, status } = notification;
  const severed = notification.type === "relationships_severed" ? severanceOf(notification) : null;
  const requested = emojiRequestOf(notification);
  // The same account can favourite a post again after taking it back: it counts once.
  const seen = new Set(account === undefined ? [] : [account.id]);
  const others: NotificationActor[] = [];
  for (const { account: other } of group.notifications) {
    if (other === undefined || seen.has(other.id)) continue;
    seen.add(other.id);
    others.push(actorOf(other));
  }
  const groupSentence = groupSentences[notification.type];
  const target = notification.type === "report_resolved" ? notification.report?.target_account : undefined;
  const action =
    notification.type === "moderation_notice" || notification.type === "appeal_decided"
      ? notification.moderation_action
      : undefined;
  const formatDate = (value: string) => intl.formatDate(value, { dateStyle: "medium", timeStyle: "short" });
  return {
    kind: kinds[notification.type],
    replyTo: notification.type === "mention" && status !== undefined ? status : null,
    sentence:
      severed?.sentence ??
      requested.sentence ??
      closureSentenceOf(notification) ??
      errorGroupSentenceOf(notification) ??
      (target === undefined ? null : "client:notifications.report_resolved.account") ??
      (others.length > 0 && groupSentence !== undefined ? groupSentence : sentences[notification.type]),
    actor: account === undefined ? null : actorOf(account),
    subject: target === undefined ? null : actorOf(target),
    others,
    time,
    post: status === undefined ? null : postOf(status),
    followBack: notification.type === "follow" && account !== undefined ? account : null,
    severance: severed?.severance ?? null,
    notice: action === undefined ? null : noticeOf(action, formatDate),
    reaction: reactionOf(notification),
    emojiRequest: requested.emojiRequest,
    supporter: supporterOf(notification, formatDate),
    move: moveOf(notification),
    links: linksOf(notification, viewer.status === "signed-in" ? viewer.account.acct : null),
  };
}
