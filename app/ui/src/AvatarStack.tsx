// SPDX-License-Identifier: AGPL-3.0-only
// A few avatars that overlap along the line, the first on top: the speakers of a voice chat room on its card and in
// the mini-player of the call. The stack only decorates (it is hidden from assistive technology), so the text beside
// it says what it shows. Each avatar has a ring of the surface it sits on (`surface-raised`, the card's), which
// parts it from the one under it; one that is talking also has the coral ring.
import { tv } from "tailwind-variants";
import { Avatar } from "./Avatar";
import type { ImageSources } from "./Picture";

const avatarStack = tv({
  slots: {
    base: "flex shrink-0 items-center",
    avatar: "relative ring-2 ring-surface-raised not-first:-ms-2",
  },
  variants: {
    speaking: {
      true: { avatar: "outline-2 outline-offset-2 outline-accent outline-solid" },
    },
  },
});

/** One avatar of the stack. */
export type AvatarStackItem = {
  /** What keeps the avatar's place in the stack: the account's ID. Also picks the tint of its initial. */
  id: string;
  /** The display name: the source of the fallback letter. */
  name: string;
  src?: string | undefined;
  sources?: ImageSources | undefined;
  /** Whether the person is talking now: the coral ring. The text beside the stack says it in words. */
  speaking?: boolean;
};

export type AvatarStackProps = {
  /** The avatars, the first on top. */
  avatars: readonly AvatarStackItem[];
  /** The most avatars it shows (default 3). */
  max?: number;
  /** The size of each avatar (default `sm`). */
  size?: "xs" | "sm";
};

/** The overlapping avatars; nothing when `avatars` is empty. */
export function AvatarStack({ avatars, max = 3, size = "sm" }: AvatarStackProps) {
  const slots = avatarStack();
  const shown = avatars.slice(0, max);
  if (shown.length === 0) return null;
  return (
    <span aria-hidden="true" className={slots.base()}>
      {shown.map((avatar, index) => (
        <Avatar
          key={avatar.id}
          name={avatar.name}
          seed={avatar.id}
          src={avatar.src}
          sources={avatar.sources}
          decorative
          size={size}
          // The first is on top, so each overlaps the next one along the line.
          style={{ zIndex: shown.length - index }}
          className={slots.avatar({ speaking: avatar.speaking === true })}
        />
      ))}
    </span>
  );
}
