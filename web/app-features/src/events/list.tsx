// SPDX-License-Identifier: AGPL-3.0-only

// The upcoming events the reader sees, at /events, a place of the menu (ADR-0161 §9, FR-951): the server's order, the
// nearest start first, an event a line that leads to its page; a cancelled one stays where the server lists it,
// marked. Show more asks for the next page. Only this server's people see events: the API answers nothing else. Under
// them, for a reader who may create events (FR-940), the way to the form that creates one, at the frame's foot.

import { EventListLine, eventNewPath } from "@my/app-components";
import { eventsQuery, useEvents, useMayCreateEvents, type AppData } from "@my/app-data";
import { FeedNotice, FrameFoot, LinkButton, RowItem, RowList, ShowMore, Stack, useAddedFocus } from "@my/design-system";
import type { ReactNode } from "react";
import { Retry, useEventStrings, type T } from "./parts";

/** What the list's loader waits for: its first page, from the cache unless a write marked it stale. */
export async function loadEvents(data: AppData): Promise<void> {
  await data.queryClient.infiniteQuery({ ...eventsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The list of the upcoming events, and the way to create one where the reader may. */
export function EventsScreen(): ReactNode {
  const t = useEventStrings();
  const { may } = useMayCreateEvents();
  return (
    <Stack gap="entry-room">
      <Events t={t} />
      {may === true && (
        // The list's one action, the foot's whole width, where the thumb is.
        <FrameFoot>
          <Stack>
            <LinkButton kind="primary" size="form" href={eventNewPath}>
              {t("events.create")}
            </LinkButton>
          </Stack>
        </FrameFoot>
      )}
    </Stack>
  );
}

/** The events, or what stands in their place: that they load, failed, or are none. */
function Events({ t }: { t: T }): ReactNode {
  const list = useEvents();
  const added = useAddedFocus(list.items.length);
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
        {list.items.map((event, i) => (
          <RowItem key={event.id} itemRef={added.item(i)}>
            <EventListLine event={event} now={now} />
          </RowItem>
        ))}
      </RowList>
      <ShowMore
        more={list.hasNextPage}
        busy={list.isFetchingNextPage}
        failed={list.failedNextPage}
        onPress={added.press(list.fetchNextPage)}
      />
    </Stack>
  );
}
