// SPDX-License-Identifier: AGPL-3.0-only

// A page of its own outside the app's frame (a sign-in, a registration, a consent), and the parts of its forms. The
// page is one column with no card on the page's surface: on a phone and a tablet, the server's mark at its start, the
// screen, and the footer; from `desktop`, the server's introduction (its banner, its mark and its description) on a
// raised panel of 5/12 at the start, which stays in view while a long form scrolls, and the column beside it without
// the mark, so that the reader sees which server they are on. With no banner, the introduction sits in the panel's
// middle. Fields are raised surfaces; what went wrong takes a glyph and the text's color, never a hue, since the accent
// means the primary action and links only. A form's own error sits right before the button that sent it, where the
// reader is when they send it again; a field's sits under the field. Like the app's frame (shell.tsx), it reads the
// viewport, and it carries the page's surface onto <html>.

import type { ComponentProps, ReactNode, Ref } from "react";
import { tv } from "tailwind-variants/lite";
import { Avatar } from "./avatar";
import { Icon, type IconName } from "./icons";

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
    introBody: "flex w-full max-w-[var(--form-page-intro-width)] flex-col gap-form-page-intro-gap [&_p]:m-0",
    banner: "aspect-banner w-full rounded-overlay object-cover",
    column: [
      "flex w-full max-w-[var(--form-page-column)] flex-col gap-form-page-gap",
      "desktop:box-border desktop:max-w-[var(--form-page-column-wide)] desktop:justify-self-center",
      "desktop:px-shell-edge desktop:pt-form-page-top-wide desktop:pb-form-page-bottom",
    ],
    header: "desktop:hidden",
    main: "flex flex-1 flex-col gap-form-page-screen-gap",
    footer: "text-caption text-muted-text",
    footerList: "m-0 flex list-none flex-wrap gap-x-form-page-gap gap-y-form-page-field-gap p-0",
    footerLink: "text-muted-text",
  },
  variants: {
    banner: { false: { intro: "desktop:items-center" }, true: { intro: "desktop:items-start" } },
  },
});

const mark = tv({
  slots: {
    root: "flex items-center gap-form-page-mark-gap",
    wordmarkLight: "block [:root[data-theme=dark]_&]:hidden dark:[:root:not([data-theme=light])_&]:hidden",
    wordmarkDark: "hidden [:root[data-theme=dark]_&]:block dark:[:root:not([data-theme=light])_&]:block",
    logo: "rounded-control object-contain",
    name: "text-muted-text",
  },
  variants: {
    size: {
      line: { wordmarkLight: "h-form-page-mark", wordmarkDark: "h-form-page-mark", logo: "size-form-page-logo" },
      intro: {
        wordmarkLight: "h-form-page-mark-intro",
        wordmarkDark: "h-form-page-mark-intro",
        logo: "size-form-page-logo-intro",
        name: "text-title",
      },
    },
    own: { true: { name: "font-label text-text" } },
  },
});

/** The server's mark: its own logo (for light and dark) with its name, else the built-in wordmark with its name. */
function Mark({
  server,
  wordmark,
  size,
}: {
  server: FormPageServer;
  wordmark: { light: string; dark: string };
  size: "line" | "intro";
}) {
  const own = server.logo !== undefined && server.logo !== "";
  const s = mark({ size, own });
  return (
    <div className={s.root()}>
      {own ? (
        <>
          <img className={`${s.logo()} ${server.logoDark ? s.wordmarkLight() : ""}`} src={server.logo} alt="" />
          {server.logoDark && <img className={`${s.logo()} ${s.wordmarkDark()}`} src={server.logoDark} alt="" />}
        </>
      ) : (
        <>
          <img className={s.wordmarkLight()} src={wordmark.light} alt="" />
          <img className={s.wordmarkDark()} src={wordmark.dark} alt="" />
        </>
      )}
      <span className={s.name()} dir="auto">
        {server.name}
      </span>
    </div>
  );
}

export type FormPageProps = {
  server: FormPageServer;
  /** The built-in wordmark, for light and dark, which a server without a logo of its own shows. */
  wordmark: { light: string; dark: string };
  /** The footer (FormPageFooter). */
  footer?: ReactNode;
  children: ReactNode;
};

/** A page of its own outside the app's frame: the server's mark or introduction, the screen, and the footer. */
export function FormPage({ server, wordmark, footer, children }: FormPageProps) {
  const banner = server.banner !== undefined && server.banner !== "";
  const s = page({ banner });
  return (
    <div className={s.root()}>
      <aside className={s.intro()} aria-label={server.name}>
        <div className={s.introBody()}>
          {banner && <img className={s.banner()} src={server.banner} alt="" />}
          <Mark server={server} wordmark={wordmark} size="intro" />
          {server.description && <p dir="auto">{server.description}</p>}
        </div>
      </aside>
      <div className={s.column()}>
        <header className={s.header()}>
          <Mark server={server} wordmark={wordmark} size="line" />
        </header>
        <main className={s.main()}>{children}</main>
        {footer}
      </div>
    </div>
  );
}

/** The footer of a page: links to the server's documents and its status page, under a name for assistive technology. */
export function FormPageFooter({ label, links }: { label: string; links: { href: string; label: string }[] }) {
  const s = page();
  if (links.length === 0) return null;
  return (
    <footer className={s.footer()}>
      <nav aria-label={label}>
        <ul className={s.footerList()}>
          {links.map((link) => (
            <li key={link.href}>
              <a className={s.footerLink()} href={link.href}>
                {link.label}
              </a>
            </li>
          ))}
        </ul>
      </nav>
    </footer>
  );
}

const form = tv({
  slots: {
    head: "grid gap-form-page-head-gap [&_p]:m-0",
    title: "m-0 text-title",
    text: "m-0",
    hint: "m-0 text-caption text-muted-text",
    form: "m-0 flex flex-col gap-form-page-form-gap",
    field: "grid gap-form-page-field-gap",
    labelLine: "flex items-baseline justify-between gap-form-page-alert-gap",
    label: "font-label",
    input: [
      "box-border h-control-form w-full rounded-control border-0 bg-surface-raised px-control-timeline-inline font-sans",
      "text-body text-text outline-hidden focus-visible:shadow-focus read-only:text-muted-text",
      "aria-invalid:outline-solid aria-invalid:outline-[length:var(--form-page-invalid)]",
      "aria-invalid:-outline-offset-[var(--form-page-invalid)] aria-invalid:outline-text",
    ],
    textarea: "h-auto resize-y py-control-timeline-inline",
    fieldError: "m-0 flex items-start gap-entry-caption font-label [&_svg]:mt-[0.2em]",
    check: "flex items-start gap-form-page-alert-gap",
    checkInput: [
      "m-0 size-form-page-check flex-none accent-text",
      "aria-invalid:outline-solid aria-invalid:outline-[length:var(--form-page-invalid)]",
      "aria-invalid:outline-offset-[var(--form-page-invalid)] aria-invalid:outline-text",
    ],
    alert: [
      "m-0 flex items-start gap-form-page-alert-gap rounded-overlay bg-text p-form-page-alert-padding font-label",
      "text-surface outline-hidden focus-visible:shadow-focus [&_svg]:mt-[0.0625rem]",
    ],
    actions: "grid gap-form-page-field-gap [&>a]:w-full [&>button]:w-full",
    choice: "flex gap-form-page-field-gap [&>button]:flex-1",
    others: "grid gap-form-page-field-gap",
    othersHeading: "m-0 text-caption font-normal text-muted-text",
    links: "m-0 flex flex-wrap gap-form-page-gap",
    link: "text-accent-text",
    section: "grid gap-form-page-list-gap [&_p]:m-0",
    sectionHeading: "m-0 text-body font-label",
    apart: "mt-form-page-gap grid gap-form-page-field-gap",
    apartHeading: "m-0 text-title",
    list: "m-0 grid gap-form-page-list-gap ps-form-page-indent",
    badge: "ms-form-page-list-gap rounded-control bg-surface-raised px-form-page-list-gap text-caption font-normal",
    caution: "m-0 flex items-start gap-entry-caption [&_svg]:mt-[0.1em]",
    account: "m-0 flex items-center gap-form-page-alert-gap",
    glyph: "inline-flex items-center gap-form-page-alert-gap",
    widget: "min-h-form-page-widget",
  },
  variants: {
    strong: { true: { caution: "font-label" } },
  },
});

/** The title of a screen, its page's one h1, and the lines under it. */
export function FormHead({ title, children }: { title: string; children?: ReactNode }) {
  const s = form();
  return (
    <div className={s.head()}>
      <h1 className={s.title()} dir="auto">
        {title}
      </h1>
      {children}
    </div>
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
    <p className={muted ? s.hint() : s.text()} id={id} role={role} dir="auto">
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
    <form ref={ref} className={form().form()} method="post" action={action} onSubmit={onSubmit}>
      {children}
    </form>
  );
}

export type TextFieldProps = {
  id: string;
  name: string;
  label: string;
  type?: "text" | "email" | "password" | undefined;
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
};

/**
 * A labelled field on a raised surface: its label and hint above it, its error right under it, both read with it
 * (aria-describedby). It posts under `name`.
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
}: TextFieldProps) {
  const s = form();
  const described =
    [describedBy, hint && `${id}-hint`, error && `${id}-error`].filter((part) => part).join(" ") || undefined;
  const common = {
    id,
    name,
    defaultValue,
    readOnly,
    required,
    maxLength,
    "aria-invalid": error ? true : undefined,
    "aria-describedby": described,
  };
  return (
    <div className={s.field()}>
      <div className={s.labelLine()}>
        <label className={s.label()} htmlFor={id}>
          {label}
        </label>
        {end}
      </div>
      {hint && (
        <p id={`${id}-hint`} className={s.hint()}>
          {hint}
        </p>
      )}
      {multiline ? (
        <textarea {...common} className={`${s.input()} ${s.textarea()}`} rows={4} dir="auto" />
      ) : (
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
      )}
      {error && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </div>
  );
}

/** A field's error, right under what it is about. */
export function FieldError({ id, children }: { id?: string | undefined; children: string }) {
  return (
    <p id={id} className={form().fieldError()}>
      <Icon name="error" place="caption" />
      <span dir="auto">{children}</span>
    </p>
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
    <div className={s.field()}>
      <label className={s.check()}>
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
        <span>{label}</span>
      </label>
      {error && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </div>
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
export function FormAlert({ focus = false, children }: { focus?: boolean; children: string }) {
  return (
    <p className={form().alert()} role="alert" tabIndex={-1} {...{ [focusOnLoadAttribute]: focus ? "" : undefined }}>
      <Icon name="error" place="action" />
      <span dir="auto">{children}</span>
    </p>
  );
}

/** A column of full-width actions, the primary first, with a form's alert right before them. */
export function FormActions({ children }: { children: ReactNode }) {
  return <div className={form().actions()}>{children}</div>;
}

/**
 * A pair of a refusal and its confirmation side by side, each half the column: the refusal at the start, the
 * confirmation at the end, so that the row mirrors in right-to-left text.
 */
export function FormChoice({ children }: { children: ReactNode }) {
  return <div className={form().choice()}>{children}</div>;
}

/** Other ways to go on (a passkey, a login provider) under a quiet heading. */
export function FormOthers({ heading, children }: { heading: string; children: ReactNode }) {
  const s = form();
  return (
    <section className={s.others()} aria-label={heading}>
      <h2 className={s.othersHeading()}>{heading}</h2>
      <div className={s.actions()}>{children}</div>
    </section>
  );
}

/** The links that lead off the screen, at its end. */
export function FormLinks({ children }: { children: ReactNode }) {
  return <p className={form().links()}>{children}</p>;
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
    <section className={apart ? s.apart() : s.section()} aria-labelledby={id}>
      <h2 id={id} className={apart ? s.apartHeading() : s.sectionHeading()}>
        {heading}
      </h2>
      {children}
    </section>
  );
}

/** A list of short items under a section's heading. */
export function FormList({ items }: { items: ReactNode[] }) {
  return (
    <ul className={form().list()}>
      {items.map((item, i) => (
        <li key={i}>{item}</li>
      ))}
    </ul>
  );
}

/** A state in a word or two beside a heading (In effect), on the raised surface. */
export function FormBadge({ children }: { children: string }) {
  return <span className={form().badge()}>{children}</span>;
}

/** A line that asks for care before agreeing, after a triangle: `strong` when it may mean another app altogether. */
export function FormCaution({ strong = false, children }: { strong?: boolean; children: ReactNode }) {
  return (
    <p className={form({ strong }).caution()}>
      <Icon name="caution" place="action" />
      <span dir="auto">{children}</span>
    </p>
  );
}

/** The account a choice is made as, its avatar beside the line that names it. */
export function FormAccount({ name, children }: { name: string; children: ReactNode }) {
  return (
    <p className={form().account()}>
      <Avatar name={name} size="inline" decorative />
      <span dir="auto">{children}</span>
    </p>
  );
}

/** A button's label after its glyph (a passkey, a login provider's mark). */
export function GlyphLabel({ icon, children }: { icon: IconName; children: ReactNode }) {
  return (
    <span className={form().glyph()}>
      <Icon name={icon} place="action" />
      <span dir="auto">{children}</span>
    </span>
  );
}

/**
 * The place of a third party's widget (a CAPTCHA), which draws itself in an iframe: an element of the service's
 * `marker` class with the data attributes it reads, holding the height of its frame so that the form does not jump.
 */
export function WidgetSlot({ marker, data }: { marker: string; data: Readonly<Record<`data-${string}`, string>> }) {
  return <div className={`${marker} ${form().widget()}`} {...data} />;
}
