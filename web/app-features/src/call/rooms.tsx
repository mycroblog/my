// SPDX-License-Identifier: AGPL-3.0-only

// The open voice chat rooms the reader sees (ADR-0100 §8, FR-928), in the server's order (the rooms of whom they
// follow first, then the fullest), each a line that leads to the room; Show more asks for the next page. Under them,
// for a reader who may open rooms (FR-922), the way to the form that opens one, at the frame's foot. While the server has the voice chat
// off, the screen says so and reads nothing.

import { VoiceRoomLine, voiceOpenPath } from "@my/app-components";
import { useMayOpenVoiceRooms, voiceRoomsQuery, useVoiceRooms, type AppData } from "@my/app-data";
import {
  Box,
  Button,
  FeedNotice,
  FrameFoot,
  LinkButton,
  RowItem,
  RowList,
  Stack,
  TextButton,
  useLocale,
} from "@my/design-system";
import type { ReactNode } from "react";
import { stringsIn } from "../locale";
import { useVoiceOff, voiceOn } from "./room";

/** What the list's loader waits for: its first page, from the cache unless a write marked it stale; nothing while off. */
export async function loadVoiceRooms(data: AppData): Promise<void> {
  if (!(await voiceOn(data))) return;
  await data.queryClient.infiniteQuery({ ...voiceRoomsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The list of the open voice chat rooms, and the way to open one where the reader may. */
export function VoiceRooms() {
  const { t } = stringsIn(useLocale().locale);
  const off = useVoiceOff();
  const may = useMayOpenVoiceRooms();
  if (off) return <FeedNotice lead>{t("voice.off")}</FeedNotice>;
  return (
    <Stack gap="entry-room">
      <Rooms />
      {may && (
        // The list's one action, the foot's whole width, where the thumb is.
        <FrameFoot>
          <Stack>
            <LinkButton kind="primary" size="form" href={voiceOpenPath}>
              {t("voice.open")}
            </LinkButton>
          </Stack>
        </FrameFoot>
      )}
    </Stack>
  );
}

/** The rooms, or what stands in their place: that they load, failed, or are none. */
function Rooms(): ReactNode {
  const { t } = stringsIn(useLocale().locale);
  const list = useVoiceRooms();
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
