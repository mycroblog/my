// SPDX-License-Identifier: AGPL-3.0-only

// What a report's screen offers beyond the report (FR-703, FR-931): the way to act on the reported account, its own
// moderation screen opened from the report, for a holder of `manage_accounts` alone; and, for the moderator of reports,
// ending the open voice chat room a report points at for everyone and cancelling the event, each asked first with an
// optional reason (the room's for the moderators alone, the event's shown with its cancellation).

import { moderatedAccountPath } from "@my/app-components";
import { placeReasonLimit, usePermissions, usePlaceWrites } from "@my/app-data";
import { Button, ConfirmDialog, LinkButton, ModalPresence, Row, Stack, TextField } from "@my/design-system";
import { useId, useState } from "react";
import { staffHref } from "../entries";
import { useStaffStrings } from "../parts";
import { refusalWords, useSaid } from "./parts";

/** Act on this account: its moderation screen, from the report of `report`; nothing without `manage_accounts`. */
export function ActOnAccount({ account, report }: { account: string; report: string }) {
  const { t } = useStaffStrings();
  if (!usePermissions().includes("manage_accounts")) return null;
  return (
    <LinkButton href={staffHref(moderatedAccountPath(account), { report })} size="timeline">
      {t("staff_accounts.report.act")}
    </LinkButton>
  );
}

/** End a place for everyone or cancel it, asked first, with a reason; and what came of it. */
function EndPlace({
  kind,
  id,
}: {
  /** A voice chat room (`room`), or an event (`event`). */
  kind: "room" | "event";
  id: string;
}) {
  const { t } = useStaffStrings();
  const writes = usePlaceWrites();
  const reasonId = useId();
  const { say, status } = useSaid();
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [reason, setReason] = useState("");
  const send = () => {
    setBusy(true);
    (kind === "room" ? writes.endRoom(id, reason.trim()) : writes.cancelEvent(id, reason.trim())).then(
      () => {
        setBusy(false);
        setAsking(false);
        setDone(true);
        say(t("staff_accounts.place.done", { kind }));
      },
      (e: unknown) => {
        setBusy(false);
        setAsking(false);
        say(refusalWords(t, e), true);
      },
    );
  };
  return (
    <Stack gap="entry-line">
      {!done && (
        <Row>
          <Button
            size="timeline"
            onPress={() => {
              setAsking(true);
            }}
          >
            {t("staff_accounts.place.end", { kind })}
          </Button>
        </Row>
      )}
      {status}
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            title={t("staff_accounts.place.confirm", { kind })}
            cancel={t("staff_accounts.cancel")}
            confirm={t("staff_accounts.place.end", { kind })}
            busy={busy}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={send}
          >
            <TextField
              id={reasonId}
              name="reason"
              label={t("staff_accounts.place.reason")}
              hint={t("staff_accounts.place.reason.hint", { kind })}
              multiline
              rows={2}
              maxLength={placeReasonLimit}
              onChange={setReason}
            />
          </ConfirmDialog>
        )}
      </ModalPresence>
    </Stack>
  );
}

/** End the open voice chat room of `id` for everyone. */
export const EndRoom = ({ id }: { id: string }) => <EndPlace kind="room" id={id} />;

/** Cancel the event of `id`. */
export const CancelEvent = ({ id }: { id: string }) => <EndPlace kind="event" id={id} />;
