// SPDX-License-Identifier: AGPL-3.0-only

// Two modals over the page (modal.tsx), every word the caller's. The confirmation asks before what cannot be undone
// (deleting a post): a small dialog in the middle of the viewport at every width, its title the question, one or two
// sentences on what follows, and the pair of Cancel and the confirmation side by side at its end, the confirmation at
// the end side; Escape and a press outside cancel. The panel shows something to read (a post's edit history) as the
// composer does: a dialog of the main column's width over the dimmed page from `tablet`, a screen of its own on a
// phone; its head holds the title and close at its end, and its body scrolls under the head.

import { useId, type ReactNode } from "react";
import { Dialog, Heading } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Button } from "./button";
import { IconButton } from "./icon-button";
import { useReader } from "./locale";
import { ModalLayer } from "./modal";

const confirm = tv({
  slots: {
    overlay: "fixed inset-0 z-50 box-border flex items-center justify-center p-dialog-margin",
    modal: [
      "relative box-border w-full max-w-[var(--dialog-max-width)] rounded-overlay bg-surface p-dialog",
      "font-sans text-body text-text shadow-overlay outline-hidden",
    ],
    dialog: "grid gap-dialog-gap outline-hidden",
    // Wrapping between phrases, never inside a word of ja.
    title: "m-0 text-title [word-break:auto-phrase]",
    text: "m-0 text-muted-text [word-break:auto-phrase]",
    row: "flex flex-wrap items-center justify-end gap-dialog-row",
  },
});

export type ConfirmDialogProps = {
  /** The question (Delete this post?). */
  title: string;
  /** What follows the confirmation, in a sentence or two. */
  children: ReactNode;
  /** The refusal's label (Cancel). */
  cancel: string;
  /** The confirmation's label, the verb of the title (Delete). */
  confirm: string;
  onCancel: () => void;
  onConfirm: () => void;
  /** The confirmed action in flight: the confirmation spins and the dialog stays. */
  busy?: boolean | undefined;
  /** Over the page, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
};

/** Asks before an action that cannot be undone: the question, what follows, then Cancel and the confirmation. */
export function ConfirmDialog({
  title,
  children,
  cancel,
  confirm: confirmLabel,
  onCancel,
  onConfirm,
  busy = false,
  layer = true,
}: ConfirmDialogProps) {
  const s = confirm();
  const { direction } = useReader(undefined);
  const titleId = useId();
  const body = (
    <>
      {layer ? (
        <Heading slot="title" className={s.title()}>
          {title}
        </Heading>
      ) : (
        <h2 id={titleId} className={s.title()}>
          {title}
        </h2>
      )}
      <p className={s.text()}>{children}</p>
      <div className={s.row()}>
        <Button kind="quiet" size="form" onPress={onCancel} isDisabled={busy}>
          {cancel}
        </Button>
        <Button kind="primary" size="form" busy={busy} onPress={onConfirm}>
          {confirmLabel}
        </Button>
      </div>
    </>
  );
  if (!layer)
    return (
      <div role="alertdialog" aria-labelledby={titleId} className={s.modal()} dir={direction}>
        <div className={s.dialog()}>{body}</div>
      </div>
    );
  return (
    <ModalLayer onDismiss={onCancel} overlay={s.overlay()} panel={s.modal()}>
      <Dialog role="alertdialog" className={s.dialog()} dir={direction}>
        {body}
      </Dialog>
    </ModalLayer>
  );
}

const panel = tv({
  slots: {
    overlay: [
      "fixed inset-0 z-50 box-border grid items-start overflow-y-auto",
      "tablet:justify-items-center tablet:px-dialog-margin tablet:pt-composer-top tablet:pb-composer-bottom",
    ],
    rise: "box-border w-full",
    frame: [
      "relative box-border flex h-dvh flex-col bg-surface font-sans text-body text-text outline-hidden",
      "tablet:mx-auto tablet:h-auto tablet:w-[min(var(--shell-column),100%)] tablet:rounded-overlay",
      "tablet:shadow-overlay",
    ],
    head: "flex items-center justify-between gap-entry-caption px-shell-edge pt-entry-line pb-entry tablet:px-dialog",
    close: "-me-icon-button-inline inline-flex",
    title: "m-0 text-title",
    body: "min-h-0 flex-1 overflow-y-auto px-shell-edge pb-entry tablet:overflow-visible tablet:px-dialog tablet:pb-dialog",
  },
  variants: {
    // A specimen in a page is as tall as what it holds.
    layer: { false: { frame: "h-auto" } },
  },
});

export type PanelDialogProps = {
  title: string;
  /** The close button's name. */
  closeLabel: string;
  /** Close, Escape or a press outside. */
  onClose: () => void;
  /** Over the page, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
  children: ReactNode;
};

/** Something to read over the page: its title and close at the end of its head, then what it shows, which scrolls. */
export function PanelDialog({ title, closeLabel, onClose, layer = true, children }: PanelDialogProps) {
  const s = panel({ layer });
  const { direction } = useReader(undefined);
  const titleId = useId();
  const head = (
    <div className={s.head()}>
      {layer ? (
        <Heading slot="title" className={s.title()}>
          {title}
        </Heading>
      ) : (
        <h2 id={titleId} className={s.title()}>
          {title}
        </h2>
      )}
      <span className={s.close()}>
        <IconButton icon="close" label={closeLabel} onPress={onClose} />
      </span>
    </div>
  );
  if (!layer)
    return (
      <div role="dialog" aria-labelledby={titleId} className={s.frame()} dir={direction}>
        {head}
        <div className={s.body()}>{children}</div>
      </div>
    );
  return (
    <ModalLayer onDismiss={onClose} backdrop="tablet" overlay={s.overlay()} panel={s.rise()}>
      <Dialog className={s.frame()} dir={direction}>
        {head}
        <div className={s.body()}>{children}</div>
      </Dialog>
    </ModalLayer>
  );
}
