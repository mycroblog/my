// SPDX-License-Identifier: AGPL-3.0-only
// An article (a long post in Markdown) as lists show it, and its cover image (ADR-0160 §1, §6): lists carry no body,
// so the card is the article's title, summary and cover, and one link to the article's page.
import { type ComponentPropsWithRef, type ReactNode, useState } from "react";
import { ToggleButton } from "react-aria-components";
import { tv } from "tailwind-variants";
import { Blurhash } from "./Blurhash";
import { HideMediaIcon } from "./Icons";
import { Link } from "./Link";
import { type ImageSources, Picture } from "./Picture";
import { VisuallyHidden } from "./VisuallyHidden";

const articleCover = tv({
  slots: {
    // Positioned, so that the blur and the toggle lie over the image.
    base: "relative aspect-video w-full overflow-hidden bg-surface-sunken",
    // Above the blur, which is positioned: the image covers it as it loads.
    image: "relative size-full object-cover",
    blur: "absolute inset-0",
    // Above the hit area of a card's link, so that it stays its own control.
    toggle: [
      "absolute z-10 cursor-pointer rounded-full bg-surface-raised font-extrabold text-fg select-none data-focus-visible:focus-ring",
      "data-hovered:bg-surface-sunken data-pressed:bg-surface-sunken motion-safe:transition-[background-color,scale] motion-safe:data-pressed:scale-95",
    ],
  },
  variants: {
    rounded: { true: { base: "rounded-lg" } },
    toggle: {
      // Concealed, in the middle of the image with its name.
      label: {
        toggle:
          "inset-0 m-auto size-fit min-h-10 max-w-[calc(100%-1rem)] px-4 py-2 text-sm shadow-md pointer-coarse:min-h-11",
      },
      // Shown, in the corner of the image.
      corner: {
        toggle: "inset-s-1.5 top-1.5 grid size-8 place-items-center shadow-sm pointer-coarse:size-11 [&>svg]:size-5",
      },
    },
  },
});

/** The cover image of an article. */
export type ArticleCoverImage = {
  /** The renditions of the image. */
  sources: ImageSources;
  /** The size of the image in pixels, when known, so that the browser knows its ratio before it loads. */
  width?: number | undefined;
  height?: number | undefined;
  /** The blurhash of the image, drawn while it is concealed or loading. */
  blurhash?: string | undefined;
  /** The author's description of the image, or undefined when it has none: the image is then decorative. */
  alt?: string | undefined;
};

export type ArticleCoverProps = Omit<ComponentPropsWithRef<"div">, "children"> & {
  image: ArticleCoverImage;
  /** The language of the article, for the description of the image. */
  lang?: string | undefined;
  /** Whether the article is sensitive: the image is concealed until the reader shows it. */
  sensitive?: boolean;
  /** The name of the toggle button that shows a sensitive image ("Show sensitive media"); only used when `sensitive`. */
  revealLabel: string;
  /** Rounds the corners, for a cover on its own (on the article's page); a card's cover takes the card's corners. */
  rounded?: boolean;
};

/**
 * The cover image of an article, 16:9, cropped to fill. A sensitive image is concealed behind a toggle button, as in
 * MediaGrid and LinkCard: while concealed, the image is not loaded and its blur stands in for it. The description is
 * user content, shown with `dir="auto"` and the article's language (web-client/README.md Y10).
 */
export function ArticleCover({
  image,
  lang,
  sensitive = false,
  revealLabel,
  rounded = false,
  className,
  ...props
}: ArticleCoverProps) {
  const [shown, setShown] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const revealed = !sensitive || shown;
  const slots = articleCover({ rounded, toggle: revealed ? "corner" : "label" });
  return (
    <div {...props} className={slots.base({ className })}>
      {image.blurhash !== undefined && !(revealed && loaded) && (
        <Blurhash hash={image.blurhash} className={slots.blur()} />
      )}
      {revealed && (
        <Picture
          sources={image.sources}
          // An article is at most about 40rem wide, as a post is.
          sizes="(min-width: 40rem) 40rem, 100vw"
          {...(image.width !== undefined && { width: image.width })}
          {...(image.height !== undefined && { height: image.height })}
          alt={image.alt ?? ""}
          {...(image.alt !== undefined && { dir: "auto", lang })}
          loading="lazy"
          decoding="async"
          onLoad={() => {
            setLoaded(true);
          }}
          className={slots.image()}
        />
      )}
      {sensitive && (
        <ToggleButton isSelected={revealed} onChange={setShown} className={slots.toggle()}>
          {revealed ? (
            <>
              <HideMediaIcon />
              <VisuallyHidden>{revealLabel}</VisuallyHidden>
            </>
          ) : (
            revealLabel
          )}
        </ToggleButton>
      )}
    </div>
  );
}

const articleCard = tv({
  slots: {
    // Positioned, so that the link's stretched hit area covers the whole card.
    base: "relative flex w-full flex-col overflow-hidden rounded-lg border border-border bg-surface-raised",
    cover: "border-b border-border",
    text: "flex min-w-0 flex-col gap-1 px-3 py-2",
    kind: "text-xs font-bold text-fg-muted",
    // The link's ::after covers the card, so that the whole card is one link without nesting the toggle in it.
    link: [
      "min-w-0 cursor-pointer text-lg font-extrabold wrap-anywhere text-fg no-underline after:absolute after:inset-0 after:rounded-lg",
      "data-hovered:text-fg data-hovered:underline data-focus-visible:underline",
    ],
    summary: "line-clamp-3 min-w-0 text-sm wrap-anywhere text-fg-muted",
  },
});

export type ArticleCardProps = Omit<ComponentPropsWithRef<"div">, "children" | "title"> & {
  /** The article's page, in this app: the card's one link. */
  href: string;
  /** The title (user content); null for an article from another server that has none, which shows `untitledLabel`. */
  title: ReactNode;
  /** What the link says for an article without a title ("Untitled article"); only used when `title` is null. */
  untitledLabel: string;
  /** The summary (user content); nothing is shown when it is undefined. */
  summary?: ReactNode;
  /** The language of the article, for the title, the summary and the description of the cover. */
  lang?: string | undefined;
  /** The cover image. */
  cover?: ArticleCoverImage | undefined;
  /** Whether the article is sensitive: its cover is concealed until the reader shows it. */
  sensitive?: boolean;
  /** The name of the toggle button that shows a sensitive cover; only used when `sensitive` and there is a cover. */
  revealLabel: string;
  /** The line above the title that says what the card is ("Article"). */
  kindLabel: string;
};

/**
 * The card of an article in a list: its cover, the line that says it is an article, its title as the card's one link
 * to the article's page (the title's link stretches over the card), and its summary cut after three lines. A sensitive
 * article shows its title and summary and conceals its cover (ArticleCover); the body is only on its page. The title
 * and the summary are user content, shown with `dir="auto"` and the article's language (web-client/README.md Y10).
 */
export function ArticleCard({
  href,
  title,
  untitledLabel,
  summary,
  lang,
  cover,
  sensitive = false,
  revealLabel,
  kindLabel,
  className,
  ...props
}: ArticleCardProps) {
  const slots = articleCard();
  return (
    <div {...props} className={slots.base({ className })}>
      {cover !== undefined && (
        <ArticleCover
          image={cover}
          lang={lang}
          sensitive={sensitive}
          revealLabel={revealLabel}
          className={slots.cover()}
        />
      )}
      <div className={slots.text()}>
        <span className={slots.kind()}>{kindLabel}</span>
        <Link href={href} className={slots.link()}>
          {title === null ? (
            untitledLabel
          ) : (
            <span dir="auto" lang={lang}>
              {title}
            </span>
          )}
        </Link>
        {summary !== undefined && summary !== null && summary !== "" && (
          <p dir="auto" lang={lang} className={slots.summary()}>
            {summary}
          </p>
        )}
      </div>
    </div>
  );
}
