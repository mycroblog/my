// SPDX-License-Identifier: AGPL-3.0-only
// The stage of a voice chat room (the room page): the participants as a grid of avatars, the speakers large with their
// role under the name and the listeners small. Each avatar shows its participant's state: a coral ring while they
// talk, a microphone-off badge, a mango raised-hand badge, and a reaction bubble for a moment. The ring and the
// badges are drawn for sighted users and each also carries its text for assistive technology, so the state never
// rests on the color or the shape alone (WCAG 1.4.1). The ring fades in and out, and holds still with reduced motion.
//
// The host and co-hosts operate on a participant from the avatar: with `menu`, the avatar and its name are the
// button of a menu (React Aria's MenuTrigger; WAI-ARIA APG, menu button), with a surface under the pointer and the
// caret of a menu after the name, so that it looks operable.
import type { ReactNode } from "react";
import { Button as RACButton, MenuTrigger } from "react-aria-components";
import { tv } from "tailwind-variants";
import { Avatar } from "./Avatar";
import { MenuIndicatorIcon, MicrophoneOffIcon, RaiseHandIcon } from "./Icons";
import type { ImageSources } from "./Picture";
import { VisuallyHidden } from "./VisuallyHidden";

const stageGrid = tv({
  base: "grid list-none",
  variants: {
    size: {
      // As many columns as fit, each as wide as the avatar with room for a name.
      speaker: "grid-cols-[repeat(auto-fill,minmax(6rem,1fr))] gap-x-3 gap-y-4",
      listener: "grid-cols-[repeat(auto-fill,minmax(4.5rem,1fr))] gap-x-2 gap-y-3",
    },
  },
  defaultVariants: { size: "speaker" },
});

export type StageGridProps = {
  /** The accessible name of the list ("Speakers"), when no heading names it. */
  "aria-label"?: string;
  /** The ID of the heading that names the list. */
  "aria-labelledby"?: string;
  /** The size of the avatars in it, which sets the width of the columns. */
  size?: "speaker" | "listener";
  /** The `StageAvatar`s. */
  children: ReactNode;
};

/** A list of `StageAvatar`s, in as many columns as fit the container. */
export function StageGrid({ size, children, ...props }: StageGridProps) {
  return (
    <ul {...props} className={stageGrid({ size })}>
      {children}
    </ul>
  );
}

const stageAvatar = tv({
  slots: {
    item: "flex min-w-0 flex-col items-center",
    // The press target of `menu`: the avatar and the name, as one button. It looks operable: a cream surface under the
    // pointer and while pressed, and the caret of a menu after the name (ADR-0110 P1).
    button: [
      "flex w-full min-w-0 cursor-default flex-col items-center gap-2 rounded-lg p-2 data-focus-visible:focus-ring",
      "data-hovered:bg-soft data-pressed:bg-surface-sunken motion-safe:transition-colors",
    ],
    nameRow: "flex w-full min-w-0 items-center justify-center gap-0.5",
    caret: "shrink-0 text-fg-muted [&>svg]:size-3",
    plain: "flex w-full min-w-0 flex-col items-center gap-2 p-2",
    figure: "relative",
    // The ring is an outline, so the gap between it and the avatar is the ground the avatar is on, whatever that is.
    avatar:
      "outline-3 outline-offset-3 outline-solid motion-safe:transition-[outline-color] motion-safe:duration-normal",
    text: "flex w-full min-w-0 flex-col items-center text-center",
    name: "w-full truncate font-extrabold",
    caption: "w-full truncate text-xs text-fg-muted",
    badge: "absolute grid place-items-center rounded-full [&>svg]:shrink-0",
    muted: "-inset-e-1.5 -bottom-1.5 border border-border bg-surface-raised text-fg-muted",
    hand: "-top-1.5 bg-hand text-hand-fg",
    reaction: "-inset-e-2.5 -top-3 size-8 bg-surface-raised text-base leading-none shadow-md",
  },
  variants: {
    size: {
      speaker: { name: "text-sm", muted: "size-6.5 [&>svg]:size-3.5", hand: "size-6.5 [&>svg]:size-3.5" },
      listener: { name: "text-xs", muted: "size-5.5 [&>svg]:size-3", hand: "size-5.5 [&>svg]:size-3" },
    },
    speaking: {
      true: { avatar: "outline-accent" },
      // Transparent, and none at all in forced colors, where a transparent outline could be drawn in the system color.
      false: { avatar: "outline-transparent forced-colors:outline-none" },
    },
    // The raised hand gives the top end corner to a reaction while it shows.
    withReaction: {
      true: { hand: "-inset-s-1.5" },
      false: { hand: "-inset-e-1.5" },
    },
  },
  defaultVariants: { size: "speaker", speaking: false, withReaction: false },
});

/** The texts of a participant's states, read by assistive technology in place of the ring and the badges. */
export type StageAvatarLabels = {
  /** "Talking" */
  speaking: string;
  /** "Microphone off" */
  muted: string;
  /** "Hand raised" */
  handRaised: string;
};

export type StageAvatarReaction = {
  /** What is drawn in the bubble: an emoji, or a `CustomEmoji`. It is hidden from assistive technology, which reads `label`. */
  content: ReactNode;
  /** What the reaction is, in words ("Reacted with ❤️"). */
  label: string;
};

export type StageAvatarProps = {
  /** The participant's display name, their own text (shown with `dir="auto"`). */
  name: string;
  /** The avatar's image. Without one, the avatar shows the first letter of `name`. */
  src?: string | undefined;
  /** The renditions of the avatar's image, used instead of `src`. */
  sources?: ImageSources | undefined;
  /** What picks the tint of the initial: the account's ID. */
  seed?: string | undefined;
  /** `speaker` (large, default) for the people who speak, `listener` (small) for the others. */
  size?: "speaker" | "listener";
  /** A line under the name: the role ("Host", "Speaker"). */
  caption?: ReactNode;
  /** Whether the participant is talking now: the coral ring. */
  speaking?: boolean;
  /** Whether the participant's microphone is off: the microphone-off badge. */
  muted?: boolean;
  /** Whether the participant has raised a hand to speak: the mango hand badge. */
  handRaised?: boolean;
  /** A reaction the participant sent, shown for a moment in a bubble on the avatar; the app removes it again. */
  reaction?: StageAvatarReaction | undefined;
  /** The texts of the states. */
  labels: StageAvatarLabels;
  /**
   * The operations on the participant, as a `Menu`: the avatar and the name become the button that opens it. The
   * button's accessible name is the participant's name with their states.
   */
  menu?: ReactNode;
};

/** One participant on the stage, as an `li` of a `StageGrid`. */
export function StageAvatar({
  name,
  src,
  sources,
  seed,
  size = "speaker",
  caption,
  speaking = false,
  muted = false,
  handRaised = false,
  reaction,
  labels,
  menu,
}: StageAvatarProps) {
  const slots = stageAvatar({ size, speaking, withReaction: reaction !== undefined });
  const hasMenu = menu !== undefined && menu !== null;
  const content = (
    <>
      <span className={slots.figure()}>
        <Avatar
          src={src}
          sources={sources}
          name={name}
          seed={seed}
          decorative
          size={size === "speaker" ? "xl" : "lg"}
          className={slots.avatar()}
        />
        {muted && (
          <span className={slots.badge({ className: slots.muted() })}>
            <MicrophoneOffIcon />
          </span>
        )}
        {handRaised && (
          <span className={slots.badge({ className: slots.hand() })}>
            <RaiseHandIcon filled />
          </span>
        )}
        {reaction !== undefined && (
          <span className={slots.badge({ className: slots.reaction() })}>
            <span aria-hidden="true">{reaction.content}</span>
          </span>
        )}
      </span>
      <span className={slots.text()}>
        <span className={slots.nameRow()}>
          <span dir="auto" className={slots.name()}>
            {name}
          </span>
          {hasMenu && (
            <span className={slots.caret()}>
              <MenuIndicatorIcon />
            </span>
          )}
        </span>
        {caption !== undefined && caption !== null && <span className={slots.caption()}>{caption}</span>}
        {/* The states in words, in the order they are drawn: the ring, then the badges. */}
        {speaking && <VisuallyHidden>{labels.speaking}</VisuallyHidden>}
        {muted && <VisuallyHidden>{labels.muted}</VisuallyHidden>}
        {handRaised && <VisuallyHidden>{labels.handRaised}</VisuallyHidden>}
        {reaction !== undefined && <VisuallyHidden>{reaction.label}</VisuallyHidden>}
      </span>
    </>
  );
  return (
    <li className={slots.item()}>
      {!hasMenu ? (
        <span className={slots.plain()}>{content}</span>
      ) : (
        <MenuTrigger>
          <RACButton className={slots.button()}>{content}</RACButton>
          {menu}
        </MenuTrigger>
      )}
    </li>
  );
}
