// SPDX-License-Identifier: AGPL-3.0-only

// Why the reader's call ended, on every screen but the room's own, which says it in its place (room.tsx): a toast in
// the shell's dock, until it is closed, leads to the room or the room's page said it (ADR-0100 §5, §6; a host removed the reader, they joined on
// another device, the room ended, the connection was lost).

import { voiceRoomPath } from "@my/app-components";
import { Toast, useLocale } from "@my/design-system";
import { stringsIn } from "../locale";
import { endSentence } from "./room";
import { dismissRoomEnd, useRoomEnd } from "./room-end";

/** The toast of the call that ended, unless the reader is on its room's page (`pathname`); `onOpen` opens the room. */
export function CallEnds({ pathname, onOpen }: { pathname: string; onOpen: (href: string) => void }) {
  const { t } = stringsIn(useLocale().locale);
  const end = useRoomEnd();
  if (end === null || end.seen) return null;
  const href = voiceRoomPath(end.roomId);
  if (pathname === href) return null;
  return (
    <Toast
      action={t("voice.end_open")}
      onAction={() => {
        // The room's page says it again, in its place.
        onOpen(href);
      }}
      closeLabel={t("voice.end_close")}
      onClose={dismissRoomEnd}
    >
      {endSentence(end, t)}
    </Toast>
  );
}
