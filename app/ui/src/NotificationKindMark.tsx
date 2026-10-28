// SPDX-License-Identifier: AGPL-3.0-only
// The kind of a notification at a glance (web-client/21-experience-principles.md P3): a small round mark with the icon of
// the kind on the corner of the actor's avatar, or in the avatar's place for a row without an actor (a notice of the
// server). The sentence of the row says the kind in words, so the mark is decorative. Each kind has its own shape, and
// its color only repeats it (WCAG 1.4.1).
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";
import {
  BoostIcon,
  DirectVisibilityIcon,
  EmojiIcon,
  FollowRequestIcon,
  ModerationIcon,
  NotificationsIcon,
  PollIcon,
  QuoteIcon,
  StarIcon,
  VoiceIcon,
} from "./Icons";

/** The kinds of notification that have a mark of their own; the rest are `notice`. */
export type NotificationKind =
  | "favourite"
  | "boost"
  | "follow"
  | "mention"
  | "reaction"
  | "quote"
  | "poll"
  | "voice"
  | "staff"
  | "notice";

const kindIcons: Readonly<Record<NotificationKind, () => ReactNode>> = {
  favourite: () => <StarIcon filled />,
  boost: () => <BoostIcon filled />,
  follow: () => <FollowRequestIcon />,
  mention: () => <DirectVisibilityIcon />,
  reaction: () => <EmojiIcon />,
  quote: () => <QuoteIcon />,
  poll: () => <PollIcon filled />,
  voice: () => <VoiceIcon />,
  staff: () => <ModerationIcon />,
  notice: () => <NotificationsIcon />,
};

const kindMark = tv({
  slots: {
    base: "relative",
    mark: "grid place-items-center rounded-full ring-2 ring-surface-raised",
  },
  variants: {
    // On the avatar's corner, or as large as an avatar for a row without one.
    placement: {
      corner: { mark: "absolute -inset-e-1 -bottom-1 size-5 [&>svg]:size-3" },
      alone: { base: "size-10", mark: "size-10 ring-0 [&>svg]:size-5" },
    },
    kind: {
      favourite: { mark: "bg-warning text-warning-fg" },
      boost: { mark: "bg-success text-success-fg" },
      follow: { mark: "bg-accent-fill text-accent-fill-fg" },
      mention: { mark: "bg-accent-fill text-accent-fill-fg" },
      reaction: { mark: "bg-warning text-warning-fg" },
      quote: { mark: "bg-soft text-accent" },
      poll: { mark: "bg-soft text-accent" },
      voice: { mark: "bg-soft text-accent" },
      staff: { mark: "bg-surface-sunken text-fg" },
      notice: { mark: "bg-surface-sunken text-fg" },
    },
  },
});

export type NotificationKindMarkProps = {
  kind: NotificationKind;
  /** The actor's avatar (a `PostCardAvatarLink`); without it the mark stands in the avatar's place. */
  children?: ReactNode;
};

/** The avatar slot of a notification's row: the avatar with the mark of the kind on its corner, or the mark alone. */
export function NotificationKindMark({ kind, children }: NotificationKindMarkProps) {
  const slots = kindMark({ kind, placement: children === undefined ? "alone" : "corner" });
  const Icon = kindIcons[kind];
  return (
    <div className={slots.base()} data-notification-kind={kind}>
      {children}
      <span aria-hidden="true" className={slots.mark()}>
        <Icon />
      </span>
    </div>
  );
}
