// SPDX-License-Identifier: AGPL-3.0-only

// Who an event is of, on its page (ADR-0161 §4, §6, §8, FR-943): its organizer and co-organizers, in the organizer's
// order, and who answered going and maybe, each list in the order of their answers, a page at a time. For its
// organizers, each attendee who does not organize it says whether they approved them, and a menu on the attendee
// approves them (their place in person and online then shows to them) or takes the approval back; what came of it is
// said in the page's live region, the focus back on the attendee's menu.

import { AccountLine, nameOf } from "@my/app-components";
import {
  eventRefusal,
  organizesEvent,
  useEventAttendees,
  useEventWrites,
  type EventAttendee,
  type EventPhase,
  type ServerEvent,
} from "@my/app-data";
import {
  Button,
  FeedNotice,
  GroupHeading,
  IconButton,
  MenuGroup,
  MenuItem,
  MenuSurface,
  Row,
  RowItem,
  RowList,
  ShowMore,
  Stack,
  StateText,
  useAddedFocus,
  type AddedFocus,
} from "@my/design-system";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { Retry, type T } from "./parts";
import type { Say } from "./places";

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

/**
 * What the organizers may do with one attendee: one waiting for approval says so and carries Approve, the row's one
 * action; one approved carries its mark and the menu that takes the approval back. Approving shows at once, and the
 * focus goes to what takes the pressed control's place (the menu, or Approve again).
 */
function Approval({
  attendee,
  approve,
  reread,
  say,
  t,
}: {
  attendee: EventAttendee;
  approve: (accountId: string, approved: boolean) => Promise<EventAttendee>;
  reread: () => void;
  say: Say;
  t: T;
}) {
  const name = nameOf(attendee.account);
  const place = useRef<HTMLDivElement>(null);
  // A press whose control the approval it shows takes away: the focus goes to the one in its place, once drawn.
  const [moved, setMoved] = useState<boolean | null>(null);
  useEffect(() => {
    if (moved === null || moved !== attendee.approved) return;
    place.current?.querySelector<HTMLElement>("button:not([disabled])")?.focus();
    setMoved(null);
  }, [moved, attendee.approved]);
  const run = (approved: boolean) => {
    setMoved(approved);
    approve(attendee.account.id, approved).then(
      () => {
        say(t("events.approval.done", { approved: approved ? "yes" : "no", name }));
      },
      (error: unknown) => {
        // The attendee answered otherwise, or the event changed: it and its attendees are read again.
        if (eventRefusal(error) !== null) reread();
        say(
          t("events.approval.failed", {
            approved: approved ? "yes" : "no",
            name,
            cause: eventRefusal(error) === "not-attending" ? "not_attending" : "other",
          }),
        );
      },
    );
  };
  return (
    <div ref={place}>
      {attendee.approved ? (
        <Row align="center" gap="entry-caption">
          <StateText>{t("events.approval.approved")}</StateText>
          <MenuSurface
            trigger={<IconButton icon="more" label={t("events.approval.menu", { name })} square />}
            label={t("events.approval.menu", { name })}
            placement="bottom end"
          >
            <MenuGroup>
              <MenuItem
                id="unapprove"
                icon="close"
                onAction={() => {
                  run(false);
                }}
              >
                {t("events.approval.op", { approved: "no" })}
              </MenuItem>
            </MenuGroup>
          </MenuSurface>
        </Row>
      ) : (
        <Row wrap align="center" justify="end" gap="entry-caption">
          <StateText>{t("events.approval.waiting")}</StateText>
          <Button
            kind="primary"
            size="target"
            aria-label={t("events.approval.approve", { name })}
            onPress={() => {
              run(true);
            }}
          >
            {t("events.approval.op", { approved: "yes" })}
          </Button>
        </Row>
      )}
    </div>
  );
}

/** The attendees of one answer, under the heading that counts them; nothing for none. */
function Group({
  attendees,
  heading,
  end,
  item,
}: {
  attendees: readonly EventAttendee[];
  heading: string;
  /** The ref of an attendee's row, for the list that grows under Show more. */
  item: (attendee: EventAttendee) => ReturnType<AddedFocus["item"]>;
  /** What stands at the end of each attendee's line (the organizers' approval). */
  end?: ((attendee: EventAttendee) => ReactNode) | undefined;
}) {
  if (attendees.length === 0) return null;
  return (
    <Stack gap="entry-line">
      <GroupHeading>{heading}</GroupHeading>
      <RowList label={heading}>
        {attendees.map((a) => (
          <RowItem key={a.account.id} itemRef={item(a)}>
            <AccountLine account={a.account} end={end?.(a)} />
          </RowItem>
        ))}
      </RowList>
    </Stack>
  );
}

/**
 * Who answered going and maybe, or what stands in their place: that they load, failed, or are none. Its organizers
 * approve each until the event is cancelled or ends.
 */
export function Attendees({
  event,
  phase,
  say,
  t,
}: {
  event: ServerEvent;
  phase: EventPhase;
  say: Say;
  t: T;
}): ReactNode {
  const list = useEventAttendees(event.id);
  const writes = useEventWrites(event.id);
  const added = useAddedFocus(list.items.length);
  // The first attendee a press added takes the focus, in whichever group it stands.
  const item = (attendee: EventAttendee) => added.item(list.items.indexOf(attendee));
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
  const approves = organizesEvent(event) && phase !== "cancelled" && phase !== "ended";
  // An organizer or co-organizer who answered always sees the places (ADR-0161 §6): nothing asks to approve them.
  const end = approves
    ? (attendee: EventAttendee) =>
        attendee.organizes ? null : (
          <Approval attendee={attendee} approve={writes.approve} reread={writes.reread} say={say} t={t} />
        )
    : organizesEvent(event)
      ? (attendee: EventAttendee) =>
          attendee.approved && !attendee.organizes ? <StateText>{t("events.approval.approved")}</StateText> : null
      : undefined;
  return (
    <Stack gap="entry-room">
      {/* The server counts who is going; who may go is counted by the pages read. */}
      <Group attendees={going} heading={t("events.going_group", { count: event.going_count })} end={end} item={item} />
      <Group attendees={maybe} heading={t("events.maybe_group")} end={end} item={item} />
      <ShowMore
        more={list.hasNextPage}
        busy={list.isFetchingNextPage}
        failed={list.failedNextPage}
        onPress={added.press(list.fetchNextPage)}
      />
    </Stack>
  );
}
