// SPDX-License-Identifier: AGPL-3.0-only

// An article (ADR-0160): a post with a title, a summary (never a content warning) and a cover, whose text is long.
// In a timeline it is its card: what it is, its title and its summary, one link to its page, and its cover under them
// at the full width, as a post's media follows its text; while the article is sensitive the cover is covered as
// sensitive media is (media.tsx), lower than the picture it stands for. On its page, its head (the title as the page's
// h1, the summary under it, the cover) and its text, set for long reading: a step larger than a post's, a measure of
// line length, headings, code, quotes, lists, tables and images, with more air between blocks than a post's. A title
// breaks between phrases, not inside a word, and balances its lines.

import { use, useId, useState, type ReactNode } from "react";
import { DisclosureStateContext, Link } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Blurhash } from "./blurhash";
import { Icon } from "./icons";
import { stringsIn, useReader } from "./locale";
import { SensitiveLine, sensitiveLabel } from "./media";

/** An article's cover, with its size, its description and its blurhash. */
export type ArticleCoverImage = {
  src: string;
  width: number;
  height: number;
  alt?: string | undefined;
  blurhash?: string | undefined;
};

export type ArticleCardProps = {
  /** The article's page in the app. */
  href: string;
  title: string;
  summary: string;
  cover?: ArticleCoverImage | undefined;
  /** The article is sensitive: its cover is covered, its blurhash in its place, until the reader shows it. */
  sensitive?: boolean | undefined;
  /** The article's language, when it is known. */
  lang?: string | undefined;
};

/** A title's lines: broken between phrases (Japanese included), never inside a word, and balanced. */
const titleWrap = "[word-break:auto-phrase] text-balance";

const card = tv({
  slots: {
    root: "grid grid-cols-[minmax(0,1fr)] gap-entry",
    link: [
      "group box-border grid min-w-0 gap-entry-line rounded-control text-text no-underline outline-hidden",
      "focus-visible:shadow-focus",
    ],
    covered: "grid grid-cols-[minmax(0,1fr)] gap-media",
    // The line's Show ends at the cover's end edge, its button's padding outside it.
    line: "-me-control-timeline-inline",
    figure: "relative m-0 block w-full max-h-media-card-max overflow-hidden rounded-control bg-surface-raised-pressed",
    image: "block size-full object-cover",
    kind: "flex items-center gap-entry-caption text-caption text-muted-text",
    title: ["line-clamp-3 text-title group-hover:underline group-data-[hovered]:underline", titleWrap],
    summary: "line-clamp-3 text-body text-text",
  },
  variants: {
    // Covered, the cover is a low band of its blurhash, not the picture's whole height.
    covered: { true: { figure: "h-media-preview-height cursor-pointer" } },
  },
});

/** The line that says what an article is, above its title: its glyph and "Article". */
function Kind({ className }: { className: string }) {
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  return (
    <span className={className} lang={lang}>
      <Icon name="article" place="caption" />
      {t("article.kind")}
    </span>
  );
}

/** An article in a timeline: what it is, its title and its summary as one link to its page, and its cover under them. */
export function ArticleCard({ href, title, summary, cover, sensitive = false, lang }: ArticleCardProps) {
  const { lang: reader, direction } = useReader(undefined);
  const { t } = stringsIn(reader);
  const [shown, setShown] = useState(false);
  const id = useId();
  const covered = sensitive && !shown;
  const s = card({ covered });
  // At the full width the cover is never taller than 16:9: a taller one is cropped to it, a wider one keeps its shape.
  const figure = cover !== undefined && (
    <span
      id={id}
      className={s.figure()}
      style={covered ? undefined : { aspectRatio: String(Math.max(cover.width / cover.height, 16 / 9)) }}
      onClick={
        covered
          ? () => {
              setShown(true);
            }
          : undefined
      }
    >
      {covered ? (
        <Blurhash hash={cover.blurhash} className={s.image()} />
      ) : (
        <img className={s.image()} src={cover.src} alt={cover.alt ?? ""} />
      )}
    </span>
  );
  return (
    <div className={s.root()} dir={direction}>
      <Link className={s.link()} href={href}>
        <Kind className={s.kind()} />
        <span className={s.title()} lang={lang} dir="auto">
          {title}
        </span>
        {summary !== "" && (
          <span className={s.summary()} lang={lang} dir="auto">
            {summary}
          </span>
        )}
      </Link>
      {figure !== false &&
        (sensitive ? (
          <div className={s.covered()}>
            <div className={s.line()}>
              <SensitiveLine
                label={sensitiveLabel([{ kind: "image" }], t)}
                shown={shown}
                controls={id}
                onToggle={() => {
                  setShown(!shown);
                }}
              />
            </div>
            {figure}
          </div>
        ) : (
          figure
        ))}
    </div>
  );
}

const head = tv({
  slots: {
    root: "grid grid-cols-[minmax(0,1fr)] gap-entry-line",
    title: ["m-0 text-headline text-text", titleWrap],
    summary: "m-0 text-reading text-muted-text",
    cover:
      "relative m-0 block w-full overflow-hidden rounded-control bg-surface-raised-pressed max-h-article-cover-max",
    image: "block size-full object-cover",
    covered: "grid grid-cols-[minmax(0,1fr)] gap-media",
    line: "-me-control-timeline-inline",
  },
  variants: { covered: { true: { cover: "h-media-preview-height" } } },
});

/**
 * The parts of an article's page (its author's line, its head, its cover, its text), a block's air apart, and a
 * block's air more after its text, before what follows it (its actions). They keep the body's size of a post, whatever
 * size the page gives the post around them (a thread's focused post, a step larger), so that the author's line is a
 * post's.
 */
export function ArticleParts({ children }: { children: ReactNode }) {
  return <div className="grid grid-cols-[minmax(0,1fr)] gap-article-block pb-article-block text-body">{children}</div>;
}

/** The head of an article's page: its title, the page's h1, over its summary. */
export function ArticleTitle({
  title,
  summary,
  lang,
}: {
  title: ReactNode;
  summary: ReactNode;
  lang?: string | undefined;
}) {
  const s = head();
  return (
    <hgroup className={s.root()} lang={lang}>
      <h1 className={s.title()} dir="auto">
        {title}
      </h1>
      {summary !== "" && (
        <p className={s.summary()} dir="auto">
          {summary}
        </p>
      )}
    </hgroup>
  );
}

/**
 * An article's cover on its page, at the full width, never taller than 16:9 nor `article-cover-max`; covered as
 * sensitive media is while the article is sensitive. In a fold (a Disclosure, as a sensitive article's page folds its
 * cover and text), its picture is not asked for until the fold opens: its blurhash stands in its place, at its shape.
 */
export function ArticleCover({
  cover,
  sensitive = false,
  folded = false,
}: {
  cover: ArticleCoverImage;
  sensitive?: boolean | undefined;
  /** The cover stands in a Disclosure, which shows it. */
  folded?: boolean | undefined;
}) {
  const { lang, direction } = useReader(undefined);
  const { t } = stringsIn(lang);
  const [shown, setShown] = useState(false);
  const fold = use(DisclosureStateContext);
  const id = useId();
  const covered = sensitive && !shown;
  const unopened = folded && fold?.isExpanded !== true;
  const s = head({ covered });
  const ratio = Math.max(cover.width / cover.height, 16 / 9);
  const figure = (
    <span id={id} className={s.cover()} style={covered ? undefined : { aspectRatio: String(ratio) }}>
      {covered || unopened ? (
        <Blurhash hash={cover.blurhash} className={s.image()} />
      ) : (
        <img className={s.image()} src={cover.src} alt={cover.alt ?? ""} />
      )}
    </span>
  );
  if (!sensitive) return figure;
  return (
    <div className={s.covered()} dir={direction}>
      <div className={s.line()}>
        <SensitiveLine
          label={sensitiveLabel([{ kind: "image" }], t)}
          shown={shown}
          controls={id}
          onToggle={() => {
            setShown(!shown);
          }}
        />
      </div>
      {figure}
    </div>
  );
}

// The text of an article, for long reading: `reading` type within the measure, a block's air between its blocks and
// a heading's more above a heading, which holds close to what follows it. Each level of heading steps down: h2 and h3
// larger than the text, h4 at its size, h5 at a post's, h6 at a caption's and muted, all bold, broken between phrases.
// The text reads in its own direction, whatever the page's; code always reads left to right. Emphasis in Chinese,
// Japanese or Korean is bold, since those scripts have no italic and a slanted glyph is a distorted one. A post's
// quote (a bar and muted text) and code (the raised surface, wrapped where the renderer marks a break first, never
// scrolling) look the same here. A table scrolls inside its own box when it is wider than the column, its header row
// on the raised surface and its rows apart by a line, where no surface can part them, its cells broken between phrases;
// a column its author aligns (a Markdown table's `---:`, as for amounts) keeps its alignment and never wraps, so that an
// amount keeps its unit and the other columns wrap instead; an image stands at the full width, its own shape kept, with no border; a rule is a
// short line in the middle.
const text = tv({
  base: [
    "grid max-w-article-measure grid-cols-[minmax(0,1fr)] gap-article-block text-reading text-text wrap-anywhere",
    "[&_:is(p,blockquote,ol,ul,pre,h2,h3,h4,h5,h6,table,figure)]:m-0",
    "[&_:is(h2,h3,h4,h5,h6)]:mt-article-heading [&_:is(h2,h3,h4,h5,h6)]:font-strong",
    "[&_:is(h2,h3,h4,h5,h6)]:[word-break:auto-phrase] [&_:is(h2,h3,h4,h5,h6)]:text-balance",
    "[&_:is(h2,h3,h4,h5,h6)+*]:-mt-[calc(var(--article-block-gap)*3/4)]",
    "[&_h2]:text-heading [&_h3]:text-title [&_h4]:text-reading [&_h5]:text-body [&_h6]:text-caption",
    "[&_:is(h4,h5,h6)]:font-strong [&_h6]:text-muted-text",
    "[&_em:is(:lang(ja),:lang(zh),:lang(ko))]:not-italic [&_em:is(:lang(ja),:lang(zh),:lang(ko))]:font-strong",
    "[&_:is(pre,code)]:[direction:ltr] [&_:is(pre,code)]:[unicode-bidi:isolate] [&_pre]:text-start",
    "[&_blockquote]:border-s-content-quote [&_blockquote]:border-s-quote-bar",
    "[&_blockquote]:[border-inline-start-style:solid] [&_blockquote]:ps-content-quote [&_blockquote]:text-muted-text",
    "[&_blockquote]:grid [&_blockquote]:gap-article-block",
    "[&_ol]:list-decimal [&_ol]:ps-content-indent [&_ul]:list-disc [&_ul]:ps-content-indent",
    "[&_li+li]:mt-entry-line",
    "[&_code]:rounded-control [&_code]:bg-surface-raised [&_code]:px-content-code-chip [&_code]:text-[0.875em]",
    "[&_pre]:rounded-control [&_pre]:bg-surface-raised [&_pre]:px-content-code-inline [&_pre]:py-content-code-block",
    "[&_pre]:whitespace-pre-wrap [&_pre]:[text-indent:var(--content-code-hang)_hanging_each-line]",
    "[&_pre]:text-body",
    "[&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_pre_code]:text-[1em]",
    // An image of the text, not a custom emoji's (a picture's), which stays in its line.
    "[&_img:not(picture>img)]:block [&_img:not(picture>img)]:h-auto [&_img:not(picture>img)]:max-w-full",
    "[&_img:not(picture>img)]:rounded-control",
    "[&_hr]:mx-auto [&_hr]:my-0 [&_hr]:w-1/4 [&_hr]:border-0 [&_hr]:border-t-content-quote",
    "[&_hr]:[border-top-style:solid] [&_hr]:border-t-quote-bar",
    "[&_table]:w-full [&_table]:border-collapse [&_table]:text-body [&_:is(th,td)]:[word-break:auto-phrase]",
    "[&_th]:bg-surface-raised [&_th]:font-label [&_:is(th,td)]:px-content-code-inline [&_:is(th,td)]:py-entry-line",
    "[&_:is(th,td)[align=right]]:text-right [&_:is(th,td)[align=center]]:text-center",
    "[&_:is(th,td)[align]]:whitespace-nowrap",
    "[&_:is(th,td):not([align])]:text-start [&_td]:border-0 [&_td]:border-b [&_td]:border-solid [&_td]:border-surface-raised",
  ],
});

/**
 * The text of an article on its page, set for long reading, in its own direction. In a fold (a Disclosure, as a
 * sensitive article's page folds its cover and text), the text is not drawn until the fold opens, so that none of its
 * images is asked for before (a hidden panel's lazy image still loads when the panel is found in the page).
 */
export function ArticleText({
  children,
  lang,
  folded = false,
}: {
  children: ReactNode;
  lang?: string | undefined;
  /** The text stands in a Disclosure, which shows it. */
  folded?: boolean | undefined;
}) {
  const fold = use(DisclosureStateContext);
  return (
    <div className={text()} lang={lang} dir="auto">
      {folded && fold?.isExpanded !== true ? null : children}
    </div>
  );
}

/**
 * A table of an article's text, in a box of its own that scrolls sideways when the table is wider than the text; its
 * columns run in its own direction.
 */
export function ArticleTable({ children, lang }: { children: ReactNode; lang?: string | undefined }) {
  return (
    // A box that scrolls takes the focus, so that a keyboard can scroll it.
    <div className="overflow-x-auto rounded-control outline-hidden focus-visible:shadow-focus" tabIndex={0}>
      <table lang={lang} dir="auto">
        {children}
      </table>
    </div>
  );
}
