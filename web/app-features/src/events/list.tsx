// SPDX-License-Identifier: AGPL-3.0-only

// The upcoming events the reader sees, at /events, a place of the menu (ADR-0161 §9, FR-951): the server's order, the
// nearest start first, an event a line that leads to its page; a cancelled one stays where the server lists it,
// marked. Show more asks for the next page. Only this server's people see events: the API answers nothing else.

import { EventListLine } from "@my/app-components";
import { eventsQuery, useEvents, type AppData } from "@my/app-data";
import { FeedNotice, RowItem, RowList, Stack } from "@my/design-system";
import type { ReactNode } from "react";
import { MoreOf, Retry, useEventStrings } from "./parts";

/** What the list's loader waits for: its first page, from the cache unless a write marked it stale. */
export async function loadEvents(data: AppData): Promise<void> {
  await data.queryClient.infiniteQuery({ ...eventsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The list of the upcoming events, or what stands in its place: that they load, failed, or are none. */
export function EventsScreen(): ReactNode {
  const t = useEventStrings();
  const list = useEvents();
  if (list.status === "pending") return <FeedNotice>{t("events.loading")}</FeedNotice>;
  if (list.items.length === 0)
    return list.status === "error" ? (
      <FeedNotice alert action={<Retry onPress={list.refetch} t={t} />}>
        {t("events.failed")}
      </FeedNotice>
    ) : (
      <FeedNotice lead>{t("events.empty")}</FeedNotice>
    );
  const now = Date.now();
  return (
    <Stack gap="entry-room">
      <RowList label={t("events.list")}>
        {list.items.map((event) => (
          <RowItem key={event.id}>
            <EventListLine event={event} now={now} />
          </RowItem>
        ))}
      </RowList>
      <MoreOf list={list} t={t} />
    </Stack>
  );
}
