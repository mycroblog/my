// SPDX-License-Identifier: AGPL-3.0-only

// A page of its own outside the app's frame (a sign-in, a registration, a consent), and the parts of its forms. The
// page is one column with no card on the page's surface: on a phone and a tablet, the server's mark at its start, the
// screen, and the footer; from `desktop`, the server's introduction (its banner, its mark and its description) on a
// raised panel of 5/12 at the start, which stays in view while a long form scrolls, and the column beside it without
// the mark, so that the reader sees which server they are on. With no banner, the introduction sits in the panel's
// middle. The server's front page shows its introduction at every width, at the top of the column on a phone and a
// tablet, with the server's name as the page's title and what leads on from it (its about page) at its end. Fields
// are raised surfaces; what went wrong takes a glyph and the text's color, never a hue, since the accent
// means the primary action and links only. A form's own error sits right before the button that sent it, where the
// reader is when they send it again; a field's sits under the field. Like the app's frame (shell.tsx), it reads the
// viewport, and it carries the page's surface onto <html>.

import { use, type ComponentProps, type ReactElement, type ReactNode, type Ref } from "react";
import { tv } from "tailwind-variants/lite";
import { Avatar } from "./avatar";
import { describedBy as idsOf, FieldContext } from "./field";
import { Icon, type IconName } from "./icons";
import { Box, Column, Row, Stack, type Gap } from "./layout";

/** The server a page belongs to: its name, its own logo for light and dark (else the built-in wordmark), and its introduction. */
export type FormPageServer = {
  name: string;
  logo?: string | undefined;
  logoDark?: string | undefined;
  banner?: string | undefined;
  description?: string | undefined;
};

const page = tv({
  slots: {
    root: [
      "box-border flex min-h-dvh justify-center bg-surface px-shell-edge pt-form-page-top pb-form-page-bottom font-sans",
      "text-body text-text [body:has(&)]:m-0 [html:has(&)]:bg-surface tablet:pt-form-page-top-wide",
      "desktop:grid desktop:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] desktop:p-0",
    ],
    // From `desktop`, the panel stays in view while the column scrolls.
    intro: [
      "hidden desktop:sticky desktop:top-0 desktop:box-border desktop:flex desktop:h-dvh desktop:justify-center",
      "desktop:overflow-y-auto desktop:bg-surface-raised desktop:px-form-page-intro-inline desktop:py-form-page-intro-block",
    ],
    introBody: "flex w-full max-w-[var(--form-page-intro-width)] flex-col gap-form-page-intro-gap",
    banner: "aspect-banner w-full rounded-overlay object-cover",
    summary: "grid gap-form-page-head-gap",
    column: [
      "flex w-full max-w-[var(--form-page-column)] flex-col gap-form-page-gap",
      "desktop:box-border desktop:max-w-[var(--form-page-column-wide)] desktop:justify-self-center",
      "desktop:px-shell-edge desktop:pt-form-page-top-wide desktop:pb-form-page-bottom",
    ],
    header: "desktop:hidden",
    main: "flex flex-1 flex-col gap-form-page-screen-gap",
  },
  variants: {
    banner: { false: { intro: "desktop:items-center" }, true: { intro: "desktop:items-start" } },
    // With no server to introduce, the column stands alone in the page's middle at every width.
    alone: { true: { root: "desktop:flex desktop:px-shell-edge", column: "desktop:px-0" } },
    // The front page: the introduction heads the column below `desktop`, and the column fills the rest of the page.
    front: {
      true: {
        root: "max-desktop:flex-col max-desktop:items-center max-desktop:gap-form-page-gap",
        intro: "max-desktop:flex max-desktop:w-full max-desktop:max-w-[var(--form-page-column)]",
        column: "flex-1",
      },
    },
  },
});

const footer = tv({
  slots: {
    root: "text-caption text-muted-text",
    list: "list-none p-0",
    // A link's target reaches the minimum around its text without moving the line (WCAG 2.5.8).
    link: "text-muted-text py-target-em",
  },
});

const mark = tv({
  slots: {
    wordmark: "",
    logo: "rounded-control object-contain",
    name: "text-muted-text",
  },
  variants: {
    size: {
      line: { wordmark: "h-form-page-mark", logo: "size-form-page-logo" },
      intro: { wordmark: "h-form-page-mark-intro", logo: "size-form-page-logo-intro", name: "text-title" },
    },
    own: { true: { name: "text-text" } },
    // The name is the page's title, at the title's weight with any logo.
    title: { true: { name: "text-text" } },
  },
  compoundVariants: [{ own: true, title: false, class: { name: "font-label" } }],
});

/**
 * The server's mark: its own logo (for light and dark) with its name, else the built-in wordmark with its name; `title`
 * makes the name the page's h1.
 */
function Mark({
  server,
  wordmark,
  size,
  title = false,
}: {
  server: FormPageServer;
  wordmark: { light: string; dark: string };
  size: "line" | "intro";
  title?: boolean;
}) {
  const own = server.logo !== undefined && server.logo !== "";
  const s = mark({ size, own, title });
  const Name = title ? "h1" : "span";
  return (
    <Row align="center" gap="form-page-mark-gap">
      {own ? (
        server.logoDark ? (
          // A logo drawn for each colour scheme: the one of the scheme shows.
          <>
            <Box only="light" as={<img className={`${s.logo()} ${s.wordmark()}`} src={server.logo} alt="" />} />
            <Box only="dark" as={<img className={`${s.logo()} ${s.wordmark()}`} src={server.logoDark} alt="" />} />
          </>
        ) : (
          <img className={s.logo()} src={server.logo} alt="" />
        )
      ) : (
        <>
          <Box only="light" as={<img className={s.wordmark()} src={wordmark.light} alt="" />} />
          <Box only="dark" as={<img className={s.wordmark()} src={wordmark.dark} alt="" />} />
        </>
      )}
      <Name className={s.name()} dir="auto">
        {server.name}
      </Name>
    </Row>
  );
}

export type FormPageProps = {
  /**
   * The server the page belongs to. None for a page the server shows while it cannot read itself (its maintenance):
   * the page is then the one column alone, with no mark and no introduction.
   */
  server?: FormPageServer | undefined;
  /** The built-in wordmark, for light and dark, which a server without a logo of its own shows. */
  wordmark: { light: string; dark: string };
  /** The footer (FormPageFooter). */
  footer?: ReactNode;
  /**
   * The server's front page: its introduction shows at every width and names the page, and `more` ends it (a link to
   * the server's about page).
   */
  front?: { more?: ReactNode } | undefined;
  children: ReactNode;
};

/** A page of its own outside the app's frame: the server's mark or introduction, the screen, and the footer. */
export function FormPage({ server, wordmark, footer, front, children }: FormPageProps) {
  const banner = server?.banner !== undefined && server.banner !== "";
  const isFront = server !== undefined && front !== undefined;
  const s = page({ banner, alone: server === undefined, front: isFront });
  const intro = server !== undefined && (
    <div className={s.introBody()}>
      {banner && <img className={s.banner()} src={server.banner} alt="" />}
      <Mark server={server} wordmark={wordmark} size="intro" title={isFront} />
      {isFront ? (
        // What leads on from the summary stays with it.
        <div className={s.summary()}>
          {server.description && <p dir="auto">{server.description}</p>}
          {front.more}
        </div>
      ) : (
        server.description && <p dir="auto">{server.description}</p>
      )}
    </div>
  );
  return (
    <div className={s.root()}>
      {server !== undefined &&
        (isFront ? (
          <header className={s.intro()}>{intro}</header>
        ) : (
          <aside className={s.intro()} aria-label={server.name}>
            {intro}
          </aside>
        ))}
      <div className={s.column()}>
        {server !== undefined && !isFront && (
          <header className={s.header()}>
            <Mark server={server} wordmark={wordmark} size="line" />
          </header>
        )}
        <main className={s.main()}>{children}</main>
        {footer}
      </div>
    </div>
  );
}

/** The footer of a page: links to the server's documents and its status page, under a name for assistive technology. */
export function FormPageFooter({ label, links }: { label: string; links: { href: string; label: string }[] }) {
  const s = footer();
  if (links.length === 0) return null;
  return (
    <footer className={s.root()}>
      <nav aria-label={label}>
        {/* Wrapped rows stand far enough apart that the targets of two rows never overlap. */}
        <Row as="ul" wrap gapX="form-page-gap" gapY="form-page-footer-rows" className={s.list()}>
          {links.map((link) => (
            <li key={link.href}>
              <a className={s.link()} href={link.href}>
                {link.label}
              </a>
            </li>
          ))}
        </Row>
      </nav>
    </footer>
  );
}

const form = tv({
  slots: {
    title: "text-title",
    // Wrapping between phrases, never inside a word of ja, as a field's error does.
    hint: "text-caption text-muted-text [word-break:auto-phrase]",
    label: "font-label",
    input: [
      "box-border rounded-control border-0 bg-surface-raised px-control-timeline-inline font-sans",
      "text-body text-text outline-hidden focus-visible:shadow-focus read-only:text-muted-text",
      // On a raised surface (a section of a settings screen), a field takes the page's surface.
      "[[data-surface-raised]_&]:bg-surface",
      "aria-invalid:outline-solid aria-invalid:outline-[length:var(--form-page-invalid)]",
      "aria-invalid:-outline-offset-[var(--form-page-invalid)] aria-invalid:outline-text",
    ],
    fieldError: "font-label [word-break:auto-phrase]",
    checkInput: [
      "size-form-page-check accent-text",
      "aria-invalid:outline-solid aria-invalid:outline-[length:var(--form-page-invalid)]",
      "aria-invalid:outline-offset-[var(--form-page-invalid)] aria-invalid:outline-text",
    ],
    alert: [
      "rounded-overlay bg-text p-form-page-alert-padding",
      "text-surface outline-hidden focus-visible:shadow-focus [word-break:auto-phrase]",
    ],
    othersHeading: "text-caption font-normal text-muted-text",
    link: "text-accent-text",
    sectionHeading: "text-body font-label",
    apartHeading: "text-title",
    list: "ps-form-page-indent",
    badge: "rounded-control bg-surface-raised px-form-page-list-gap text-caption font-normal",
    caution: "",
    widget: "min-h-form-page-widget",
  },
  variants: {
    // A textarea starts at a control's height, and the reader may make it taller.
    multiline: {
      false: { input: "h-control-form" },
      true: { input: "h-control-form resize-y py-control-timeline-inline" },
    },
    strong: { true: { caution: "font-label" } },
    // A sentence that tells what happened rather than what is wrong (a draft gone): in the text's own weight.
    regular: { true: { alert: "font-normal" }, false: { alert: "font-label" } },
  },
});

/** How far down a glyph stands to sit on the first line of the text after it, by the glyph's size and the text's. */
const NUDGE = {
  error: "[&_svg]:mt-[0.2em]",
  alert: "[&_svg]:mt-[0.0625rem]",
  caution: "[&_svg]:mt-[0.1em]",
} as const;

/**
 * A glyph and the text after it, as `as`: the text wraps beside the glyph, which stays on its first line, nudged down
 * onto it.
 */
function GlyphLine({
  as,
  nudge,
  gap,
  children,
}: {
  as: ReactElement<{ className?: unknown }>;
  nudge: keyof typeof NUDGE;
  gap: Gap;
  children: ReactNode;
}) {
  return (
    <Row as={as} align="start" gap={gap} className={NUDGE[nudge]}>
      {children}
    </Row>
  );
}

/** The title of a screen, its page's one h1, and the lines under it. */
export function FormHead({ title, children }: { title: string; children?: ReactNode }) {
  const s = form();
  return (
    <Stack gap="form-page-head-gap">
      <h1 className={s.title()} dir="auto">
        {title}
      </h1>
      {children}
    </Stack>
  );
}

/**
 * A paragraph of a screen: plain, or `muted` as a hint at the caption's size. `role="status"` makes it the outcome
 * that assistive technology announces (a request sent).
 */
export function FormText({
  muted = false,
  id,
  role,
  children,
}: {
  muted?: boolean;
  id?: string | undefined;
  role?: "status" | undefined;
  children: ReactNode;
}) {
  const s = form();
  return (
    <p className={muted ? s.hint() : undefined} id={id} role={role} dir="auto">
      {children}
    </p>
  );
}

/**
 * A form that posts to `action` with a full page load; `onSubmit` may stop it, and `ref` reaches it to send it from a
 * script (a passkey's answer).
 */
export function Form({
  action,
  onSubmit,
  ref,
  children,
}: {
  action: string;
  onSubmit?: ComponentProps<"form">["onSubmit"];
  ref?: Ref<HTMLFormElement> | undefined;
  children: ReactNode;
}) {
  return (
    <Column gap="form-page-form-gap" as={<form ref={ref} method="post" action={action} onSubmit={onSubmit} />}>
      {children}
    </Column>
  );
}

export type TextFieldProps = {
  id: string;
  name: string;
  label: string;
  /** `search`: a field that finds as it is written (a searchbox). */
  type?: "text" | "email" | "password" | "search" | undefined;
  /** Left out while an error says the same rule. */
  hint?: string | undefined;
  /** The field's error, right under it; the field takes a line of the text's color. */
  error?: string | undefined;
  defaultValue?: string | undefined;
  autoComplete?: string | undefined;
  inputMode?: "text" | "email" | "numeric" | undefined;
  autoCapitalize?: "none" | "characters" | undefined;
  required?: boolean | undefined;
  minLength?: number | undefined;
  maxLength?: number | undefined;
  /** A textarea of a few rows instead of a line. */
  multiline?: boolean | undefined;
  readOnly?: boolean | undefined;
  /** A link at the end of the label's line (Forgot your password?). */
  end?: ReactNode;
  /** Further ids that describe the field (a prompt above the form). */
  describedBy?: string | undefined;
  /** It takes the focus as it mounts (a field the reader just opened: a list's name). */
  autoFocus?: boolean | undefined;
  /** Its value as the reader types it (a search as they type); else the form reads it as it posts. */
  onChange?: ((value: string) => void) | undefined;
};

/**
 * A labelled field on a raised surface: its label and hint above it, its error right under it, both read with it
 * (aria-describedby), as is what holds it says of it (a dialog's refusal of what the field holds). It posts under
 * `name`.
 */
export function TextField({
  id,
  name,
  label,
  type = "text",
  hint,
  error,
  defaultValue,
  autoComplete,
  inputMode,
  autoCapitalize,
  required,
  minLength,
  maxLength,
  multiline = false,
  readOnly,
  end,
  describedBy,
  autoFocus,
  onChange,
}: TextFieldProps) {
  const s = form({ multiline });
  const holder = use(FieldContext);
  const described = idsOf(describedBy, hint && `${id}-hint`, error && `${id}-error`, holder.describedBy);
  const common = {
    id,
    name,
    defaultValue,
    readOnly,
    autoFocus,
    ...(onChange === undefined
      ? {}
      : {
          onChange: (e: { currentTarget: { value: string } }) => {
            onChange(e.currentTarget.value);
          },
        }),
    required,
    maxLength,
    "aria-invalid": error ? true : undefined,
    "aria-describedby": described,
  };
  return (
    <Stack gap="form-page-field-gap">
      <Row align="baseline" justify="between" gap="form-page-alert-gap">
        <label className={s.label()} htmlFor={id}>
          {label}
        </label>
        {end}
      </Row>
      {hint && (
        <p id={`${id}-hint`} className={s.hint()}>
          {hint}
        </p>
      )}
      {multiline ? (
        <Box full as={<textarea {...common} className={s.input()} rows={4} dir="auto" />} />
      ) : (
        <Box
          full
          as={
            <input
              {...common}
              className={s.input()}
              type={type}
              autoComplete={autoComplete}
              inputMode={inputMode}
              autoCapitalize={autoCapitalize}
              minLength={minLength}
              spellCheck={false}
            />
          }
        />
      )}
      {error && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </Stack>
  );
}

/** A field's error, right under what it is about. */
export function FieldError({ id, children }: { id?: string | undefined; children: string }) {
  return (
    <GlyphLine nudge="error" gap="entry-caption" as={<p id={id} className={form().fieldError()} />}>
      <Icon name="error" place="caption" />
      <span dir="auto">{children}</span>
    </GlyphLine>
  );
}

/** A checkbox with its label at its end, its error under it; it posts `value` under `name` when checked. */
export function CheckField({
  id,
  name,
  value,
  label,
  defaultChecked,
  checked,
  onChange,
  required,
  error,
  describedBy,
}: {
  id: string;
  name: string;
  value: string;
  label: string;
  defaultChecked?: boolean | undefined;
  /** Its state, where its owner keeps it (a poll's settings), with `onChange`; else the checkbox keeps its own. */
  checked?: boolean | undefined;
  onChange?: ((checked: boolean) => void) | undefined;
  required?: boolean | undefined;
  error?: string | undefined;
  describedBy?: string | undefined;
}) {
  const s = form();
  const described = [describedBy, error && `${id}-error`].filter((part) => part).join(" ") || undefined;
  return (
    <Stack gap="form-page-field-gap">
      <Row as="label" align="start" gap="form-page-alert-gap">
        <Box
          rigid
          as={
            <input
              className={s.checkInput()}
              id={id}
              name={name}
              value={value}
              type="checkbox"
              defaultChecked={defaultChecked}
              checked={checked}
              onChange={
                onChange &&
                ((e) => {
                  onChange(e.target.checked);
                })
              }
              required={required}
              aria-invalid={error ? true : undefined}
              aria-describedby={described}
            />
          }
        />
        <span>{label}</span>
      </Row>
      {error && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </Stack>
  );
}

/** The attribute of what a page moves the focus to when it opens (FormAlert's `focus`). */
export const focusOnLoadAttribute = "data-focus-on-load";

/**
 * What the server refused of a form, in one short sentence, directly before the button that sent it: knocked out, the
 * text's color as its fill and the page's as its text. It announces itself (role="alert"); a page that loads anew
 * after a refused post moves the focus to the one marked with `focus` (focusOnLoadAttribute), so that the next stop
 * is the button.
 */
export function FormAlert({
  focus = false,
  regular = false,
  action,
  children,
}: {
  focus?: boolean;
  /** In the text's own weight: what happened, which the button after it acts on (a draft gone), rather than what is wrong. */
  regular?: boolean;
  /** What the reader may do about it, after the sentence (a `TextButton`: open the newer draft). */
  action?: ReactNode;
  children: string;
}) {
  return (
    <GlyphLine
      nudge="alert"
      gap="form-page-alert-gap"
      as={
        <p
          className={form({ regular }).alert()}
          role="alert"
          tabIndex={-1}
          {...{ [focusOnLoadAttribute]: focus ? "" : undefined }}
        />
      }
    >
      <Icon name="error" place="action" />
      <span dir="auto">
        {children}
        {action !== undefined && (
          <>
            {" "}
            <span className="whitespace-nowrap">{action}</span>
          </>
        )}
      </span>
    </GlyphLine>
  );
}

/** A column of full-width actions, the primary first, with a form's alert right before them. */
export function FormActions({ children }: { children: ReactNode }) {
  return <Stack gap="form-page-field-gap">{children}</Stack>;
}

/**
 * A pair of a refusal and its confirmation side by side, each half the column: the refusal at the start, the
 * confirmation at the end, so that the row mirrors in right-to-left text.
 */
export function FormChoice({ children }: { children: ReactNode }) {
  return (
    <Row even gap="form-page-field-gap">
      {children}
    </Row>
  );
}

/** Other ways to go on (a passkey, a login provider) under a quiet heading. */
export function FormOthers({ heading, children }: { heading: string; children: ReactNode }) {
  const s = form();
  return (
    <Stack as="section" gap="form-page-field-gap" aria-label={heading}>
      <h2 className={s.othersHeading()}>{heading}</h2>
      <FormActions>{children}</FormActions>
    </Stack>
  );
}

/** The links that lead off the screen, at its end. */
export function FormLinks({ children }: { children: ReactNode }) {
  return (
    <Row as="p" wrap gap="form-page-gap">
      {children}
    </Row>
  );
}

/** A link of a screen, in the links' color; `newTab` opens it in a new tab and says so to assistive technology. */
export function FormLink({
  href,
  newTab,
  children,
}: {
  href: string;
  /** The words that say it opens in a new tab, for assistive technology; without them it opens here. */
  newTab?: string | undefined;
  children: ReactNode;
}) {
  const s = form();
  return newTab ? (
    <a className={s.link()} href={href} target="_blank" rel="noopener noreferrer">
      {children}
      <span className="sr-only"> {newTab}</span>
    </a>
  ) : (
    <a className={s.link()} href={href}>
      {children}
    </a>
  );
}

/**
 * A part of a screen under its heading (a group of permissions, a moderation action); `apart` sets what cannot be
 * undone last and apart, under a heading of a title's size.
 */
export function FormSection({
  id,
  heading,
  apart = false,
  children,
}: {
  id: string;
  heading: ReactNode;
  apart?: boolean;
  children: ReactNode;
}) {
  const s = form();
  return (
    <Stack
      as="section"
      gap={apart ? "form-page-field-gap" : "form-page-list-gap"}
      spaceAbove={apart ? "form-page-gap" : undefined}
      aria-labelledby={id}
    >
      <h2 id={id} className={apart ? s.apartHeading() : s.sectionHeading()}>
        {heading}
      </h2>
      {children}
    </Stack>
  );
}

/** A list of short items under a section's heading. */
export function FormList({ items }: { items: ReactNode[] }) {
  return (
    <Stack as="ul" gap="form-page-list-gap" className={form().list()}>
      {items.map((item, i) => (
        <li key={i}>{item}</li>
      ))}
    </Stack>
  );
}

/** A state in a word or two beside a heading (In effect), on the raised surface. */
export function FormBadge({ children }: { children: string }) {
  return (
    <Box as="span" spaceBefore="form-page-list-gap" className={form().badge()}>
      {children}
    </Box>
  );
}

/** A line that asks for care before agreeing, after a triangle: `strong` when it may mean another app altogether. */
export function FormCaution({ strong = false, children }: { strong?: boolean; children: ReactNode }) {
  return (
    <GlyphLine nudge="caution" gap="entry-caption" as={<p className={form({ strong }).caution()} />}>
      <Icon name="caution" place="action" />
      <span dir="auto">{children}</span>
    </GlyphLine>
  );
}

/** The account a choice is made as, its avatar beside the line that names it. */
export function FormAccount({ name, children }: { name: string; children: ReactNode }) {
  return (
    <Row as="p" align="center" gap="form-page-alert-gap">
      <Avatar name={name} size="inline" decorative />
      <span dir="auto">{children}</span>
    </Row>
  );
}

/** A button's label after its glyph (a passkey, a login provider's mark). */
export function GlyphLabel({ icon, children }: { icon: IconName; children: ReactNode }) {
  return (
    <Row as="span" inline align="center" gap="form-page-alert-gap">
      <Icon name={icon} place="action" />
      <span dir="auto">{children}</span>
    </Row>
  );
}

/**
 * The place of a third party's widget (a CAPTCHA), which draws itself in an iframe: an element of the service's
 * `markerClassName` class with the data attributes it reads, holding the height of its frame so that the form does not jump.
 */
export function WidgetSlot({
  markerClassName,
  data,
}: {
  markerClassName: string;
  data: Readonly<Record<`data-${string}`, string>>;
}) {
  return <div className={`${markerClassName} ${form().widget()}`} {...data} />;
}

const bare = tv({
  slots: {
    dateTime: [
      "box-border h-control-form rounded-control border-0 bg-surface-raised px-control-timeline-inline",
      "font-sans text-body text-text tabular-nums outline-hidden [color-scheme:inherit] focus-visible:shadow-focus",
      "aria-invalid:outline-solid aria-invalid:outline-[length:var(--form-page-invalid)]",
      "aria-invalid:-outline-offset-[var(--form-page-invalid)] aria-invalid:outline-text",
    ],
    // As tall as what it holds, never cut, at least a control's height.
    textArea: [
      "box-border min-h-control-form resize-none rounded-control border-0 bg-surface-raised",
      "px-control-timeline-inline py-control-timeline-inline font-sans text-body text-text outline-hidden",
      "[field-sizing:content] [word-break:auto-phrase] focus-visible:shadow-focus",
    ],
  },
});

/**
 * A field of a date and a time (`datetime-local`) with no label of its own: `label` names it for assistive technology,
 * `value`, `min` and `max` are as the browser holds them (2026-11-16T09:00), `invalid` draws the line of a refusal,
 * and `describedBy` names the ids that describe it.
 */
export function DateTimeField({
  label,
  value,
  onChange,
  min,
  max,
  invalid = false,
  describedBy,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  min?: string | undefined;
  max?: string | undefined;
  invalid?: boolean;
  describedBy?: string | undefined;
}) {
  return (
    <Box
      full
      shrink
      as={
        <input
          type="datetime-local"
          className={bare().dateTime()}
          aria-label={label}
          aria-invalid={invalid ? true : undefined}
          aria-describedby={describedBy}
          value={value}
          min={min}
          max={max}
          onChange={(e) => {
            onChange(e.target.value);
          }}
        />
      }
    />
  );
}

/**
 * A box for a few lines of text with no label of its own, as tall as the text it holds and at least a control's
 * height, read in the direction of its text; `label` names it for assistive technology. `autoFocus` takes the focus as
 * it mounts, for a box the reader just opened.
 */
export function BareTextArea({
  label,
  value,
  onChange,
  maxLength,
  autoFocus = false,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  maxLength?: number | undefined;
  autoFocus?: boolean;
}) {
  return (
    <Box
      full
      as={
        <textarea
          className={bare().textArea()}
          aria-label={label}
          value={value}
          maxLength={maxLength}
          dir="auto"
          autoFocus={autoFocus}
          onChange={(e) => {
            onChange(e.target.value);
          }}
        />
      }
    />
  );
}
