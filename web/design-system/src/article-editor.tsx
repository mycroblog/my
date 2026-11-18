// SPDX-License-Identifier: AGPL-3.0-only

// The parts of the article editor (FR-122, FR-123, ADR-0160): a screen of its own, not the composer. Its head (the
// title, the drafts' menu, close at its end); what is written: the title and the Markdown body in its editor, then
// the article's details (the summary, the cover and the hashtags) under their heading; the article as it will be
// published (the server's rendering), beside what is written from `desktop` and behind Write | Preview under it; and at
// its end the limits, the form's error and the row of the tools and the action, held at the bottom of the viewport as
// the page scrolls. The head, the two columns and the row share one width, centred. Every word is the caller's.

import { use, useRef, useState, type ReactElement, type ReactNode } from "react";
import { ToggleButton, ToggleButtonGroup } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { ArticleText, ArticleTitle } from "./article";
import { Button } from "./button";
import { CaptionLine } from "./entry";
import { describedBy as idsOf, FieldContext } from "./field";
import { FieldError, GlyphLabel } from "./form-page";
import { IconButton } from "./icon-button";
import { Icon } from "./icons";
import { Box, Row, Stack } from "./layout";
import { useReader } from "./locale";

const frame = tv({
  slots: {
    root: "box-border flex min-h-dvh flex-col bg-surface font-sans text-body text-text",
    // The head, the columns and the end row share one width, centred on the page.
    inner: "mx-auto box-border w-full max-w-[calc(var(--shell-column)+2*var(--shell-edge))] px-shell-edge",
    head: "flex items-center justify-between gap-entry-caption pt-entry-line",
    headEnd: "-me-icon-button-inline inline-flex flex-none items-center",
    caption: "hidden min-w-0 items-center gap-entry-caption text-caption text-muted-text",
    // A group's room at its end, so that the last field stands clear of the row's shadow when the page is scrolled to
    // its end, where the row follows it.
    body: "grid flex-1 content-start gap-form-page-form-gap pt-entry pb-form-page-gap",
    main: "grid min-w-0 content-start gap-form-page-form-gap",
    aside: "hidden min-w-0",
    end: "sticky bottom-0 z-3 bg-surface pb-[env(safe-area-inset-bottom)] shadow-bar",
    endInner: "grid gap-entry-line py-entry",
    row: "flex items-center gap-entry-caption",
    tools: [
      "-ms-icon-button-inline flex min-w-0 flex-1 flex-wrap items-center",
      "max-[22.5rem]:[--icon-button-padding-inline:0.25rem]",
    ],
  },
  variants: {
    // From `desktop`, the preview beside what is written, both columns as wide, as wide together as the main column
    // and the one beside it; its caption in the head over it, so that both columns start on one line.
    beside: {
      true: {
        inner: [
          "desktop:max-w-[calc(var(--shell-column)+var(--shell-aside)+var(--shell-gap-wide)+2*var(--shell-edge))]",
        ],
        head: "desktop:grid desktop:grid-cols-2 desktop:gap-shell-gap-wide",
        headEnd: "desktop:flex desktop:justify-between",
        caption: "desktop:flex",
        body: "desktop:grid-cols-2 desktop:gap-shell-gap-wide",
        aside: "desktop:sticky desktop:top-0 desktop:grid desktop:self-start",
      },
      false: {},
    },
  },
});

/**
 * The frame of the article editor's screen, one of the frames of a page: the head (the title, then what ends it), what
 * is written and, from `desktop`, the preview beside it under its caption in the head, both columns as wide; and at
 * its end, held at the bottom of the viewport, what stands over the row, and the row of the tools and the action. The
 * head, the columns and the row share one width, centred.
 */
function EditorFrame({
  dir,
  beside,
  title,
  caption,
  headEnd,
  aside,
  end,
  children,
}: {
  dir: "ltr" | "rtl";
  /** A column stands beside what is written, from `desktop`. */
  beside: boolean;
  title: ReactNode;
  /** What names the column beside what is written, in the head over it, from `desktop`. */
  caption?: ReactNode;
  headEnd: ReactNode;
  /** The column beside what is written, from `desktop`. */
  aside?: ReactNode;
  /** The end of the screen: what stands over its row, and the row's tools and action. */
  end?: { over: ReactNode; tools: ReactNode; action: ReactNode } | undefined;
  children: ReactNode;
}) {
  const s = frame({ beside });
  return (
    <div className={s.root()} dir={dir}>
      <div className={`${s.inner()} ${s.head()}`}>
        {title}
        <div className={s.headEnd()}>
          {beside && <p className={s.caption()}>{caption}</p>}
          <span className="inline-flex items-center">{headEnd}</span>
        </div>
      </div>
      <div className={`${s.inner()} ${s.body()}`}>
        <div className={s.main()}>{children}</div>
        {beside && <div className={s.aside()}>{aside}</div>}
      </div>
      {end !== undefined && (
        <div className={s.end()}>
          <div className={`${s.inner()} ${s.endInner()}`}>
            {end.over}
            <div className={s.row()}>
              <div className={s.tools()}>{end.tools}</div>
              {end.action}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export type ArticleFrameProps = {
  /** New article, Edit article. */
  title: string;
  closeLabel: string;
  onClose: () => void;
  /** What stands in the head before close (the drafts' menu). */
  headEnd?: ReactNode;
  /** The preview, beside what is written from `desktop`, under its caption (As it will be published). */
  preview?: { caption: string; children: ReactNode } | undefined;
  /** The limits of the body, right above the row. */
  limits?: ReactNode;
  /** A line about how it will be published (when it is scheduled for), right above the row. */
  note?: ReactNode;
  /** The form's error, directly before the row that sends it. */
  alert?: ReactNode;
  /** The tools at the row's start; with no tools and no action, there is no row. */
  tools?: ReactNode;
  /** The action that publishes it. */
  action?: ReactNode;
  children: ReactNode;
};

/**
 * The article editor's screen: the head, what is written and, from `desktop`, the preview beside it, and at its end,
 * held at the bottom of the viewport, the limits, the form's error and the row of the tools and the action.
 */
export function ArticleFrame({
  title,
  closeLabel,
  onClose,
  headEnd,
  preview,
  limits,
  note,
  alert,
  tools,
  action,
  children,
}: ArticleFrameProps) {
  const { direction } = useReader(undefined);
  return (
    <EditorFrame
      dir={direction}
      beside={preview !== undefined}
      title={
        // The screen's heading, which takes the focus after a change of screen, as the frame's does.
        <Box as="h1" shrink className="text-title outline-hidden" tabIndex={-1} data-frame-heading="">
          {title}
        </Box>
      }
      caption={
        preview === undefined ? undefined : (
          <>
            <Icon name="about" place="caption" />
            {preview.caption}
          </>
        )
      }
      headEnd={
        <>
          {headEnd}
          <IconButton icon="close" label={closeLabel} onPress={onClose} />
        </>
      }
      aside={preview?.children}
      end={
        tools === undefined && action === undefined
          ? undefined
          : {
              over: (
                <>
                  {limits}
                  {note}
                  {alert}
                </>
              ),
              tools,
              action,
            }
      }
    >
      {children}
    </EditorFrame>
  );
}

/** The row of Write | Preview, as `as`: its choices side by side, under `desktop` alone. */
function ModeRow({ as, children }: { as: ReactElement<{ className?: unknown }>; children: ReactNode }) {
  return (
    <Row as={as} gap="screen-tab-gap" className="desktop:hidden">
      {children}
    </Row>
  );
}

const mode = tv({
  slots: {
    // As the tabs over a list: muted on the page, the current one on the raised surface in the text's color.
    item: [
      "box-border h-control-timeline cursor-pointer rounded-control border-0 bg-transparent",
      "px-control-timeline-inline font-sans text-body font-label whitespace-nowrap text-muted-text outline-hidden",
      "data-hovered:bg-surface-hover data-hovered:text-text data-focus-visible:shadow-focus",
      "data-selected:bg-surface-raised data-selected:text-text",
    ],
  },
});

/**
 * Write or Preview: which of the two the screen shows under `desktop`, as tabs over it read; from `desktop`, where both
 * stand side by side, it is not there.
 */
export function ArticleMode<T extends string>({
  label,
  items,
  current,
  onChange,
}: {
  label: string;
  items: readonly { id: T; name: string }[];
  current: T;
  onChange: (id: T) => void;
}) {
  const s = mode();
  return (
    <ModeRow
      as={
        <ToggleButtonGroup
          aria-label={label}
          selectionMode="single"
          disallowEmptySelection
          selectedKeys={[current]}
          onSelectionChange={(keys) => {
            const id = items.find((item) => keys.has(item.id));
            if (id) onChange(id.id);
          }}
        />
      }
    >
      {items.map((item) => (
        <Row key={item.id} rigid align="center" as={<ToggleButton id={item.id} className={s.item()} />}>
          {item.name}
        </Row>
      ))}
    </ModeRow>
  );
}

const pane = tv({
  slots: { root: "grid min-w-0 content-start gap-form-page-form-gap" },
  variants: {
    shown: {
      always: {},
      desktop: { root: "hidden desktop:grid" },
      narrow: { root: "desktop:hidden" },
      never: { root: "hidden" },
    },
  },
});

/**
 * A part of the screen that shows at some widths: at every width, only from `desktop`, only under it (`narrow`), or
 * at none (the pane that is not the current one).
 */
export function ArticlePane({
  shown = "always",
  children,
}: {
  shown?: "always" | "desktop" | "narrow" | "never" | undefined;
  children: ReactNode;
}) {
  return <div className={pane({ shown }).root()}>{children}</div>;
}

const field = tv({
  slots: {
    label: "font-label",
    hint: "text-caption text-muted-text",
    // The title of the article on the page's surface, at the size its page heads it with (the preview's beside it),
    // clearly over the screen's own title.
    title: [
      "box-border resize-none rounded-control border-0 bg-transparent p-0 font-sans text-headline text-text",
      "outline-hidden [field-sizing:content] placeholder:text-muted-text focus-visible:shadow-focus",
      "aria-invalid:outline-solid aria-invalid:outline-[length:var(--form-page-invalid)]",
      "aria-invalid:outline-offset-[var(--form-page-invalid)] aria-invalid:outline-text",
    ],
    text: [
      "box-border min-h-control-form resize-none rounded-control border-0 bg-surface-raised py-article-field-block",
      "px-control-timeline-inline font-sans text-body text-text outline-hidden [field-sizing:content]",
      "placeholder:text-muted-text focus-visible:shadow-focus",
      "aria-invalid:outline-solid aria-invalid:outline-[length:var(--form-page-invalid)]",
      "aria-invalid:-outline-offset-[var(--form-page-invalid)] aria-invalid:outline-text",
    ],
  },
});

/** A field's parts one under another: its label's line, its hint, its control, its error. */
function FieldStack({
  role,
  "aria-labelledby": labelledBy,
  children,
}: {
  role?: "group" | undefined;
  "aria-labelledby"?: string | undefined;
  children: ReactNode;
}) {
  return (
    <Stack gap="form-page-field-gap" role={role} aria-labelledby={labelledBy}>
      {children}
    </Stack>
  );
}

/** A field's label at the start of its line and what ends the line (a count, a control), on one baseline. */
function LabelLine({ children }: { children: ReactNode }) {
  return (
    <Row align="baseline" justify="between" gap="form-page-alert-gap">
      {children}
    </Row>
  );
}

/** The article's title: its page's headline size, on the page, wrapping, its count at its end once near the limit. */
export function ArticleTitleInput({
  id,
  label,
  placeholder,
  value,
  onChange,
  readOnly = false,
  count,
  error,
}: {
  id: string;
  label: string;
  placeholder: string;
  value: string;
  onChange?: ((value: string) => void) | undefined;
  readOnly?: boolean | undefined;
  count?: ReactNode;
  error?: string | undefined;
}) {
  const s = field();
  return (
    <FieldStack>
      <LabelLine>
        {/* It wraps, as the article's title does; Enter writes no line break in it. */}
        <Box
          grow
          as={
            <textarea
              id={id}
              className={s.title()}
              aria-label={label}
              placeholder={placeholder}
              value={value}
              readOnly={readOnly}
              rows={1}
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? `${id}-error` : undefined}
              dir="auto"
              onChange={(e) => {
                // A line break pasted into it is a space: the title is one line that wraps.
                onChange?.(e.target.value.replace(/\r?\n/g, " "));
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter") e.preventDefault();
              }}
            />
          }
        />
        {count}
      </LabelLine>
      {error && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </FieldStack>
  );
}

/**
 * A labelled field of the article (the summary, the cover, the hashtags): its label, what stands at the label line's
 * end (a count, the cover's remove), its hint, the control, and its error right under it. Its caller describes
 * the control by the hint and the error, by their ids (`<id>-hint`, `<id>-error`).
 */
export function ArticleField({
  id,
  label,
  end,
  hint,
  error,
  children,
}: {
  id: string;
  label: string;
  end?: ReactNode;
  hint?: string | undefined;
  error?: string | undefined;
  children: ReactNode;
}) {
  const s = field();
  return (
    <FieldStack role="group" aria-labelledby={`${id}-label`}>
      <LabelLine>
        <span id={`${id}-label`} className={s.label()}>
          {label}
        </span>
        {end}
      </LabelLine>
      {hint && (
        <p id={`${id}-hint`} className={s.hint()}>
          {hint}
        </p>
      )}
      {children}
      {error && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </FieldStack>
  );
}

/** A plain text of a few lines on a field's surface (the summary), as tall as what it holds. */
export function ArticleTextArea({
  label,
  placeholder,
  value,
  onChange,
  readOnly = false,
  invalid = false,
  describedBy,
}: {
  label: string;
  placeholder: string;
  value: string;
  onChange?: ((value: string) => void) | undefined;
  readOnly?: boolean | undefined;
  invalid?: boolean | undefined;
  /** The IDs of what describes it (its field's hint, `<id>-hint`, and error, `<id>-error`). */
  describedBy?: string | undefined;
}) {
  return (
    <Box
      full
      as={
        <textarea
          className={field().text()}
          aria-label={label}
          placeholder={placeholder}
          value={value}
          readOnly={readOnly}
          onChange={(e) => {
            onChange?.(e.target.value);
          }}
          rows={2}
          aria-invalid={invalid ? true : undefined}
          aria-describedby={describedBy}
          dir="auto"
        />
      }
    />
  );
}

const cover = tv({
  slots: {
    thumb: "size-composer-thumb rounded-control bg-surface-raised object-cover",
    label: "font-label",
  },
});

/**
 * The cover: its thumbnail beside its description, which runs to the row's end; while it uploads, or once it failed,
 * a line under the description says so (`status`), after its spinner or the error's glyph.
 */
export function CoverRow({
  id,
  src,
  description,
  onDescription,
  descriptionLabel,
  placeholder,
  status,
}: {
  id: string;
  /** The image; none while the file has no preview yet. */
  src?: string | undefined;
  description: string;
  onDescription?: ((value: string) => void) | undefined;
  descriptionLabel: string;
  placeholder: string;
  status?: { text: string; busy: boolean } | undefined;
}) {
  const s = cover();
  return (
    <Row align="start" gap="entry-avatar">
      <Box
        rigid
        as={src === undefined ? <span className={s.thumb()} /> : <img className={s.thumb()} src={src} alt="" />}
      />
      <Stack grow gap="entry-line">
        <label className={s.label()} htmlFor={id}>
          {descriptionLabel}
        </label>
        <Box
          full
          as={
            <textarea
              id={id}
              className={field().text()}
              rows={2}
              value={description}
              onChange={(e) => {
                onDescription?.(e.target.value);
              }}
              placeholder={placeholder}
              maxLength={1500}
              dir="auto"
            />
          }
        />
        <div role="status">
          {status !== undefined && (
            <CaptionLine icon="error" busy={status.busy} alert={!status.busy}>
              {status.text}
            </CaptionLine>
          )}
        </div>
      </Stack>
    </Row>
  );
}

/** Adds the cover: a button in words that opens the system's file picker for one image. */
export function CoverAdd({ label, onFile }: { label: string; onFile: (file: File) => void }) {
  const input = useRef<HTMLInputElement>(null);
  return (
    <span>
      <Button
        kind="secondary"
        size="timeline"
        onPress={() => {
          input.current?.click();
        }}
      >
        <GlyphLabel icon="attach">{label}</GlyphLabel>
      </Button>
      <input
        ref={input}
        type="file"
        accept="image/*"
        hidden
        onChange={(e) => {
          const file = e.target.files?.[0];
          e.target.value = "";
          if (file !== undefined) onFile(file);
        }}
      />
    </span>
  );
}

/** The cover's remove, at the end of the cover's label line, in words: what it removes is the cover, not its description. */
export function CoverRemove({ label, onPress = () => undefined }: { label: string; onPress?: () => void }) {
  return (
    <Button kind="quiet" size="timeline" onPress={onPress}>
      <Row as="span" inline align="center" gap="entry-caption">
        <Icon name="delete" place="caption" />
        {label}
      </Row>
    </Button>
  );
}

const tags = tv({
  slots: {
    // On a raised surface (a section of a settings screen), the field takes the page's surface and a hashtag the
    // raised one, as every field does there.
    root: [
      "box-border min-h-control-form rounded-control bg-surface-raised [[data-surface-raised]_&]:bg-surface",
      "py-article-tags-inset ps-article-tags-inset pe-control-timeline-inline",
    ],
    tag: [
      "h-chip rounded-control bg-surface ps-chip-inline text-body text-text whitespace-nowrap",
      "[[data-surface-raised]_&]:bg-surface-raised",
    ],
    // What is typed takes the room left on the line, and goes on a next one where less than its least width is left.
    // Its line, and its placeholder's, which a browser gives a line of its own, is as tall as the field: a field cuts
    // what reaches under a shorter line (the tail of a g).
    input: [
      "h-chip min-w-article-tag-input border-0 bg-transparent p-0 ps-article-tags-inset font-sans text-body text-text",
      "leading-(--chip-height) outline-hidden placeholder:text-muted-text",
    ],
  },
});

/**
 * The hashtags: each on the page's surface with its remove, inside a field's surface that takes the next one. What is
 * typed becomes a hashtag (without its `#`) at Enter, a comma, a space or leaving the field, and Backspace in the empty
 * field takes the last one back into it to change; once `full`, the field takes no more. In a setting's row, a press
 * on the row's name puts the focus on it, and what the row says of it (the line about it, its save state) describes it.
 */
export function TagInput({
  label,
  items,
  removeLabel,
  placeholder,
  full = false,
  onAdd,
  onRemove,
}: {
  label: string;
  items: readonly string[];
  removeLabel: (tag: string) => string;
  placeholder: string;
  full?: boolean | undefined;
  onAdd?: ((tag: string) => void) | undefined;
  onRemove?: ((tag: string) => void) | undefined;
}) {
  const s = tags();
  const holder = use(FieldContext);
  const [typed, setTyped] = useState("");
  const add = (text: string) => {
    const tag = text.trim().replace(/^#+/, "");
    if (tag !== "" && !full) onAdd?.(tag);
    setTyped("");
  };
  return (
    <Row wrap align="center" gap="entry-caption" className={s.root()}>
      {items.map((tag) => (
        <Row key={tag} as="span" inline rigid align="center" gap="hair" className={s.tag()}>
          <bdi>#{tag}</bdi>
          <IconButton
            icon="close"
            label={removeLabel(tag)}
            onPress={() => {
              onRemove?.(tag);
            }}
          />
        </Row>
      ))}
      <Box
        grow
        as={
          <input
            id={holder.id}
            className={s.input()}
            aria-label={label}
            aria-describedby={idsOf(holder.describedBy, holder.stateBy)}
            placeholder={placeholder}
            dir="auto"
            value={typed}
            readOnly={full}
            onChange={(e) => {
              const value = e.target.value;
              // A comma or a space ends the hashtag, as Enter does.
              if (/[,\s]$/u.test(value)) add(value.slice(0, -1));
              else setTyped(value);
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.nativeEvent.isComposing) {
                e.preventDefault();
                add(typed);
              }
              if (e.key === "Backspace" && typed === "" && items.length > 0) {
                const last = items.at(-1) ?? "";
                e.preventDefault();
                onRemove?.(last);
                setTyped(last);
              }
            }}
            onBlur={() => {
              add(typed);
            }}
          />
        }
      />
    </Row>
  );
}

const limits = tv({
  slots: {
    root: "text-caption text-muted-text tabular-nums",
    item: "",
  },
  variants: {
    state: {
      ok: {},
      near: { item: "text-text" },
      over: { item: "font-label text-text" },
    },
  },
});

/**
 * How much of the body's limits is used, before they are hit: each limit as its words say it (12,345 of 100,000
 * bytes), muted until near it, in the text's color near it, and with the error's glyph past it.
 */
export function ArticleLimits({
  items,
}: {
  items: readonly { id: string; text: string; state: "ok" | "near" | "over" }[];
}) {
  return (
    <Row as="p" wrap align="center" gapX="entry-avatar" gapY="0" className={limits().root()}>
      {items.map((item) => (
        <Row
          key={item.id}
          as="span"
          inline
          align="center"
          gap="entry-caption"
          className={limits({ state: item.state }).item()}
        >
          {item.state === "over" && <Icon name="error" place="caption" />}
          {item.text}
        </Row>
      ))}
    </Row>
  );
}

const view = tv({
  slots: {
    // The article as its page sets it: its head (the page's title and summary), its cover, then its body, whose images
    // fill the column as the cover does, a block's air apart.
    root: "text-body",
    // The page's shape of a cover the editor knows no size of: 16:9, never taller than `article-cover-max`.
    cover: "[aspect-ratio:16/9] rounded-control bg-surface-raised-pressed object-cover",
    empty: "text-muted-text",
  },
});

/** The body of the preview: its pictures fill the column, as the cover does. */
function PreviewBody({ children }: { children: ReactNode }) {
  return <div className="[&_img:not(picture>img)]:w-full">{children}</div>;
}

/**
 * The article as it will be published, from the server's rendering: its title, its summary, its cover and its body,
 * whose text is set as the article's page sets it; `empty` stands in for a body not written yet, and `notice` for one
 * the server could not render (a notice with its Try again). While it renders again it is busy, showing what it showed.
 */
export function ArticlePreview({
  title,
  summary,
  cover: coverImage,
  empty,
  lang,
  busy = false,
  notice,
  children,
}: {
  title: string;
  summary?: string | undefined;
  cover?: { src: string; alt: string } | undefined;
  empty?: string | undefined;
  lang?: string | undefined;
  busy?: boolean | undefined;
  notice?: ReactNode;
  children?: ReactNode;
}) {
  const s = view();
  return (
    <Stack
      as="article"
      shrink
      pack="start"
      gap="article-block"
      className={s.root()}
      lang={lang}
      aria-busy={busy || undefined}
    >
      {/* Under the editor's own title, the page's h1. */}
      <ArticleTitle title={title} summary={summary ?? ""} lang={lang} level={2} />
      {coverImage && (
        <Box full max="article-cover" as={<img className={s.cover()} src={coverImage.src} alt={coverImage.alt} />} />
      )}
      {notice ??
        (children === undefined ? (
          <p className={s.empty()}>{empty}</p>
        ) : (
          <PreviewBody>
            <ArticleText lang={lang}>{children}</ArticleText>
          </PreviewBody>
        ))}
    </Stack>
  );
}

// A title's size, over the fields' labels under it.
const group = tv({ slots: { heading: "text-title" } });

/** The article's details after the body, under their heading: the summary, the cover and the hashtags. */
export function ArticleDetails({ id, heading, children }: { id: string; heading: string; children: ReactNode }) {
  const s = group();
  return (
    // After the body, a group's room before it.
    <Stack as="section" gap="form-page-form-gap" spaceAbove="form-page-gap" aria-labelledby={id}>
      <h2 id={id} className={s.heading()}>
        {heading}
      </h2>
      {children}
    </Stack>
  );
}
