// SPDX-License-Identifier: AGPL-3.0-only
// The heading of a profile (FR-204): the header image, the overlapping avatar beside the relationship actions, the name
// and handle, the profile text, its metadata fields, when the account joined and its counts. The header is its own size
// container, so the fields line up in rows once it is wide (web-client/README.md D2). Strings and content come in as props.
import { type ReactNode, useState } from "react";
import { tv } from "tailwind-variants";
import { useAnimationScope } from "./AnimationScope";
import { Avatar } from "./Avatar";
import { AnimationsPlayContext, type ImageSources, Picture } from "./Picture";
import { Tag } from "./Tag";

const noTags: readonly string[] = [];
const noCounts: readonly ProfileHeaderCount[] = [];

const profileHeader = tv({
  slots: {
    root: "@container flex flex-col",
    // The avatar overlaps the header image; the actions start below it, a gap apart, rather than touching it.
    top: "flex flex-wrap items-start justify-between gap-3 px-4",
    actions: "pt-3",
    avatar: "-mt-10 ring-4 ring-surface",
    body: "mt-3 flex flex-col gap-3 px-4",
    identity: "min-w-0 wrap-anywhere",
    handle: "text-fg-muted",
    tags: "flex flex-wrap gap-2",
    joined: "text-sm text-fg-muted",
    counts: "flex flex-wrap gap-x-4 gap-y-1",
  },
});

export type ProfileHeaderCount = {
  /** Identifies the count among its siblings. */
  key: string;
  /** Usually a link to the list the count stands for. */
  content: ReactNode;
};

export type ProfileHeaderProps = {
  /** The header image. A plain band shows when it is missing or fails to load. */
  image?: ImageSources | undefined;
  /** The account's picture; decorative, as the name beside it says whose page this is. */
  avatar?: ImageSources | undefined;
  /** What picks the tint of the avatar's letter, such as the account's ID (see `Avatar`). */
  avatarSeed?: string | undefined;
  /** The account's name, for the avatar's fallback letter. */
  name: string;
  /** The relationship buttons, at the end of the avatar's row. */
  actions?: ReactNode;
  /** The page heading around the name (the route's level-one heading). */
  heading: ReactNode;
  /** The handle (`@name@domain`), or a server's domain; it reads left to right whatever the page's direction. */
  handle?: string;
  /** The role badges. */
  badges?: ReactNode;
  /** Short words about the account or the relationship ("Follows you"), shown as pills. */
  tags?: readonly string[];
  /** The profile text. */
  note?: ReactNode;
  /** A `ProfileFields`. */
  fields?: ReactNode;
  /** The text saying when the account joined. */
  joined?: ReactNode;
  /** The counts, side by side. */
  counts?: readonly ProfileHeaderCount[];
};

/**
 * The profile's `<header>`. It is an animation scope (`useAnimationScope`): while animations do not play by themselves, its
 * avatar, header image and emoji move while it is hovered or has keyboard focus inside, and from a tap on it until another
 * tap or until it scrolls off the screen.
 */
export function ProfileHeader({
  image,
  avatar,
  avatarSeed,
  name,
  actions,
  heading,
  handle,
  badges,
  tags = noTags,
  note,
  fields,
  joined,
  counts = noCounts,
}: ProfileHeaderProps) {
  const styles = profileHeader();
  const scope = useAnimationScope<HTMLElement>();
  return (
    <header {...scope.props} className={styles.root()}>
      <AnimationsPlayContext value={scope.playing}>
        <ProfileHeaderImage sources={image} size="banner" />
        <div className={styles.top()}>
          <Avatar sources={avatar} name={name} seed={avatarSeed} decorative size="xl" className={styles.avatar()} />
          {actions !== undefined && <div className={styles.actions()}>{actions}</div>}
        </div>
        <div className={styles.body()}>
          <div className={styles.identity()}>
            {heading}
            {/* The handle reads left to right, and lines up with the page's start in either direction. */}
            {handle !== undefined && (
              <p className={styles.handle()}>
                <span dir="ltr">{handle}</span>
              </p>
            )}
          </div>
          {badges}
          {tags.length > 0 && (
            <ul className={styles.tags()}>
              {tags.map((tag) => (
                <Tag key={tag} as="li" variant="pill">
                  {tag}
                </Tag>
              ))}
            </ul>
          )}
          {note}
          {fields}
          {joined !== undefined && <p className={styles.joined()}>{joined}</p>}
          {counts.length > 0 && (
            <ul className={styles.counts()}>
              {counts.map((count) => (
                <li key={count.key}>{count.content}</li>
              ))}
            </ul>
          )}
        </div>
      </AnimationsPlayContext>
    </header>
  );
}

const headerImage = tv({
  slots: {
    band: "rounded-xl bg-surface-sunken",
    picture: "w-full rounded-xl bg-surface-sunken object-cover",
  },
  variants: {
    size: {
      /** The profile's banner, taller in a wide container. */
      banner: { band: "h-24 @md:h-36", picture: "h-24 @md:h-36" },
      /** A preview in a form, in the 3:1 ratio of the header. */
      preview: { band: "aspect-3/1 rounded-md", picture: "aspect-3/1 rounded-md" },
    },
  },
  defaultVariants: { size: "banner" },
});

export type ProfileHeaderImageProps = {
  /** The renditions of the image. */
  sources: ImageSources | undefined;
  /** `banner` is the profile's (inside a size container); `preview` is a 3:1 image for a form. */
  size?: "banner" | "preview";
};

/**
 * An account's header image, or a plain band when it has none or the image fails to load. Either is
 * decorative (`alt=""`, ADR-0027): the name and the profile below it say whose page this is.
 */
export function ProfileHeaderImage({ sources, size }: ProfileHeaderImageProps) {
  // The URL that failed, not a flag: a new src is tried again without resetting state in an Effect.
  const [failedSrc, setFailedSrc] = useState<string>();
  const styles = headerImage({ size });
  const src = sources?.still[0]?.url;
  if (sources === undefined || src === undefined || src === "" || src === failedSrc) {
    return <div aria-hidden="true" className={styles.band()} />;
  }
  return (
    <Picture
      sources={sources}
      // The page's column is at most about 40rem wide.
      sizes="(min-width: 40rem) 40rem, 100vw"
      alt=""
      decoding="async"
      className={styles.picture()}
      onError={() => {
        setFailedSrc(src);
      }}
    />
  );
}

const profileFields = tv({
  slots: {
    list: "flex flex-col divide-y divide-border rounded-lg border border-border",
    item: "flex flex-col gap-1 px-3 py-2 @md:flex-row @md:gap-4",
    term: "text-sm font-semibold wrap-anywhere text-fg-muted align-match-parent @md:w-1/3 @md:shrink-0",
    value: "min-w-0",
  },
});

/** The metadata fields of a profile: a framed `<dl>` of `ProfileField`s. */
export function ProfileFields({ children }: { children: ReactNode }) {
  return <dl className={profileFields().list()}>{children}</dl>;
}

export type ProfileFieldProps = {
  /** The field's name; it follows its own direction. */
  term: ReactNode;
  /** Its value. */
  children: ReactNode;
};

/** One field: its name above its value, or beside it from `@md` of the header. */
export function ProfileField({ term, children }: ProfileFieldProps) {
  const styles = profileFields();
  return (
    <div className={styles.item()}>
      <dt dir="auto" className={styles.term()}>
        {term}
      </dt>
      <dd className={styles.value()}>{children}</dd>
    </div>
  );
}
