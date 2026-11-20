// SPDX-License-Identifier: AGPL-3.0-only

// Voice chat, the entry of the settings at /settings/voice while the server has voice chat (ADR-0100 §13): the rooms
// the reader hosted, from opening them or from taking them over, newest first, each by its name, when it opened and
// when and why it ended; a room still open leads to its page. Show more reads the next page.

import { voiceRoomPath } from "@my/app-components";
import { useVoiceHistory, voiceHistoryQuery, type AppData, type VoiceRoomHistoryEntry } from "@my/app-data";
import {
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsScreen,
  SettingsSection,
  ShowMore,
  useAddedFocus,
  useLocale,
  type AddedFocus,
} from "@my/design-system";
import { stringsIn } from "../locale";
import type { SettingsEntryProps } from "./entries";
import { ReadingNotice, useDates } from "./parts";

type T = ReturnType<typeof stringsIn>["t"];

/** What the route's loader waits for: the first page of the rooms, from the cache when it has it. */
export async function loadVoiceSettings(data: AppData): Promise<void> {
  await data.queryClient.infiniteQuery({ ...voiceHistoryQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** A room the reader hosted: its name, when it opened, and when and why it ended, or that it is open. */
function Hosted({ room, itemRef, t }: { room: VoiceRoomHistoryEntry; itemRef: ReturnType<AddedFocus["item"]>; t: T }) {
  const { hour } = useDates();
  const open = room.ended_at === undefined;
  return (
    // The name in the text's color; when it opened, and when and why it ended, muted under it.
    <SettingsListRow
      title={room.name}
      facts={[
        t("settings.voice.opened_at", { date: hour(room.opened_at) }),
        ...(room.ended_at === undefined
          ? [t("voice.open_now")]
          : [
              t("settings.voice.ended_at", { date: hour(room.ended_at) }),
              t("settings.voice.ended", { reason: room.end_reason ?? "host" }),
            ]),
      ]}
      href={open ? voiceRoomPath(room.id) : undefined}
      itemRef={itemRef}
    />
  );
}

/** The screen of the entry: the rooms the reader hosted, once the first page is read. */
export function VoiceSettings({ back }: SettingsEntryProps) {
  const { t } = stringsIn(useLocale().locale);
  const list = useVoiceHistory();
  const read = list.items.length > 0 || list.status === "success";
  const added = useAddedFocus(list.items.length);
  return (
    <SettingsScreen
      icon="room"
      title={t("settings.entry.voice")}
      back={back}
      notice={
        read ? undefined : (
          <ReadingNotice status={list.status === "error" ? "failed" : "loading"} retry={list.refetch} />
        )
      }
    >
      {read && (
        <SettingsSection heading={t("settings.voice.hosted")} about={t("settings.voice.hosted.about")}>
          {list.items.length === 0 ? (
            <SettingsEmpty>{t("settings.voice.none")}</SettingsEmpty>
          ) : (
            <SettingsList label={t("settings.voice.hosted")}>
              {list.items.map((room, i) => (
                <Hosted key={room.id} room={room} itemRef={added.item(i)} t={t} />
              ))}
            </SettingsList>
          )}
          <ShowMore
            more={list.hasNextPage}
            busy={list.isFetchingNextPage}
            failed={list.failedNextPage}
            onPress={added.press(list.fetchNextPage)}
          />
        </SettingsSection>
      )}
    </SettingsScreen>
  );
}
