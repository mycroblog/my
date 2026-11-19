// SPDX-License-Identifier: AGPL-3.0-only

// What the host and the co-hosts of a voice chat room do in it (ADR-0100 §5, §6; FR-925, FR-926):
//
// - On a participant, a menu opened by their tile: let them speak (also the answer to a raised hand) or dismiss the
//   hand, stop them speaking, mute a speaker, remove them (asked first); and for the host alone, make or unmake a
//   co-host (3 at most) and hand the room over (asked first). By §5's rules: nobody acts on the host or on themselves,
//   and the host and the co-hosts do not remove each other; a tile the reader can do nothing with is no control.
// - The room's own menu, in its head: its settings and ending it for the host, its invitations for both.
// - The host leaving a room with others in it: who hosts next (any other participant; the server refuses one who may
//   not host, and the dialog says so) or ending the room, in a dialog; a host alone in it leaves, and the room ends.

import { nameOf } from "@my/app-components";
import { isVoiceLeaveRefused, type VoiceHostOperation, type VoiceRoom, type VoiceRoomParticipant } from "@my/app-data";
import {
  Box,
  Button,
  ChoiceList,
  ConfirmDialog,
  FormAlert,
  MenuGroup,
  MenuItem,
  MenuSurface,
  PanelDialog,
  ParticipantTile,
  Row,
  Stack,
  type ParticipantTileProps,
} from "@my/design-system";
import { ProblemError } from "@mycroblog/mykit";
import { useState } from "react";
import type { stringsIn } from "../locale";
import type { VoiceCall } from "./voice-call";

type T = ReturnType<typeof stringsIn>["t"];

/** The most co-hosts a room has (ADR-0100 §5). */
export const cohostLimit = 3;

/** Whether the reader, as the host or a co-host of the room, may act on `target` (ADR-0100 §5). */
export function mayActOn(self: VoiceRoomParticipant | undefined, target: VoiceRoomParticipant): boolean {
  if (self === undefined || !(self.hosting || self.cohosting)) return false;
  return target.account.id !== self.account.id && !target.hosting;
}

/** What the reader may do to `target`, in the menu's order; nothing for a participant they may not act on. */
export function hostOperations(
  self: VoiceRoomParticipant | undefined,
  target: VoiceRoomParticipant,
): readonly VoiceHostOperation[] {
  if (self === undefined || !mayActOn(self, target)) return [];
  const ops: VoiceHostOperation[] =
    target.role === "speaker" ? ["revoke", "mute"] : target.hand_raised ? ["grant", "dismiss"] : ["grant"];
  if (self.hosting) ops.push(target.cohosting ? "uncohost" : "cohost", "hand-over");
  // The host and the co-hosts do not remove each other.
  if (!target.cohosting) ops.push("remove");
  return ops;
}

/** The glyph of each operation in the menu. */
const opIcons = {
  grant: "microphone",
  dismiss: "hand",
  revoke: "hear",
  mute: "mute",
  remove: "leave",
  cohost: "members",
  uncohost: "members",
  "hand-over": "moved",
} as const;

/** A press of the host's that did not finish: its operation and who it was on, as `voice.host.failed` says it. */
export type HostFailure = { readonly op: VoiceHostOperation; readonly name: string; readonly cause: string };

/** Why a host's operation was refused, for its sentence: too many co-hosts, a next host who may not, or another. */
function causeOf(error: unknown): string {
  if (!(error instanceof ProblemError)) return "other";
  if (error.problem.type === "/api/v1/problems/voice-too-many-cohosts") return "cohosts";
  if (error.problem.type === "/api/v1/problems/voice-invalid-next-host") return "next_host";
  return "other";
}

/** The key of an operation in the catalog's selects (`hand-over` is `hand_over`). */
export const opKey = (op: VoiceHostOperation): string => op.replace("-", "_");

/** An operation that ends something for its participant, which the host confirms first. */
export type AskedOperation = "remove" | "hand-over";

/**
 * A participant's tile, and when the reader may act on them, the menu it opens: what they may do; the ones that end
 * something (remove, hand over) are asked first by the room (`onAsk`), whose dialog outlives the menu and the tile.
 * `onFailed` says what did not finish.
 */
export function ParticipantMenu({
  call,
  room,
  self,
  target,
  tile,
  onAsk,
  onFailed,
  t,
}: {
  call: VoiceCall;
  room: VoiceRoom;
  self: VoiceRoomParticipant | undefined;
  target: VoiceRoomParticipant;
  tile: Omit<ParticipantTileProps, "onPress">;
  onAsk: (op: AskedOperation, target: VoiceRoomParticipant) => void;
  onFailed: (failure: HostFailure | null) => void;
  t: T;
}) {
  const [open, setOpen] = useState(false);
  const ops = hostOperations(self, target);
  if (ops.length === 0) return <ParticipantTile {...tile} />;
  const name = nameOf(target.account);
  const cohosts = room.participants.filter((p) => p.cohosting).length;
  const run = (op: VoiceHostOperation) => {
    onFailed(null);
    call.host(target.account.id, op).catch((error: unknown) => {
      onFailed({ op, name, cause: causeOf(error) });
    });
  };
  const item = (op: VoiceHostOperation) => {
    const full = op === "cohost" && cohosts >= cohostLimit;
    return (
      <MenuItem
        key={op}
        id={op}
        icon={opIcons[op]}
        disabled={full}
        about={full ? t("voice.host.cohosts_full", { count: cohostLimit }) : undefined}
        onAction={() => {
          if (op === "remove" || op === "hand-over") onAsk(op, target);
          else run(op);
        }}
      >
        {t("voice.host.op", { op: opKey(op) })}
      </MenuItem>
    );
  };
  const ending = ops.filter((op) => op === "remove");
  return (
    <MenuSurface
      trigger={
        <ParticipantTile
          {...tile}
          onPress={() => {
            setOpen(true);
          }}
        />
      }
      label={t("voice.host.menu", { name })}
      placement="bottom start"
      open={open}
      onOpenChange={setOpen}
    >
      <MenuGroup>{ops.filter((op) => op !== "remove").map(item)}</MenuGroup>
      {ending.length > 0 && <MenuGroup>{ending.map(item)}</MenuGroup>}
    </MenuSurface>
  );
}

/**
 * The confirmation of an operation that ends something for `target` (removing them, handing the room over), held by
 * the room so that it outlives the tile it was asked from: `onDone` says whether it was done, cancelled, or why not.
 */
export function HostAskDialog({
  call,
  op,
  target,
  onDone,
  t,
}: {
  call: VoiceCall;
  op: AskedOperation;
  target: VoiceRoomParticipant;
  /** It was done, cancelled, or did not finish (why). */
  onDone: (outcome: "done" | "cancelled" | HostFailure) => void;
  t: T;
}) {
  const [busy, setBusy] = useState(false);
  const name = nameOf(target.account);
  const key = opKey(op);
  return (
    <ConfirmDialog
      title={t("voice.host.ask_title", { op: key, name })}
      cancel={t("voice.host.cancel")}
      confirm={t("voice.host.ask_confirm", { op: key })}
      busy={busy}
      onCancel={() => {
        if (!busy) onDone("cancelled");
      }}
      onConfirm={() => {
        setBusy(true);
        call.host(target.account.id, op).then(
          () => {
            onDone("done");
          },
          (error: unknown) => {
            onDone({ op, name, cause: causeOf(error) });
          },
        );
      }}
    >
      {t("voice.host.ask_text", { op: key, name })}
    </ConfirmDialog>
  );
}

/** The room's menu in its head: its settings and ending it for the host, its invitations for the host and co-hosts. */
export function RoomMenu({
  self,
  settingsHref,
  invitationsHref,
  onEnd,
  open,
  onOpenChange,
  t,
}: {
  self: VoiceRoomParticipant | undefined;
  settingsHref: string;
  invitationsHref: string;
  onEnd: () => void;
  /** The menu is open (a story's), with `onOpenChange`; else its button alone opens it. */
  open?: boolean | undefined;
  onOpenChange?: ((open: boolean) => void) | undefined;
  t: T;
}) {
  if (self === undefined || !(self.hosting || self.cohosting)) return null;
  return (
    // A button that names itself, a sheet on a phone (the app's MenuSheet), a popover from a tablet.
    <MenuSurface
      trigger={
        <Button size="timeline" touch>
          {t("voice.host.room_menu")}
        </Button>
      }
      label={t("voice.host.room_menu")}
      placement="bottom end"
      open={open}
      onOpenChange={onOpenChange}
    >
      <MenuGroup>
        {self.hosting && (
          <MenuItem id="settings" icon="settings" href={settingsHref}>
            {t("voice.settings.title")}
          </MenuItem>
        )}
        <MenuItem id="invitations" icon="invite" href={invitationsHref}>
          {t("voice.invitations.title")}
        </MenuItem>
      </MenuGroup>
      {self.hosting && (
        <MenuGroup>
          <MenuItem id="end" icon="leave" onAction={onEnd}>
            {t("voice.host.end")}
          </MenuItem>
        </MenuGroup>
      )}
    </MenuSurface>
  );
}

/**
 * Ending the room, asked first: everyone in it leaves. `onEnding` runs as the reader confirms (the page then moves the
 * focus where the room was), `onFailed` when it did not end.
 */
export function EndRoomDialog({
  end,
  onEnding,
  onClose,
  onEnded,
  onFailed,
  t,
}: {
  /** Ends the room: the call's `endRoom` while the reader is in it. */
  end: () => Promise<void>;
  onEnding: () => void;
  onClose: () => void;
  /** It ended. */
  onEnded?: (() => void) | undefined;
  onFailed: () => void;
  t: T;
}) {
  const [busy, setBusy] = useState(false);
  return (
    <ConfirmDialog
      title={t("voice.host.end_title")}
      cancel={t("voice.host.cancel")}
      confirm={t("voice.end_room")}
      busy={busy}
      onCancel={() => {
        if (!busy) onClose();
      }}
      onConfirm={() => {
        setBusy(true);
        onEnding();
        end().then(
          () => {
            onClose();
            onEnded?.();
          },
          () => {
            setBusy(false);
            onClose();
            onFailed();
          },
        );
      }}
    >
      {t("voice.host.end_text")}
    </ConfirmDialog>
  );
}

/** Whether the reader hosts a room with others in it: leaving asks who hosts next, or to end it. */
export function hostsOthers(room: VoiceRoom, self: VoiceRoomParticipant | undefined): boolean {
  return self?.hosting === true && room.participants.some((p) => p.account.id !== self.account.id);
}

/**
 * The host's leave of a room with others in it: who hosts next, the co-hosts first, or ending the room, then Leave.
 * The server refuses a next host who may not host, which the dialog says, the reader choosing again.
 */
export function LeaveDialog({
  call,
  room,
  self,
  onLeft,
  onClose,
  t,
}: {
  call: VoiceCall;
  room: VoiceRoom;
  self: VoiceRoomParticipant;
  /** Runs as the leave is sent: the page then moves the focus where the room was. */
  onLeft: () => void;
  onClose: () => void;
  t: T;
}) {
  const others = room.participants.filter((p) => p.account.id !== self.account.id);
  const ordered = [...others.filter((p) => p.cohosting), ...others.filter((p) => !p.cohosting)];
  // The choice is whom it names, or ending the room, never a place in the list, which a join or a leave reorders.
  const [chosen, setChosen] = useState<{ id: string; name: string } | "end" | null>(() => {
    const first = ordered[0];
    return first === undefined ? "end" : { id: first.account.id, name: nameOf(first.account) };
  });
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState<string | null>(null);
  const next = chosen === null || chosen === "end" ? undefined : ordered.find((p) => p.account.id === chosen.id);
  // The one chosen left the room meanwhile: nothing is chosen, and the dialog says why.
  const gone = chosen !== null && chosen !== "end" && next === undefined ? chosen.name : null;
  const items = [
    ...ordered.map((p) => ({
      key: p.account.id,
      label: t("voice.leave_dialog.next", { name: nameOf(p.account), cohost: p.cohosting ? "yes" : "no" }),
    })),
    { key: "end", label: t("voice.leave_dialog.end") },
  ];
  const at = chosen === "end" ? ordered.length : next === undefined ? -1 : ordered.indexOf(next);
  const leave = () => {
    if (busy || chosen === null || gone !== null) return;
    setBusy(true);
    setRefused(null);
    onLeft();
    call.leave(next === undefined ? { end: true } : { nextHostId: next.account.id }).then(onClose, (error: unknown) => {
      setBusy(false);
      setRefused(
        isVoiceLeaveRefused(error) && next !== undefined
          ? t("voice.leave_dialog.refused", { name: nameOf(next.account) })
          : t("voice.leave_dialog.failed"),
      );
    });
  };
  return (
    <PanelDialog
      title={t("voice.leave_dialog.title")}
      closeLabel={t("voice.host.cancel")}
      onClose={() => {
        if (!busy) onClose();
      }}
    >
      <Stack gap="entry-room">
        <Box as="p">{t("voice.leave_dialog.text")}</Box>
        <ChoiceList
          label={t("voice.leave_dialog.title")}
          multiple={false}
          items={items}
          chosen={at < 0 ? [] : [at]}
          disabled={busy}
          onChange={([i]) => {
            const p = ordered[i ?? -1];
            setChosen(p === undefined ? "end" : { id: p.account.id, name: nameOf(p.account) });
            setRefused(null);
          }}
        />
        {gone !== null && <FormAlert regular>{t("voice.leave_dialog.gone", { name: gone })}</FormAlert>}
        {refused !== null && <FormAlert regular>{refused}</FormAlert>}
        <Row wrap align="center" justify="end" gap="dialog-row">
          <Button kind="quiet" size="form" isDisabled={busy} onPress={onClose}>
            {t("voice.host.cancel")}
          </Button>
          <Button kind="primary" size="form" busy={busy} isDisabled={chosen === null || gone !== null} onPress={leave}>
            {t(chosen === "end" ? "voice.leave_dialog.end_confirm" : "voice.leave_dialog.confirm")}
          </Button>
        </Row>
      </Stack>
    </PanelDialog>
  );
}
