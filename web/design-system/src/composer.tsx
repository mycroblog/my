// SPDX-License-Identifier: AGPL-3.0-only

// The parts of the post composer (ADR-0025 §4), from the rough the decider approved (#3046): the frame, a dialog over
// the dimmed page from `tablet` and a screen of its own on a phone (where the app asks for one), whose row with the
// action stays right above the software keyboard; the text with its count, and the content warning, each around its
// editor (editor.tsx, loaded lazily); the images as rows of a thumbnail and its
// description; the settings of a poll under its options, its duration
// a select that opens the one menu (menu.tsx); and the notice of a discarded post
// with its Undo. Its head may hold more before close, and a line over the row may say how the post is
// sent (when it is scheduled for); with neither tools nor an action (the drafts), it has no row. The frame tells how it is left: its close button and Escape (`onClose`, meant), and a
// press on the backdrop (`onDismiss`, by accident), and neither while what was written is sent (`busy`). Every word is
// the caller's.
//
// On a phone the frame is as tall as the viewport less the keyboard: `--keyboard-inset` where the app measures it
// (Safari's visualViewport), else `env(keyboard-inset-height)` where the browser overlays the keyboard; Chrome on Android
// resizes the viewport itself (`interactive-widget=resizes-content`).

import { Suspense, useId, useRef, type KeyboardEvent, type ReactNode } from "react";
import { Button as AriaButton, Dialog, Heading, Link as AriaLink } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Button } from "./button";
import { FieldError } from "./form-page";
import { IconButton } from "./icon-button";
import { Icon, type IconName } from "./icons";
import { Box, Pin, Row, Stack } from "./layout";
import { useReader } from "./locale";
import { MenuGroup, MenuItem, MenuSurface } from "./menu";
import { ModalLayer } from "./modal";

const frame = tv({
  slots: {
    panel: [
      "relative box-border flex flex-col gap-entry bg-surface font-sans text-body text-text outline-hidden",
      "h-[calc(100dvh-var(--keyboard-inset,env(keyboard-inset-height,0px)))] px-shell-edge pt-entry-line",
    ],
    body: "flex min-h-0 flex-1 flex-col gap-entry overflow-y-auto",
    end: "relative grid gap-entry-line bg-surface py-entry",
    // On the narrowest phones (under 360 px) the glyphs sit closer, and the language names itself by its tag
    // (ToolText), so that the tools, the count and Post keep one line.
    tools: "max-[22.5rem]:[--icon-button-padding-inline:0.25rem]",
  },
  variants: {
    // From `tablet` a dialog of the main column's width over the dimmed page, which scrolls with what opens below it;
    // the page behind keeps its place.
    dialog: {
      true: {
        panel: [
          "tablet:mx-auto tablet:h-auto tablet:w-[min(var(--shell-column),100%)] tablet:rounded-overlay tablet:p-dialog",
          "tablet:pt-[calc(var(--dialog-padding)*0.6)] tablet:shadow-overlay",
        ],
        body: "tablet:overflow-visible",
        end: "tablet:py-0",
      },
    },
    // A specimen in a page is as tall as what it holds, with a form's room unless it fits what it holds (`fit`).
    layer: { false: { panel: "h-auto" } },
    room: { true: { panel: "min-h-[24rem]" } },
  },
});

/**
 * The frame of the composer, one of the frames of a page: the head (the title, then what ends it), what is written
 * and at its end what stands over the row and the row of the tools, the count and the action. On a phone it is a screen
 * as tall as the viewport less the keyboard; from `tablet` a dialog of the main column's width, as tall as what it
 * holds. A specimen in a page (`layer` off) is as tall as what it holds at every width.
 */
function ComposerPanel({
  phone,
  layer,
  room,
  dir,
  onKeyDown,
  title,
  headEnd,
  over,
  tools,
  count,
  action,
  children,
}: {
  phone: "screen" | "dialog";
  layer: boolean;
  room: boolean;
  dir: "ltr" | "rtl";
  onKeyDown: (e: KeyboardEvent<HTMLElement>) => void;
  title: ReactNode;
  headEnd: ReactNode;
  over: ReactNode;
  tools: ReactNode;
  count: ReactNode;
  action: ReactNode;
  children: ReactNode;
}) {
  const s = frame({ dialog: true, layer, room });
  return (
    <div
      className={phone === "dialog" ? `${s.panel()} h-auto rounded-overlay p-dialog shadow-overlay` : s.panel()}
      dir={dir}
      onKeyDown={onKeyDown}
    >
      <Row align="center" justify="between" gap="entry-caption" bleedEnd="icon-button-inline">
        {title}
        <Row as="span" inline rigid align="center">
          {headEnd}
        </Row>
      </Row>
      <div className={s.body()}>{children}</div>
      {(tools !== undefined || action !== undefined) && (
        <div className={s.end()}>
          {over}
          <Row align="center" gap="entry-caption">
            <Row grow wrap align="center" bleedStart="icon-button-inline" className={s.tools()}>
              {tools}
            </Row>
            {count}
            {action}
          </Row>
        </div>
      )}
    </div>
  );
}

export type ComposerFrameProps = {
  /** The title of what is written: a new post, a reply, a quote. */
  title: string;
  /** The close button's name. */
  closeLabel: string;
  /** Close or Escape: the reader means to leave. */
  onClose: () => void;
  /** A press on the backdrop: the reader left by accident. */
  onDismiss: () => void;
  /**
   * What was written is being sent or saved: close stays in its place, disabled (the action's spinner says it is sent),
   * and neither it, Escape nor the backdrop leaves.
   */
  busy?: boolean | undefined;
  /** How it shows on a phone: a screen of its own (the composer), or a dialog as from `tablet`. */
  phone?: "screen" | "dialog" | undefined;
  /** Over the page, as the app opens it, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
  /**
   * As tall as what it holds, also as a specimen, where a frame otherwise keeps a form's room (one line: the form that
   * could not be loaded). Over the page from `tablet` it is always as tall as what it holds.
   */
  fit?: boolean | undefined;
  /** What stands in the head before close (the drafts). */
  headEnd?: ReactNode;
  /** A line about how it will be sent (when it is scheduled for), right above the row that sends it. */
  note?: ReactNode;
  /** The form's error, directly before the row that sends it. */
  alert?: ReactNode;
  /** The tools at the row's start: the glyphs and what they open; with no tools and no action, there is no row. */
  tools?: ReactNode;
  /** What stands before the action: the count. */
  count?: ReactNode;
  /** The action that sends it (Post), and what stands at its end. */
  action?: ReactNode;
  children: ReactNode;
};

/**
 * The composer's frame: the head (the title, then close at its end), what is written, and at its end the form's error
 * and the row of the tools, the count and the action. It holds the focus while it is over the page.
 */
export function ComposerFrame({
  title,
  closeLabel,
  onClose,
  onDismiss,
  busy = false,
  phone = "screen",
  layer = true,
  fit = false,
  headEnd,
  note,
  alert,
  tools,
  count,
  action,
  children,
}: ComposerFrameProps) {
  const leave = (how: () => void) => {
    if (!busy) how();
  };
  const { direction } = useReader(undefined);
  const titleId = useId();
  const panel = (
    <ComposerPanel
      phone={phone}
      layer={layer}
      room={!layer && !fit}
      dir={direction}
      onKeyDown={(e) => {
        // Escape means to leave, but not while an input method composes, where it cancels the composition.
        if (e.key === "Escape" && !e.defaultPrevented && !e.nativeEvent.isComposing) {
          e.preventDefault();
          leave(onClose);
        }
      }}
      title={
        layer ? (
          <Heading slot="title" className="text-title">
            {title}
          </Heading>
        ) : (
          <h2 id={titleId} className="text-title">
            {title}
          </h2>
        )
      }
      headEnd={
        <>
          {headEnd}
          <IconButton
            icon="close"
            label={closeLabel}
            isDisabled={busy}
            onPress={() => {
              leave(onClose);
            }}
          />
        </>
      }
      over={
        <>
          {note}
          {alert}
        </>
      }
      tools={tools}
      count={count}
      action={action}
    >
      {children}
    </ComposerPanel>
  );
  if (!layer)
    return (
      <div role="dialog" aria-labelledby={titleId}>
        {panel}
      </div>
    );
  // A modal (modal.tsx) that shows on the frame it mounts in, its text focused inside the press; Escape is the
  // panel's, which tells it from a press on the backdrop.
  return (
    <ModalLayer
      onDismiss={() => {
        leave(onDismiss);
      }}
      ownEscape
      backdrop={phone === "dialog" ? "always" : "tablet"}
      place="screen"
    >
      <Box as={<Dialog className="outline-hidden" />} full className="box-border">
        {panel}
      </Box>
    </ModalLayer>
  );
}

const count = tv({
  base: "text-caption text-muted-text tabular-nums",
  variants: { near: { true: "text-text" }, over: { true: "font-label text-text" } },
});

/**
 * How many characters are left before the limit (negative past it): muted until `near` it, stronger past it. `id` is
 * the id of its words (`label`), by which the field it counts is described by them (`aria-describedby`).
 */
export function CharCount({
  left,
  near,
  label,
  id,
}: {
  left: number;
  near: number;
  label: string;
  id?: string | undefined;
}) {
  return (
    <span className={count({ near: left >= 0 && left <= near, over: left < 0 })}>
      <span aria-hidden="true">{left}</span>
      <span id={id} className="sr-only">
        {label}
      </span>
    </span>
  );
}

/** The room the text takes while its editor arrives, so that the composer does not jump when it comes. */
const waiting = tv({
  variants: {
    roomy: { true: "min-h-composer-room", false: "min-h-composer-line" },
    line: { true: "h-control-form" },
  },
});

/**
 * The text of a post on the composer's surface: its editor (`children`, the `PlainTextEditor` of
 * `@my/design-system/editor` with the composer's look, loaded lazily: its room is held while it arrives), a button at
 * the end of its first line, and what is wrong with it right under it, by `errorId`, which the editor is described by.
 */
export function ComposerText({
  error,
  errorId,
  roomy = true,
  end,
  children,
}: {
  error?: string | undefined;
  errorId?: string | undefined;
  /** Three lines of room while nothing follows the text, while its editor arrives. */
  roomy?: boolean | undefined;
  /** A button at the end of the text's first line (the emoji picker's), which the editor keeps clear of (`ended`). */
  end?: ReactNode;
  children: ReactNode;
}) {
  return (
    <Stack gap="form-page-field-gap">
      <Stack anchor>
        <Suspense fallback={<div className={waiting({ roomy })} aria-busy="true" />}>{children}</Suspense>
        {end !== undefined && (
          <Pin top="0" end="0" layer={10}>
            {end}
          </Pin>
        )}
      </Stack>
      {error && <FieldError id={errorId}>{error}</FieldError>}
    </Stack>
  );
}

const warning = tv({
  slots: {
    // The suggestions of its editor open under the band, across its width.
    // As tall as its editor's line also while it has none (an editor that could not be loaded).
    root: "min-h-control-form rounded-control bg-surface-raised ps-disclosure-start",
    glyph: "text-muted-text",
  },
});

/**
 * The content warning: a band of the raised surface, as a post shows it, to type the warning on, with its editor
 * (`children`, the `PlainTextEditor` with the `line` look, loaded lazily: its room is held while it arrives), and what
 * is wrong with it right under it, by `errorId`, which the editor is described by.
 */
export function WarningField({
  error,
  errorId,
  children,
}: {
  error?: string | undefined;
  errorId?: string | undefined;
  children: ReactNode;
}) {
  const s = warning();
  return (
    <Stack gap="form-page-field-gap">
      <Row anchor align="center" gap="disclosure-gap" className={s.root()}>
        <Row as="span" inline className={s.glyph()}>
          <Icon name="hidden" place="caption" />
        </Row>
        <Suspense fallback={<Box grow className={waiting({ line: true })} aria-busy="true" />}>{children}</Suspense>
      </Row>
      {error && <FieldError id={errorId}>{error}</FieldError>}
    </Stack>
  );
}

const attachment = tv({
  slots: {
    thumb: "size-composer-thumb rounded-control bg-surface-raised object-cover",
    placeholder: "size-composer-thumb rounded-control bg-surface-raised text-muted-text",
    label: "font-label",
    description: [
      "box-border min-h-control-form resize-none rounded-control border-0 bg-surface-raised py-composer-field-block",
      "px-control-timeline-inline font-sans text-body text-text outline-hidden [field-sizing:content]",
      "placeholder:text-muted-text focus-visible:shadow-focus",
      "aria-invalid:outline-solid aria-invalid:outline-[length:var(--form-page-invalid)]",
      "aria-invalid:-outline-offset-[var(--form-page-invalid)] aria-invalid:outline-text",
    ],
    status: "pt-entry-line text-caption text-muted-text",
  },
  variants: { waiting: { true: { thumb: "opacity-disabled" } } },
});

/** The list of the images of a post, a row each. */
export function AttachmentList({ children }: { children: ReactNode }) {
  return <ul className="m-0 grid list-none gap-entry p-0">{children}</ul>;
}

/** An image's row: its thumbnail, its description and field, and remove at its end, the middle taking the room left. */
function AttachmentLine({ children }: { children: ReactNode }) {
  return (
    <li className="grid grid-cols-[var(--composer-thumb)_minmax(0,1fr)_auto] items-start gap-entry-avatar">
      {children}
    </li>
  );
}

export type AttachmentRowProps = {
  /** The image's preview, or none for a file that has none (a video without a poster). */
  src?: string | undefined;
  /** Uploading or processing, `failed`, or `ready` for its description. */
  state: "uploading" | "processing" | "failed" | "ready";
  /** What happens to it while it is not ready: uploading, processing, or that it failed. */
  status: string;
  /** A failed upload's retry. */
  retry?: { label: string; onPress: () => void } | undefined;
  /** The description, its field's label and the hint in it. */
  description: string;
  onDescription: (value: string) => void;
  descriptionLabel: string;
  placeholder: string;
  /** A description that is missing: a caution, or an error where the server asks for every one. */
  missing?: { text: string; required: boolean } | undefined;
  /** What the server refused of the description, in its place of `missing`. */
  error?: string | undefined;
  removeLabel: string;
  onRemove: () => void;
};

/** An image of a post: its thumbnail beside its description, which is asked for where it is missing; remove at the end. */
export function AttachmentRow({
  src,
  state,
  status,
  retry,
  description,
  onDescription,
  descriptionLabel,
  placeholder,
  missing: missingNote,
  error,
  removeLabel,
  onRemove,
}: AttachmentRowProps) {
  const s = attachment({ waiting: state === "uploading" || state === "processing" });
  const id = useId();
  const missing = error ? { text: error, required: true } : missingNote;
  return (
    <AttachmentLine>
      {src === undefined ? (
        <Row as="span" inline align="center" justify="center" className={s.placeholder()}>
          <Icon name="attach" place="action" />
        </Row>
      ) : (
        <img className={s.thumb()} src={src} alt="" />
      )}
      <Stack gap="entry-line">
        {state === "ready" ? (
          <>
            <label className={s.label()} htmlFor={id}>
              {descriptionLabel}
            </label>
            <Box
              as={
                <textarea
                  id={id}
                  rows={2}
                  value={description}
                  placeholder={placeholder}
                  maxLength={1500}
                  aria-invalid={missing?.required ? true : undefined}
                  aria-describedby={missing ? `${id}-note` : undefined}
                  dir="auto"
                  onChange={(e) => {
                    onDescription(e.target.value);
                  }}
                />
              }
              full
              className={s.description()}
            />
            {missing &&
              (missing.required ? (
                <FieldError id={`${id}-note`}>{missing.text}</FieldError>
              ) : (
                <Row as="p" id={`${id}-note`} wrap align="center" gap="entry-caption" className={s.status()}>
                  <Icon name="caution" place="caption" />
                  {missing.text}
                </Row>
              ))}
          </>
        ) : (
          <Row as="p" wrap align="center" gap="entry-caption" className={s.status()} role="status">
            {state === "failed" && <Icon name="error" place="caption" />}
            <span>{status}</span>
            {retry && (
              <Button kind="secondary" size="timeline" onPress={retry.onPress}>
                {retry.label}
              </Button>
            )}
          </Row>
        )}
      </Stack>
      <IconButton icon="close" label={removeLabel} onPress={onRemove} />
    </AttachmentLine>
  );
}

/** The tool that adds files: an icon button that opens the system's file picker. */
export function AttachButton({
  label,
  accept,
  isDisabled = false,
  onFiles,
}: {
  label: string;
  /** The types of file it takes, as an input's accept. */
  accept: string;
  isDisabled?: boolean | undefined;
  onFiles: (files: File[]) => void;
}) {
  const input = useRef<HTMLInputElement>(null);
  return (
    <>
      <IconButton
        icon="attach"
        label={label}
        isDisabled={isDisabled}
        onPress={() => {
          input.current?.click();
        }}
      />
      <input
        ref={input}
        type="file"
        accept={accept}
        multiple
        hidden
        onChange={(e) => {
          const files = Array.from(e.target.files ?? []);
          e.target.value = "";
          if (files.length > 0) onFiles(files);
        }}
      />
    </>
  );
}

/** The settings of a poll under its options: how long it runs, and how it counts. */
export function PollSettings({ children }: { children: ReactNode }) {
  return <div className="grid justify-items-start gap-entry-line">{children}</div>;
}

/** A value a `ChoiceSelect` offers. */
export type Choice<T extends string> = {
  id: T;
  icon: IconName;
  label: string;
};

const select = tv({
  base: [
    "h-control-timeline cursor-pointer rounded-control border-0 bg-surface",
    "px-control-timeline-inline font-sans text-body text-text outline-hidden [&>svg]:text-muted-text",
    "data-hovered:bg-surface-hover data-pressed:bg-surface-pressed data-pressed:translate-y-press",
    "data-focus-visible:shadow-focus",
  ],
});

/**
 * A control that opens a menu of choices, as a select reads (a poll's duration): on the fields' surface, its glyph,
 * what it holds now and a chevron; the menu opens under it, the chosen one checked, and a choice closes it.
 */
export function ChoiceSelect<T extends string>({
  label,
  icon,
  value,
  items,
  chosen,
  onChoose,
}: {
  /** The name of what it chooses, which names its menu. */
  label: string;
  icon: IconName;
  /** What it holds now, as it reads alone: the control's name, which says what it chooses too ("Ends in 1 day"). */
  value: string;
  items: readonly Choice<T>[];
  chosen: T;
  onChoose: (id: T) => void;
}) {
  return (
    <MenuSurface
      label={label}
      placement="bottom start"
      trigger={
        <Row as={<AriaButton />} inline align="center" gap="entry-caption" className={select()}>
          <Icon name={icon} place="caption" />
          <span>{value}</span>
          <Icon name="expand" place="caption" />
        </Row>
      }
    >
      <MenuGroup
        chosen={chosen}
        onChoose={(id) => {
          const item = items.find((i) => i.id === id);
          if (item) onChoose(item.id);
        }}
      >
        {items.map((item) => (
          <MenuItem key={item.id} id={item.id} icon={item.icon}>
            {item.label}
          </MenuItem>
        ))}
      </MenuGroup>
    </MenuSurface>
  );
}

/**
 * What a tool of the row says: its words, or on the narrowest phones (under 360 px) `short`. One of the frames of a
 * page: it reads the viewport.
 */
function ToolWords({ short, children }: { short: string; children: string }) {
  return (
    <>
      <span className="max-[22.5rem]:hidden">{children}</span>
      <span className="hidden max-[22.5rem]:inline">{short}</span>
    </>
  );
}

/**
 * A tool of the row that names what it holds in words (the language), muted at the caption's size, the trigger of its
 * menu (`MenuSurface`); on the narrowest phones (under 360 px) it names it by `short` (EN), so that the row keeps its
 * tools and Post on one line.
 */
export function ToolText({
  label,
  lang,
  short,
  children,
}: {
  /** Its words on the narrowest phones (the language's tag). */
  short?: string | undefined;
  /** Its name for assistive technology: what it is and what it holds. */
  label: string;
  /** The language of its words. */
  lang?: string | undefined;
  children: string;
}) {
  return (
    <Button kind="quiet" size="timeline">
      {/* Its name is the label alone, read in place of its words, which may be the short tag. */}
      <span className="sr-only">{label}</span>
      <span className="text-caption font-label text-muted-text" lang={lang} aria-hidden="true">
        {short === undefined ? children : <ToolWords short={short}>{children}</ToolWords>}
      </span>
    </Button>
  );
}

const notice = tv({
  base: [
    "box-border flex w-max items-center gap-entry-avatar",
    "rounded-overlay bg-text py-composer-inset ps-control-timeline-inline pe-composer-inset font-sans text-body",
    "text-surface shadow-overlay",
  ],
  variants: {
    inline: {
      // In the flow nothing floats beside it: it may be as wide as what holds it.
      true: "max-w-full",
      // Over the page, as wide at most as leaves the action floating at the end clear, with an edge on either side.
      false: [
        "fixed inset-x-shell-edge bottom-[calc(var(--shell-bar)+var(--shell-edge)+env(safe-area-inset-bottom))] z-60",
        "max-w-[calc(100%-3*var(--shell-edge)-var(--shell-bar))]",
        "tablet:inset-x-0 tablet:bottom-shell-end tablet:mx-auto tablet:max-w-[calc(100%-2*var(--shell-edge))]",
      ],
    },
    bare: { true: "pe-control-timeline-inline" },
  },
});

/**
 * Where a notice stands: over the page at the bottom, at the start on a phone, clear of the action floating at the end,
 * and in the middle of the viewport from `tablet`; or in the flow, as a specimen (`inline`). One of the frames of a
 * page: it reads the viewport.
 */
function NoticePlace({
  inline,
  bare,
  dir,
  children,
}: {
  inline: boolean;
  bare: boolean;
  dir: "ltr" | "rtl";
  children: ReactNode;
}) {
  return (
    <div className={notice({ inline, bare })} role="status" dir={dir}>
      {children}
    </div>
  );
}

const noticeAction = tv({
  base: [
    "h-control-timeline cursor-pointer rounded-control border-0 bg-transparent px-control-timeline-inline font-sans",
    "text-body font-label text-inherit outline-hidden hover:underline active:translate-y-press",
    "focus-visible:shadow-[inset_0_0_0_var(--focus-ring-width)_var(--color-surface)]",
  ],
  variants: {
    // A link looks as the button does: underlined only while hovered.
    link: { true: "no-underline" },
  },
});

/**
 * A short notice at the bottom of the page, with its action when it has one (a discarded post, with Undo): knocked out
 * and neutral, at the start on a phone, clear of the action floating at the end. Its action is a button doing
 * `onAction`, or a link to `href` where the reader can do something about what the notice says (a refusal of a
 * restricted account, Account status); either keeps its words on one line, and its sentence gives way beside it.
 * `inline` keeps it in the flow (a specimen).
 */
export function UndoNotice({
  action,
  onAction,
  href,
  inline = false,
  children,
}: {
  action?: string | undefined;
  onAction?: (() => void) | undefined;
  href?: string | undefined;
  inline?: boolean | undefined;
  children: string;
}) {
  const { direction } = useReader(undefined);
  const acts = action !== undefined && (onAction !== undefined || href !== undefined);
  return (
    <NoticePlace inline={inline} bare={!acts} dir={direction}>
      {acts ? (
        <Box as="span" shrink>
          {children}
        </Box>
      ) : (
        <Row as="span" align="center" className="min-h-control-timeline">
          {children}
        </Row>
      )}
      {acts && (
        <Row
          inline
          rigid
          align="center"
          justify="center"
          as={
            href === undefined ? (
              <button type="button" className={noticeAction()} onClick={onAction} />
            ) : (
              <AriaLink href={href} className={noticeAction({ link: true })} />
            )
          }
        >
          {action}
        </Row>
      )}
    </NoticePlace>
  );
}
