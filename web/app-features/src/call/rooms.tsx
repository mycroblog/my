// SPDX-License-Identifier: AGPL-3.0-only

// The open voice chat rooms the reader sees (ADR-0100 §8, FR-928), in the server's order (the rooms of whom they
// follow first, then the fullest), each a line that leads to the room; Show more asks for the next page. While the
// server has the voice chat off, the screen says so and reads nothing.

import { VoiceRoomLine } from "@my/app-components";
import { voiceRoomsQuery, useVoiceRooms, type AppData } from "@my/app-data";
import { Box, Button, FeedNotice, RowItem, RowList, Stack, TextButton, useLocale } from "@my/design-system";
import { stringsIn } from "../locale";
import { useVoiceOff, voiceOn } from "./room";

/** What the list's loader waits for: its first page, from the cache unless a write marked it stale; nothing while off. */
export async function loadVoiceRooms(data: AppData): Promise<void> {
  if (!(await voiceOn(data))) return;
  await data.queryClient.infiniteQuery({ ...voiceRoomsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The list of the open voice chat rooms. */
export function VoiceRooms() {
  const { t } = stringsIn(useLocale().locale);
  const off = useVoiceOff();
  const list = useVoiceRooms(undefined, !off);
  if (off) return <FeedNotice lead>{t("voice.off")}</FeedNotice>;
  const retry = (onPress: () => void) => <TextButton onPress={onPress}>{t("voice.retry")}</TextButton>;
  if (list.status === "pending") return <FeedNotice>{t("voice.rooms_loading")}</FeedNotice>;
  if (list.items.length === 0)
    return list.status === "error" ? (
      <FeedNotice alert action={retry(list.refetch)}>
        {t("voice.rooms_failed")}
      </FeedNotice>
    ) : (
      <FeedNotice>{t("voice.rooms_empty")}</FeedNotice>
    );
  return (
    <Stack gap="entry-room">
      <RowList label={t("voice.rooms")}>
        {list.items.map((room) => (
          <RowItem key={room.id}>
            <VoiceRoomLine room={room} />
          </RowItem>
        ))}
      </RowList>
      {list.failedNextPage ? (
        <FeedNotice alert action={retry(list.fetchNextPage)}>
          {t("voice.rooms_failed_more")}
        </FeedNotice>
      ) : list.isFetchingNextPage ? (
        <FeedNotice>{t("voice.rooms_loading_more")}</FeedNotice>
      ) : (
        list.hasNextPage && (
          <Box>
            <Button size="timeline" touch onPress={list.fetchNextPage}>
              {t("voice.rooms_more")}
            </Button>
          </Box>
        )
      )}
    </Stack>
  );
}
