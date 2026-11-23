// SPDX-License-Identifier: AGPL-3.0-only

// What the organizers do with an event from its page (ADR-0161 §4, FR-940, FR-944): its menu in the page's head,
// "Manage the event", for the organizer and the co-organizers of an event that was not cancelled: changing it, its
// invitations, and for the organizer alone its co-organizers and cancelling it, asked first with a note the attendees
// read. The menu is a sheet on a phone (the app's MenuSheet) and a popover from a tablet, as a voice room's is. Another
// reader has More there, which reports the event and its organizer (report.tsx); a co-organizer, the menu's last group.

import { eventCoorganizersPath, eventEditPath, eventInvitationsPath } from "@my/app-components";
import { eventFaultField, eventLimits, eventRefusal, type EventWrites, type ServerEvent } from "@my/app-data";
import {
  Box,
  Button,
  focusRefusedField,
  FormAlert,
  MenuButton,
  MenuGroup,
  MenuItem,
  MenuSurface,
  PanelDialog,
  Row,
  Stack,
  TextField,
} from "@my/design-system";
import { useEffect, useId, useRef, useState } from "react";
import type { T } from "./parts";

/** Whether the reader may manage `event` from its menu: they organize it, and it was not cancelled. */
const managesEvent = (event: ServerEvent) =>
  (event.viewer.organizer || event.viewer.coorganizer) && event.state !== "canceled";

/**
 * The event's menu in its page's head, for its organizers: Change the event, Invitations, and for the organizer
 * Co-organizers and Cancel the event…, last and apart, and for a co-organizer Report the event…; for another reader
 * More, which holds Report the event… alone (`onReport`); nothing for its organizer once it was cancelled.
 */
export function EventMenu({
  event,
  onCancel,
  onReport,
  open,
  onOpenChange,
  t,
}: {
  event: ServerEvent;
  /** The organizer chose Cancel the event…: the page asks. */
  onCancel: () => void;
  /** Opens the report of the event; none for its organizer, who does not report their own event. */
  onReport?: (() => void) | undefined;
  /** The menu is open (a story's), with `onOpenChange`; else its button alone opens it. */
  open?: boolean | undefined;
  onOpenChange?: ((open: boolean) => void) | undefined;
  t: T;
}) {
  const report =
    onReport !== undefined && !event.viewer.organizer ? (
      <MenuGroup>
        <MenuItem id="report" icon="report" onAction={onReport}>
          {t("report.menu_event")}
        </MenuItem>
      </MenuGroup>
    ) : null;
  if (!managesEvent(event)) {
    if (report === null) return null;
    return (
      <MenuButton icon="more" label={t("report.more")} open={open} onOpenChange={onOpenChange}>
        {report}
      </MenuButton>
    );
  }
  const organizer = event.viewer.organizer;
  return (
    <MenuSurface
      trigger={<Button size="target">{t("events.manage")}</Button>}
      label={t("events.manage")}
      placement="bottom end"
      open={open}
      onOpenChange={onOpenChange}
    >
      <MenuGroup>
        <MenuItem id="edit" icon="edit" href={eventEditPath(event.id)}>
          {t("events.edit")}
        </MenuItem>
        <MenuItem id="invitations" icon="invite" href={eventInvitationsPath(event.id)}>
          {t("events.invitations")}
        </MenuItem>
        {organizer && (
          <MenuItem id="coorganizers" icon="members" href={eventCoorganizersPath(event.id)}>
            {t("events.coorganizers")}
          </MenuItem>
        )}
      </MenuGroup>
      {organizer && (
        <MenuGroup>
          <MenuItem id="cancel" icon="eventCancelled" onAction={onCancel}>
            {t("events.cancel_menu")}
          </MenuItem>
        </MenuGroup>
      )}
      {report}
    </MenuSurface>
  );
}

/**
 * Runs `gone` once the component that calls it is out of the document: from the cleanup of its effect, which React
 * runs after it took the component's nodes out, never from the presence's exit, while the dialog still holds the focus.
 */
export function useGone(gone: () => void): void {
  const latest = useRef(gone);
  useEffect(() => {
    latest.current = gone;
  });
  useEffect(
    () => () => {
      latest.current();
    },
    [],
  );
}

/** How many code points `text` holds, as the server counts a note. */
const codePoints = (text: string): number => Array.from(text).length;

/**
 * Cancelling the event, asked first: what it does, the note the attendees read with it (none is needed), then Keep
 * the event and Cancel the event. `onCancelled` runs once the server cancelled it, `onClosed` once it says the event
 * was cancelled already (the page reads it again and says so), `onClose` as the dialog closes otherwise; a note the
 * server refuses is said under it, and what did not finish in the dialog, which stays.
 */
export function CancelDialog({
  writes,
  onCancelled,
  onClosed,
  onClose,
  onGone,
  t,
}: {
  writes: EventWrites;
  onCancelled: () => void;
  onClosed: () => void;
  onClose: () => void;
  /** The dialog is out of the document: what it changed may take the focus. */
  onGone: () => void;
  t: T;
}) {
  const id = useId();
  useGone(onGone);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState<string | null>(null);
  // The server refused the note as it was sent, until the reader changes it.
  const [noteRefused, setNoteRefused] = useState(false);
  const tooLong = codePoints(note) > eventLimits.cancelNote;
  const noteId = `${id}-note`;
  const cancel = () => {
    if (busy) return;
    if (tooLong) return;
    setBusy(true);
    setFailed(null);
    writes.cancel(note.trim() === "" ? undefined : note).then(onCancelled, (error: unknown) => {
      setBusy(false);
      const refusal = eventRefusal(error);
      if (refusal === "closed") onClosed();
      else if (refusal === "invalid" && eventFaultField(error) === "note") {
        setNoteRefused(true);
        focusRefusedField(noteId);
      } else setFailed(t("events.cancel.failed"));
    });
  };
  return (
    <PanelDialog
      title={t("events.cancel.title")}
      closeLabel={t("events.cancel.keep")}
      onClose={() => {
        if (!busy) onClose();
      }}
    >
      <Stack gap="entry-room">
        <Box as="p">{t("events.cancel.text")}</Box>
        <TextField
          id={noteId}
          name="note"
          label={t("events.cancel.note")}
          hint={tooLong || noteRefused ? undefined : t("events.cancel.note_hint", { max: eventLimits.cancelNote })}
          error={tooLong || noteRefused ? t("events.cancel.note_rule", { max: eventLimits.cancelNote }) : undefined}
          multiline
          rows={3}
          onChange={(text) => {
            setNote(text);
            setNoteRefused(false);
          }}
        />
        {failed !== null && <FormAlert regular>{failed}</FormAlert>}
        <Row wrap align="center" justify="end" gap="dialog-row">
          <Button kind="quiet" size="form" isDisabled={busy} onPress={onClose}>
            {t("events.cancel.keep")}
          </Button>
          <Button kind="primary" size="form" busy={busy} isDisabled={tooLong} onPress={cancel}>
            {t("events.cancel.confirm")}
          </Button>
        </Row>
      </Stack>
    </PanelDialog>
  );
}
