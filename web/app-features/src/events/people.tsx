// SPDX-License-Identifier: AGPL-3.0-only

// Who an event is of, on its page (ADR-0161 §4, §8, FR-943): its organizer and co-organizers, in the organizer's order,
// and who answered going and maybe, each list in the order of their answers, a page at a time.

import { AccountLine } from "@my/app-components";
import { useEventAttendees, type EventAttendee, type ServerEvent } from "@my/app-data";
import { FeedNotice, GroupHeading, RowItem, RowList, Stack } from "@my/design-system";
import type { ReactNode } from "react";
import { MoreOf, Retry, type T } from "./parts";

/** The organizer, then the co-organizers. */
export function Organizers({ event, t }: { event: ServerEvent; t: T }) {
  const heading = t("events.organizers", { count: 1 + event.coorganizers.length });
  return (
    <Stack gap="entry-line">
      <GroupHeading>{heading}</GroupHeading>
      <RowList label={heading}>
        {[event.organizer, ...event.coorganizers].map((account) => (
          <RowItem key={account.id}>
            <AccountLine account={account} />
          </RowItem>
        ))}
      </RowList>
    </Stack>
  );
}

/** The attendees of one answer, under the heading that counts them; nothing for none. */
function Group({ attendees, heading }: { attendees: readonly EventAttendee[]; heading: string }) {
  if (attendees.length === 0) return null;
  return (
    <Stack gap="entry-line">
      <GroupHeading>{heading}</GroupHeading>
      <RowList label={heading}>
        {attendees.map((a) => (
          <RowItem key={a.account.id}>
            <AccountLine account={a.account} />
          </RowItem>
        ))}
      </RowList>
    </Stack>
  );
}

/** Who answered going and maybe, or what stands in their place: that they load, failed, or are none. */
export function Attendees({ event, t }: { event: ServerEvent; t: T }): ReactNode {
  const list = useEventAttendees(event.id);
  if (list.status === "pending") return <FeedNotice start>{t("events.attendees_loading")}</FeedNotice>;
  if (list.items.length === 0)
    return list.status === "error" ? (
      <FeedNotice alert start action={<Retry onPress={list.refetch} t={t} />}>
        {t("events.attendees_failed")}
      </FeedNotice>
    ) : (
      <FeedNotice start>{t("events.attendees_none")}</FeedNotice>
    );
  const going = list.items.filter((a) => a.answer === "going");
  const maybe = list.items.filter((a) => a.answer === "maybe");
  return (
    <Stack gap="entry-room">
      {/* The server counts who is going; who may go is counted by the pages read. */}
      <Group attendees={going} heading={t("events.going_group", { count: event.going_count })} />
      <Group attendees={maybe} heading={t("events.maybe_group")} />
      <MoreOf list={list} t={t} />
    </Stack>
  );
}
