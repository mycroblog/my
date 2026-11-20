// SPDX-License-Identifier: AGPL-3.0-only

// The modals over the page (modal.tsx), every word the caller's but the name of a confirmation's close, which is the
// design system's own where the caller gives none. The confirmation asks before what cannot be undone
// (deleting a post): a small dialog in the middle of the viewport at every width, its title the question, one or two
// sentences on what follows, and the pair of Cancel and the confirmation side by side at its end, the confirmation at
// the end side; Escape and a press outside cancel. The panel shows something to read (a post's edit history) as the
// composer does: a dialog of the main column's width over the dimmed page from `tablet`, a screen of its own on a
// phone; its head holds the title and close at its end, and its body scrolls under the head. The confirmation that
// asks the reader to authenticate again (before changing the password, deleting the account) is the confirmation's
// dialog with the proof in it: what the confirmation does where it does an act at once (deleting the account), in the
// text's color, then what the proof lets them do, the field of the proof (the password, a two-factor code) or none
// where the confirmation itself proves it (a passkey, going to an external provider), the other ways to prove it, each
// a row of its own a finger's target tall, what the server refused right before the pair, and the pair; where the
// reader has no way to prove it here, it says so and has Cancel alone. A confirmation before what is grave as well as
// final (banning an account, closing the server, deleting a role) has the reader type a stated text first (ADR-0110,
// NFR-MOD-7): under what follows, a field labelled with the text, on a line of its own in the code's face; the
// confirmation stays disabled until the field holds it. A handle in a title breaks before its server, never inside. The head of every modal ends with close, after its title; a confirmation's close cancels,
// as its Cancel does.
//
// For assistive technology and the keyboard: a modal keeps the focus inside it and gives it back as it leaves, and
// the page behind is out of reach. The confirmation with a text to type is a form: the focus starts in its field, and
// Enter there confirms once the field holds the text. The confirmation that asks to authenticate again is a form: the
// focus starts in the field of the proof, Enter in it confirms, what the server refused is an alert that also
// describes the field, and the focus goes back to the field for another try; while the proof is in flight nothing
// confirms it again.

import { Children, isValidElement, useEffect, useId, useRef, useState, type ReactElement, type ReactNode } from "react";
import { Dialog, Heading } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Button } from "./button";
import { FieldContext } from "./field";
import { TextField } from "./form-page";
import { IconButton } from "./icon-button";
import { Box, Row, Stack } from "./layout";
import { stringsIn, useReader } from "./locale";
import { ModalLayer } from "./modal";

const confirm = tv({
  slots: {
    modal: "rounded-overlay bg-surface p-dialog font-sans text-body text-text shadow-overlay outline-hidden",
    dialog: "outline-hidden",
    // Wrapping between phrases, never inside a word of ja.
    title: "text-title [word-break:auto-phrase]",
    text: "text-muted-text [word-break:auto-phrase]",
    // What the press does, where it does an act at once: in the text's color, over the muted line of the proof.
    effect: "text-text [word-break:auto-phrase]",
    // The other ways to prove it, one under another, each a finger's target tall.
    ways: "list-none p-0",
    way: "min-h-icon-button-touch",
    // The text to type, inside the field's label, on a line of its own: in the code's face, breaking anywhere only
    // where it is longer than the line.
    phrase: "font-mono font-strong [overflow-wrap:anywhere]",
  },
});

/**
 * What the reader typed, as it is compared with the text to type: its edges trimmed, its full-width forms folded, and
 * its case, as handles, domains and a role's name are told apart by none (`toLowerCase` reads no locale).
 */
const folded = (text: string) => text.normalize("NFKC").trim().toLowerCase();

/** The label of the field of a text to type: the catalog's sentence, the text in the code's face in its place. */
function PhraseLabel({ lang, phrase, className }: { lang: string; phrase: string; className: string }) {
  const mark = "\u0000";
  const [before = "", after = ""] = stringsIn(lang).t("dialog.type_to_confirm", { phrase: mark }).split(mark);
  // The text to type stands on a line of its own, so that neither it nor the sentence around it breaks inside a word.
  return (
    <>
      <span className="[word-break:auto-phrase]">{before.trim()}</span>{" "}
      <Box as="bdi" display="block" className={className}>
        {phrase}
      </Box>
      {after.trim() !== "" && (
        <>
          {" "}
          <span className="[word-break:auto-phrase]">{after.trim()}</span>
        </>
      )}
    </>
  );
}

/**
 * A title with a chance to break before each handle's server (@name@server.example), so that a handle too long for a
 * line wraps at its second @, never inside a word; the title itself where it has no handle to break.
 */
function titleBreaks(title: string): ReactNode {
  const parts = title.split(/(?<=\S)(?=@)/u);
  if (parts.length === 1) return title;
  return parts.flatMap((part, i) => (i === 0 ? [part] : [<wbr key={i} />, part]));
}

/**
 * The head of a small dialog: its title, which names the dialog (the slot of React Aria's over the page, `titleId`
 * in a page's flow), and close at its end, as in every modal.
 */
function DialogHead({
  title,
  titleId,
  className,
  closeLabel,
  onClose,
  busy,
}: {
  title: string;
  /** The title's id in the flow of a page; none over the page, where React Aria ties the title. */
  titleId: string | undefined;
  className: string;
  closeLabel: string;
  onClose: () => void;
  busy: boolean;
}) {
  // A title broken before a handle's server still names the dialog by its text as written: a name computed from the
  // breaks may read them as spaces.
  const broken = titleBreaks(title);
  const named = broken === title ? undefined : title;
  return (
    <Row align="start" justify="between" gap="entry-caption">
      {titleId === undefined ? (
        <Heading slot="title" className={className} aria-label={named}>
          {broken}
        </Heading>
      ) : (
        <h2 id={titleId} className={className} aria-label={named}>
          {broken}
        </h2>
      )}
      <Row as="span" inline rigid bleedEnd="icon-button-inline">
        <IconButton icon="close" label={closeLabel} onPress={onClose} isDisabled={busy} />
      </Row>
    </Row>
  );
}

export type ConfirmDialogProps = {
  /** The question (Delete this post?). */
  title: string;
  /** What follows the confirmation, in a sentence or two. */
  children: ReactNode;
  /** The close button's name, where the design system's own word (Close) does not fit; it cancels, as Cancel does. */
  closeLabel?: string | undefined;
  /** The refusal's label (Cancel). */
  cancel: string;
  /** The confirmation's label, the verb of the title (Delete). */
  confirm: string;
  onCancel: () => void;
  /** The confirmation, given the text the reader typed where there is one to type (its edges trimmed), else "". */
  onConfirm: (entered: string) => void;
  /** The confirmed action in flight: the confirmation spins and the dialog stays. */
  busy?: boolean | undefined;
  /**
   * The text the reader types to unlock the confirmation, before what is grave as well as final (the account's handle
   * before a ban, the server's domain before closing it, the role's name before deleting it): the confirmation stays
   * disabled until the field holds it, its edges, full-width forms and case aside.
   */
  phrase?: string | undefined;
  /** Over the page, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
};

/**
 * Asks before an action that cannot be undone: the question and close, what follows, the field of the text to type
 * where there is one, then Cancel and the confirmation.
 */
export function ConfirmDialog({
  title,
  children,
  closeLabel,
  cancel,
  confirm: confirmLabel,
  onCancel,
  onConfirm,
  busy = false,
  phrase,
  layer = true,
}: ConfirmDialogProps) {
  const s = confirm();
  const { lang, direction } = useReader(undefined);
  const titleId = useId();
  const textId = useId();
  const fieldId = useId();
  const [entered, setEntered] = useState("");
  // Without a text to type, the confirmation is open from the start.
  const unlocked = phrase === undefined || folded(entered) === folded(phrase);
  // Over the page, the focus starts in the field of the text to type: the reader came to type it.
  const typing = phrase !== undefined;
  useEffect(() => {
    if (layer && typing) document.getElementById(fieldId)?.focus();
  }, [layer, typing, fieldId]);
  const body = (
    <>
      <DialogHead
        title={title}
        titleId={layer ? undefined : titleId}
        className={s.title()}
        closeLabel={closeLabel ?? stringsIn(lang).t("dialog.close")}
        onClose={onCancel}
        busy={busy}
      />
      <p id={textId} className={s.text()}>
        {children}
      </p>
      {phrase !== undefined && (
        <TextField
          id={fieldId}
          name="phrase"
          label={<PhraseLabel lang={lang} phrase={phrase} className={s.phrase()} />}
          autoComplete="off"
          autoCapitalize="none"
          onChange={setEntered}
        />
      )}
      <Row wrap align="center" justify="end" gap="dialog-row">
        <Button kind="quiet" size="form" onPress={onCancel} isDisabled={busy}>
          {cancel}
        </Button>
        {phrase === undefined ? (
          <Button
            kind="primary"
            size="form"
            busy={busy}
            onPress={() => {
              onConfirm("");
            }}
          >
            {confirmLabel}
          </Button>
        ) : (
          <Button kind="primary" size="form" busy={busy} isDisabled={!unlocked} type="submit">
            {confirmLabel}
          </Button>
        )}
      </Row>
    </>
  );
  // With a text to type, the dialog is a form, which Enter in the field sends once the field holds the text.
  const content =
    phrase === undefined ? (
      body
    ) : (
      <Stack
        gap="dialog-gap"
        as={
          <form
            onSubmit={(e) => {
              e.preventDefault();
              if (unlocked && !busy) onConfirm(entered.trim());
            }}
          />
        }
      >
        {body}
      </Stack>
    );
  // A specimen in a page stands as the modal's frame would place it: as wide as its column, a dialog's width at most.
  if (!layer)
    return (
      <Box
        anchor
        full
        max="dialog"
        role="alertdialog"
        aria-labelledby={titleId}
        aria-describedby={textId}
        className={`box-border ${s.modal()}`}
        dir={direction}
      >
        <Stack gap="dialog-gap" className={s.dialog()}>
          {content}
        </Stack>
      </Box>
    );
  return (
    // In flight, the dialog stays: Escape and a press outside cancel no more than Cancel, which is disabled.
    <ModalLayer onDismiss={busy ? () => undefined : onCancel} place="center" panelClassName={s.modal()}>
      <Stack
        gap="dialog-gap"
        as={<Dialog role="alertdialog" aria-describedby={textId} className={s.dialog()} dir={direction} />}
      >
        {content}
      </Stack>
    </ModalLayer>
  );
}

export type ReauthDialogProps = {
  /** What it asks (Confirm it's you; the act's question where the confirmation does it: Delete this account?). */
  title: string;
  /**
   * What the confirmation does once the proof holds, where it does an act at once (The account is deleted at once…),
   * in a sentence in the text's color, over `about`; none where the proof only lets the reader go on.
   */
  effect?: ReactNode;
  /** What the proof lets the reader do, in a sentence; with an `effect`, how to prove it. */
  about: ReactNode;
  /**
   * The field of the proof (a `TextField`: the password, a two-factor code), which takes the focus as the dialog
   * opens; none where the confirmation itself proves it (using a passkey, going to an external provider).
   */
  children?: ReactNode;
  /**
   * The other ways to prove it, under the field (`TextButton`s: use the password instead, continue with a provider):
   * one, or several as an array. The dialog gives each a row of its own, a finger's target tall, so that one is never
   * pressed for another.
   */
  other?: ReactNode;
  /** What the server refused, right before the pair (a `FormAlert`): read as it comes, and with the field. */
  error?: ReactNode;
  /** The close button's name; it cancels, as Cancel does. */
  closeLabel: string;
  cancel: string;
  /**
   * The confirmation's label, which says the way (Confirm, Use a passkey, Continue with GitHub) or, with an `effect`,
   * the act it does (Delete account); none where the reader has no way to prove it here, and the dialog only says so
   * (`error`): Cancel and close are then its ways out.
   */
  confirm?: string | undefined;
  onCancel: () => void;
  /** The confirmation's press, or Enter in the field of the proof; never while the proof is in flight. */
  onConfirm?: (() => void) | undefined;
  /** The proof in flight: the confirmation spins, nothing confirms again, and the dialog stays. */
  busy?: boolean | undefined;
  /** Over the page, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
};

/** The field of a proof inside `root`, the first it holds. */
const proofField = (root: HTMLElement | null) => root?.querySelector<HTMLElement>("input, textarea, select") ?? null;

/**
 * Asks the reader to authenticate again before something only they may do: what it lets them do, the proof, then
 * Cancel and the confirmation. It is a form, which the confirmation sends.
 */
export function ReauthDialog({
  title,
  effect,
  about,
  children,
  other,
  error,
  closeLabel,
  cancel,
  confirm: confirmLabel,
  onCancel,
  onConfirm,
  busy = false,
  layer = true,
}: ReauthDialogProps) {
  const s = confirm();
  const { direction } = useReader(undefined);
  const titleId = useId();
  const textId = useId();
  const errorId = useId();
  const effectId = useId();
  const form = useRef<HTMLFormElement>(null);
  const refused = error !== undefined && error !== null && error !== false;
  const acts = effect !== undefined && effect !== null && effect !== false;
  // The dialog is described by what it does, then by how to prove it.
  const describedBy = acts ? `${effectId} ${textId}` : textId;
  const ways = Children.toArray(other);
  // Over the page, the focus starts in the field of the proof, where there is one: the reader came to enter it.
  useEffect(() => {
    if (layer) proofField(form.current)?.focus();
  }, [layer]);
  // A proof refused: the focus goes back to the field, for another try.
  useEffect(() => {
    if (layer && refused && !busy) proofField(form.current)?.focus();
  }, [layer, refused, busy]);
  const body = (
    <Stack
      gap="dialog-gap"
      as={
        <form
          ref={form}
          onSubmit={(e) => {
            e.preventDefault();
            if (!busy) onConfirm?.();
          }}
        />
      }
    >
      <DialogHead
        title={title}
        titleId={layer ? undefined : titleId}
        className={s.title()}
        closeLabel={closeLabel}
        onClose={onCancel}
        busy={busy}
      />
      {acts && (
        <p id={effectId} className={s.effect()}>
          {effect}
        </p>
      )}
      <p id={textId} className={s.text()}>
        {about}
      </p>
      {/* What the server refused describes the field of the proof. */}
      <FieldContext value={refused ? { describedBy: errorId } : {}}>{children}</FieldContext>
      {ways.length > 0 && (
        // Each way a row of its own, a finger's target tall: a press meant for one never lands on the next.
        <Stack as="ul" className={s.ways()}>
          {ways.map((way, i) => (
            <Row
              as="li"
              key={isValidElement(way) && way.key !== null ? way.key : i}
              align="center"
              justify="start"
              className={s.way()}
            >
              {way}
            </Row>
          ))}
        </Stack>
      )}
      {refused && <div id={errorId}>{error}</div>}
      <Row wrap align="center" justify="end" gap="dialog-row">
        <Button kind="quiet" size="form" onPress={onCancel} isDisabled={busy}>
          {cancel}
        </Button>
        {confirmLabel !== undefined && (
          <Button kind="primary" size="form" busy={busy} type="submit">
            {confirmLabel}
          </Button>
        )}
      </Row>
    </Stack>
  );
  if (!layer)
    return (
      <Box
        anchor
        full
        max="dialog"
        role="dialog"
        aria-labelledby={titleId}
        aria-describedby={describedBy}
        className={`box-border ${s.modal()} ${s.dialog()}`}
        dir={direction}
      >
        {body}
      </Box>
    );
  return (
    <ModalLayer onDismiss={busy ? () => undefined : onCancel} place="center" panelClassName={s.modal()}>
      <Dialog aria-describedby={describedBy} className={s.dialog()} dir={direction}>
        {body}
      </Dialog>
    </ModalLayer>
  );
}

const panel = tv({ slots: { title: "text-title" } });

/**
 * The frame of a panel: a column, its head over its body, and its foot under it where it has one. Under `tablet` it is
 * a screen of its own, as tall as the viewport, its body scrolling between its head and its foot; from `tablet` a
 * dialog of the main column's width on its own surface, as tall as what it holds: without a foot the modal's overlay
 * scrolls it, with one it is at most as tall as the room the overlay leaves, and its body scrolls between the head and
 * the foot, so that the foot's actions stay in sight. A `specimen` in a page is as tall as what it holds at every
 * width. One of the frames of a page: it reads the viewport.
 */
function PanelFrame({
  as,
  specimen,
  head,
  foot,
  children,
}: {
  /** The dialog's element. */
  as: ReactElement<{ className?: unknown }>;
  specimen: boolean;
  head: ReactNode;
  foot?: ReactNode;
  children: ReactNode;
}) {
  const footed = foot !== undefined && foot !== null;
  const frame = [
    "relative box-border flex h-dvh flex-col bg-surface font-sans text-body text-text outline-hidden",
    "tablet:mx-auto tablet:h-auto tablet:w-[min(var(--shell-column),100%)] tablet:rounded-overlay",
    "tablet:shadow-overlay",
    footed && !specimen ? "tablet:max-h-[calc(100dvh-var(--composer-top)-var(--composer-bottom))]" : "",
  ].join(" ");
  const body = footed
    ? "min-h-0 flex-1 overflow-y-auto px-shell-edge pb-entry tablet:px-dialog"
    : "min-h-0 flex-1 overflow-y-auto px-shell-edge pb-entry tablet:overflow-visible tablet:px-dialog tablet:pb-dialog";
  return (
    <Box as={as} className={specimen ? `${frame} h-auto` : frame}>
      <div className="flex items-center justify-between gap-entry-caption px-shell-edge pt-entry-line pb-entry tablet:px-dialog">
        {head}
      </div>
      <div className={body}>{children}</div>
      {footed && <div className="px-shell-edge pt-entry pb-entry tablet:px-dialog tablet:pb-dialog">{foot}</div>}
    </Box>
  );
}

export type PanelDialogProps = {
  title: string;
  /** The close button's name. */
  closeLabel: string;
  /** Close, Escape or a press outside. */
  onClose: () => void;
  /** Over the page, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
  /**
   * What stays in sight under what scrolls (a form's last choice, its alert and its buttons): at the screen's bottom on
   * a phone, at the dialog's from `tablet`.
   */
  foot?: ReactNode;
  children: ReactNode;
};

/**
 * Something to read over the page: its title and close at the end of its head, then what it shows, which scrolls, and
 * what stays under it (`foot`).
 */
export function PanelDialog({ title, closeLabel, onClose, layer = true, foot, children }: PanelDialogProps) {
  const s = panel();
  const { direction } = useReader(undefined);
  const titleId = useId();
  const head = (
    <>
      {layer ? (
        <Heading slot="title" className={s.title()}>
          {titleBreaks(title)}
        </Heading>
      ) : (
        <h2 id={titleId} className={s.title()}>
          {titleBreaks(title)}
        </h2>
      )}
      <Row as="span" inline bleedEnd="icon-button-inline">
        <IconButton icon="close" label={closeLabel} onPress={onClose} />
      </Row>
    </>
  );
  if (!layer)
    return (
      <PanelFrame specimen head={head} foot={foot} as={<div role="dialog" aria-labelledby={titleId} dir={direction} />}>
        {children}
      </PanelFrame>
    );
  return (
    <ModalLayer onDismiss={onClose} backdrop="tablet" place="screen">
      <PanelFrame specimen={false} head={head} foot={foot} as={<Dialog dir={direction} />}>
        {children}
      </PanelFrame>
    </ModalLayer>
  );
}
