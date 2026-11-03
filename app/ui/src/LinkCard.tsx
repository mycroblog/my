// SPDX-License-Identifier: AGPL-3.0-only
import { type ComponentPropsWithRef, type ReactNode, useState } from "react";
import { ToggleButton } from "react-aria-components";
import { tv } from "tailwind-variants";
import { Blurhash } from "./Blurhash";
import { HideMediaIcon } from "./Icons";
import { ExternalLink } from "./ExternalLink";
import { VisuallyHidden } from "./VisuallyHidden";

const linkCard = tv({
  slots: {
    // Positioned, so that the link's stretched hit area covers the whole card.
    base: "relative flex w-full overflow-hidden rounded-lg border border-border bg-surface-raised",
    media: "relative overflow-hidden bg-surface-sunken",
    // Above the blur, which is positioned: the image covers it as it loads.
    image: "relative size-full object-cover",
    blur: "absolute inset-0",
    text: "flex min-w-0 flex-1 flex-col gap-0.5 px-3 py-2",
    site: "min-w-0 truncate text-xs font-bold text-fg-muted",
    // The link's ::after covers the card, so that the whole card is one link without nesting the toggle in it.
    link: [
      "min-w-0 font-extrabold wrap-anywhere text-fg no-underline after:absolute after:inset-0 after:rounded-lg",
      "data-hovered:text-fg data-hovered:underline data-focus-visible:underline",
    ],
    description: "line-clamp-2 min-w-0 text-sm wrap-anywhere text-fg-muted",
    byline: "min-w-0 text-xs wrap-anywhere text-fg-muted",
    // Above the link's hit area, so that it stays its own control.
    toggle:
      "absolute z-10 cursor-default rounded-full bg-surface-raised font-extrabold text-fg select-none data-focus-visible:focus-ring data-hovered:bg-surface-sunken motion-safe:transition-colors",
  },
  variants: {
    size: {
      large: {
        base: "flex-col",
        media: "aspect-video w-full border-b border-border",
      },
      // The image is square and at the end of the row, on the right in a left-to-right language, whatever the length
      // of the text: the title and the byline are cut short, so that a card stays about the height of its image.
      small: {
        base: "@container flex-row",
        media: "order-last m-2 aspect-square w-20 shrink-0 self-start rounded-md @sm:w-28",
        link: "line-clamp-2",
        byline: "truncate",
      },
    },
    toggle: {
      // Concealed, in the middle of the image with its name; in a small image only the icon fits.
      label: {
        toggle:
          "inset-0 m-auto size-fit min-h-10 max-w-[calc(100%-1rem)] px-4 py-2 text-sm shadow-md pointer-coarse:min-h-11",
      },
      icon: {
        toggle: "inset-0 m-auto grid size-8 place-items-center shadow-md pointer-coarse:size-11 [&>svg]:size-5",
      },
      // Shown, in the corner of the image.
      corner: {
        toggle: "inset-s-1.5 top-1.5 grid size-8 place-items-center shadow-sm pointer-coarse:size-11 [&>svg]:size-5",
      },
    },
  },
});

/** The image of a link card. */
export type LinkCardImage = {
  /** The image URL (the media proxy's small rendition). */
  src: string;
  /** The size of the image in pixels, so that the browser reserves its room. */
  width: number;
  height: number;
  /** The blurhash of the image, drawn while it is concealed or loading. */
  blurhash?: string | undefined;
  /** The page's description of the image, or undefined when it has none: the image is then decorative. */
  alt?: string | undefined;
};

/** How a link card is laid out: the image above the text, or a square image beside it. */
export type LinkCardSize = "large" | "small";

export type LinkCardProps = Omit<ComponentPropsWithRef<"div">, "children" | "title"> & {
  /** The page the card links to. It opens in a new tab, without a referrer. */
  href: string;
  /** The layout of the card. */
  size?: LinkCardSize;
  /** The title of the page. */
  title: string;
  /** The description of the page; nothing is shown when it is empty. */
  description?: string | undefined;
  /** The name of the site (its host when the page names none). */
  siteName: string;
  /** The language of the page, for the title, description and site name. */
  lang?: string | undefined;
  /** The image of the page. */
  image?: LinkCardImage | undefined;
  /** One line under the description: who wrote the page and when. */
  byline?: ReactNode;
  /** Whether the image is sensitive: it is concealed until the reader shows it. */
  sensitive?: boolean;
  /** The name of the toggle button that shows a sensitive image ("Show sensitive media"); only used when `sensitive`. */
  revealLabel: string;
  /** Whether a sensitive image is shown at first. */
  defaultRevealed?: boolean;
};

/**
 * The card of a link in a post: the image, site name, title, description and byline of the linked page. The whole
 * card is one link to the page (the title's link stretches over the card), an ExternalLink that opens in a new tab
 * with `rel="noopener noreferrer nofollow"`. The page's texts are user content, shown with `dir="auto"` and the
 * page's language (web-client/README.md Y10). A sensitive image is concealed behind a toggle button above the link's area, as in
 * MediaGrid: while concealed, the image is not loaded and its blur stands in for it. The small layout puts a square
 * image at the end of the row and cuts long texts short; without an image it is the same text alone.
 */
export function LinkCard({
  href,
  size = "large",
  title,
  description,
  siteName,
  lang,
  image,
  byline,
  sensitive = false,
  revealLabel,
  defaultRevealed = false,
  className,
  ...props
}: LinkCardProps) {
  const [shown, setShown] = useState(defaultRevealed);
  const [loaded, setLoaded] = useState(false);
  const revealed = !sensitive || shown;
  const slots = linkCard({ size, toggle: revealed ? "corner" : size === "small" ? "icon" : "label" });
  return (
    <div {...props} data-size={size} className={slots.base({ className })}>
      {image !== undefined && (
        <div className={slots.media()}>
          {image.blurhash !== undefined && !(revealed && loaded) && (
            <Blurhash hash={image.blurhash} className={slots.blur()} />
          )}
          {revealed && (
            <img
              src={image.src}
              width={image.width}
              height={image.height}
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
              {revealed || size === "small" ? (
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
      )}
      <div className={slots.text()}>
        <span dir="auto" lang={lang} className={slots.site()}>
          {siteName}
        </span>
        <ExternalLink href={href} rel="nofollow" icon={false} className={slots.link()}>
          <span dir="auto" lang={lang}>
            {title}
          </span>
        </ExternalLink>
        {description !== undefined && description !== "" && (
          <p dir="auto" lang={lang} className={slots.description()}>
            {description}
          </p>
        )}
        {byline !== undefined && byline !== null && <p className={slots.byline()}>{byline}</p>}
      </div>
    </div>
  );
}
