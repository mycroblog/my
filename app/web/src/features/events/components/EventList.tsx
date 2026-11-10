// SPDX-License-Identifier: AGPL-3.0-only
// The lists of events (ADR-0161 §9, web-client/screens/events.md): the list page's body (`/events`) with the button
// that creates an event, the third column's "Coming up" on wide screens, and the events an account organizes on its
// profile. Each event is a card: its title (a link to its page), when it is in the viewer's time zone and in its own,
// its organizer, its state and the user's answer, and how many are going (of its capacity).
import {
  AsidePanel,
  Box,
  ButtonLink,
  Card,
  Cluster,
  EmptyState,
  EventIcon,
  Heading,
  Link,
  PlusIcon,
  RegionError,
  Section,
  Stack,
  Text,
  Button,
} from "@my/ui";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { profilePath } from "../../../lib/paths";
import { type Event, eventPath } from "../api/events";
import { useEventList, useEventRail } from "../hooks/useEvents";
import { useEventWhen } from "../hooks/useEventTime";
import { accountName } from "../hooks/names";
import { EventBadges, EventWhenText } from "./EventParts";
import { EventListSkeleton } from "./EventSkeletons";

type EventItemProps = { event: Event; headingLevel: 2 | 3; surface?: "card" | "plain" };

/** How many are going, of the capacity when the event has one. */
function useGoingText(event: Event): string {
  const t = useT();
  const intl = useIntl();
  return event.capacity > 0
    ? t("client:events.going_of_capacity", { count: event.going_count, capacity: intl.formatNumber(event.capacity) })
    : t("client:events.going_count", { count: event.going_count });
}

/** "Organized by {name}", the name a link to the organizer's profile. */
export function OrganizedBy({ event }: { event: Event }) {
  const intl = useIntl();
  return (
    <>
      {intl.formatMessage(
        { id: "client:events.organized_by" },
        {
          name: (
            <Link key="organizer" href={profilePath(event.organizer.acct)} variant="standalone">
              {/* The name is the user's text, in any script (web-client/README.md Y10). */}
              <bdi>{accountName(event.organizer)}</bdi>
            </Link>
          ),
        },
      )}
    </>
  );
}

/** One event as an `li` of a list. */
function EventItem({ event, headingLevel, surface = "card" }: EventItemProps) {
  const when = useEventWhen(event);
  const going = useGoingText(event);
  const body = (
    <Stack gap="2">
      <Cluster gap="2" align="center">
        <EventBadges event={event} when={when} />
      </Cluster>
      <Heading level={headingLevel} size={surface === "card" ? "subsection" : "label"} wrap="anywhere">
        <Link href={eventPath(event.id)} variant="standalone">
          {/* The title is the organizer's text, in any script (web-client/README.md Y10). */}
          <span dir="auto">{event.title}</span>
        </Link>
      </Heading>
      <EventWhenText when={when} size="sm" />
      <Text as="p" size="sm" tone="muted">
        <OrganizedBy event={event} />
      </Text>
      <Text as="p" size="sm" tone="muted">
        {going}
      </Text>
    </Stack>
  );
  return surface === "card" ? (
    <Card as="li" padding="4">
      {body}
    </Card>
  ) : (
    <li>{body}</li>
  );
}

/** The list page's body (`/events`): the button that creates an event, then the upcoming events the user sees. */
export function EventListView() {
  const t = useT();
  return (
    <Stack gap="6">
      <div>
        <ButtonLink href="/events/new" variant="primary">
          <PlusIcon />
          {t("client:events.new")}
        </ButtonLink>
      </div>
      <Events />
    </Stack>
  );
}

function Events() {
  const t = useT();
  const list = useEventList();
  switch (list.status) {
    case "loading":
      return <EventListSkeleton />;
    case "error":
      return (
        <RegionError
          heading={t("client:events.list.failed")}
          reason={list.reason}
          retryLabel={t("client:events.retry")}
          onRetry={list.retry}
          isRetrying={list.isRetrying}
        />
      );
    case "ready":
      return (
        <>
          {list.events.length === 0 ? (
            <EmptyState
              illustration={<EventIcon />}
              heading={t("client:events.list.empty")}
              description={t("client:events.list.empty.description")}
            />
          ) : (
            <Stack as="ul" gap="3" aria-label={t("client:events.list.label")}>
              {list.events.map((event) => (
                <EventItem key={event.id} event={event} headingLevel={2} />
              ))}
            </Stack>
          )}
          {list.moreError !== undefined && (
            <RegionError heading={t("client:events.list.more_failed")} reason={list.moreError} />
          )}
          {list.hasMore && (
            <div>
              <Button onPress={list.loadMore} isPending={list.isLoadingMore}>
                {t("client:events.list.more")}
              </Button>
            </div>
          )}
        </>
      );
  }
}

/**
 * The third column's "Coming up" (web-client/09-app-shell.md §9): the first upcoming events the user sees and the link
 * to all of them. Absent for a visitor who is not signed in and while no event is coming.
 */
export function EventRail() {
  const t = useT();
  const events = useEventRail();
  if (events.length === 0) return null;
  return (
    <AsidePanel heading={t("client:events.rail.title")}>
      <Stack as="ul" gap="4">
        {events.map((event) => (
          <EventItem key={event.id} event={event} headingLevel={3} surface="plain" />
        ))}
      </Stack>
      <Text as="p" size="sm">
        <Link href="/events" variant="standalone">
          {t("client:events.rail.all")}
        </Link>
      </Text>
    </AsidePanel>
  );
}

export type ProfileEventsProps = {
  accountId: string;
  /** The account's handle; a remote account (`user@domain`) organizes no events of this server. */
  acct: string;
};

/** The upcoming events that a local account organizes and the user sees, on its profile; absent when there are none. */
export function ProfileEvents({ accountId, acct }: ProfileEventsProps) {
  const t = useT();
  const events = useEventRail(accountId, { enabled: !acct.includes("@") });
  if (events.length === 0) return null;
  return (
    // Inset like the profile's other notes under its header.
    <Box paddingX="4">
      <Section title={t("client:events.profile.title")}>
        <Stack as="ul" gap="3">
          {events.map((event) => (
            <EventItem key={event.id} event={event} headingLevel={3} />
          ))}
        </Stack>
      </Section>
    </Box>
  );
}
