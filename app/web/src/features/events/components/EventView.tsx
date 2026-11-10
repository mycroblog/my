// SPDX-License-Identifier: AGPL-3.0-only
// The page of an event (`/events/{id}`, ADR-0161 §5–§9, §15; web-client/screens/event.md): its title is the page's
// heading, then its state, its cancellation and the reason the organizer or a moderator gave, when it is (in the
// viewer's time zone and in the event's own), who organizes it, adding it to the user's calendar, the user's answer,
// where it is, what it is about, and who is going. Its organizers also approve attendees, change it, invite people,
// and (the organizer alone) name co-organizers and cancel it. A place that the user does not see never reaches the
// client: the page names only its kind and that it shows to approved attendees. Links that leave the site say so.
import {
  Button,
  ButtonLink,
  Callout,
  Cluster,
  ConfirmDialog,
  ExternalLink,
  Hint,
  InlineError,
  Link,
  Radio,
  RadioGroup,
  RegionError,
  Section,
  Stack,
  Text,
  TextArea,
} from "@my/ui";
import { type ReactNode, useState } from "react";
import { useT, type ClientMessageKey } from "../../../lib/i18n";
import { profilePath, signInPath } from "../../../lib/paths";
import { textTokens } from "../../../lib/textTokens";
import {
  type Event,
  type EventAnswer,
  eventAnswers,
  type EventCancelReason,
  eventLimits,
  eventEditPath,
  eventPath,
  type EventPlace,
  type EventPlaceKind,
} from "../api/events";
import { accountName } from "../hooks/names";
import { type EventRole, useCancelEvent, useEventAnswer, useEventCalendar, useEventPage } from "../hooks/useEventPage";
import { type EventWhen, useEventWhen } from "../hooks/useEventTime";
import { EventAttendees } from "./EventAttendees";
import { EventCoorganizers, EventInvitations } from "./EventOrganizing";
import { EventBadges, EventWhenText } from "./EventParts";
import { OrganizedBy } from "./EventList";
import { EventPageSkeleton } from "./EventSkeletons";

export type EventViewProps = {
  id: string;
  /** Renders the page heading (the route's level-one heading) around its text. */
  renderHeading: (text: ReactNode) => ReactNode;
};

export function EventView({ id, renderHeading }: EventViewProps) {
  const t = useT();
  const page = useEventPage(id);
  switch (page.status) {
    case "loading":
      return (
        <>
          {renderHeading(t("client:events.page.title"))}
          <EventPageSkeleton />
        </>
      );
    case "signed-out":
      return (
        <>
          {renderHeading(t("client:events.page.title"))}
          <Callout>
            <p>{t("client:events.page.local_only")}</p>
            <Link href={signInPath(eventPath(id))} variant="standalone">
              {t("client:events.page.sign_in")}
            </Link>
          </Callout>
        </>
      );
    case "not-found":
      return (
        <>
          {renderHeading(t("client:events.page.not_found"))}
          <Text as="p">{t("client:events.page.not_found.body")}</Text>
          <AllEventsLink />
        </>
      );
    case "error":
      return (
        <>
          {renderHeading(t("client:events.page.title"))}
          <RegionError
            heading={t("client:events.page.failed")}
            reason={page.reason}
            retryLabel={t("client:events.retry")}
            onRetry={page.retry}
            isRetrying={page.isRetrying}
          />
        </>
      );
    case "ready":
      return <EventDetails event={page.event} role={page.role} renderHeading={renderHeading} />;
  }
}

function AllEventsLink() {
  const t = useT();
  return (
    <Text as="p">
      <Link href="/events" variant="standalone">
        {t("client:events.rail.all")}
      </Link>
    </Text>
  );
}

type EventDetailsProps = { event: Event; role: EventRole; renderHeading: (text: ReactNode) => ReactNode };

function EventDetails({ event, role, renderHeading }: EventDetailsProps) {
  const when = useEventWhen(event);
  const organizes = role !== "guest";
  return (
    <Stack gap="6">
      <Stack gap="3">
        {/* The title is the organizer's text, in any script (web-client/README.md Y10). */}
        {renderHeading(
          <Text wrap="anywhere" dir="auto">
            {event.title}
          </Text>,
        )}
        <Cluster gap="2" align="center">
          <EventBadges event={event} when={when} />
        </Cluster>
        {event.cancellation !== undefined && <Cancellation event={event} />}
        <EventWhenText when={when} />
        <Organizers event={event} />
        <EventActions event={event} role={role} when={when} />
      </Stack>
      {event.state !== "cancelled" && <AnswerSection event={event} when={when} />}
      <PlacesSection event={event} />
      {event.description !== "" && <DescriptionSection description={event.description} />}
      <EventAttendees event={event} organizes={organizes} />
      {organizes && event.state !== "cancelled" && <EventInvitations event={event} />}
      {role === "organizer" && event.state !== "cancelled" && <EventCoorganizers event={event} />}
    </Stack>
  );
}

/** The words of each reason of a cancellation. */
const cancelReasons: Readonly<Record<EventCancelReason, ClientMessageKey>> = {
  organizer: "client:events.cancelled.organizer",
  moderator: "client:events.cancelled.moderator",
  organizer_suspended: "client:events.cancelled.organizer_suspended",
  organizer_deleted: "client:events.cancelled.organizer_deleted",
};

/** That the event was cancelled, by whom, and the reason given (a moderator's note shows too). */
function Cancellation({ event }: { event: Event }) {
  const t = useT();
  if (event.cancellation === undefined) return null;
  const { reason, note } = event.cancellation;
  return (
    <Callout>
      <p>{t(cancelReasons[reason])}</p>
      {note !== undefined && note !== "" && (
        <Text as="p" whitespace="preLine" dir="auto" wrap="anywhere">
          {t("client:events.cancelled.note", { note })}
        </Text>
      )}
    </Callout>
  );
}

/** The organizer and the co-organizers, each a link to their profile. */
function Organizers({ event }: { event: Event }) {
  const t = useT();
  return (
    <Stack gap="1">
      <Text as="p" size="sm">
        <OrganizedBy event={event} />
      </Text>
      {event.coorganizers.length > 0 && (
        <Cluster as="ul" gap="2" aria-label={t("client:events.coorganizers.title")}>
          {event.coorganizers.map((account) => (
            <Text as="li" size="sm" key={account.id}>
              <Link href={profilePath(account.acct)} variant="standalone">
                <bdi>{accountName(account)}</bdi>
              </Link>
            </Text>
          ))}
        </Cluster>
      )}
    </Stack>
  );
}

/** Adding the event to the user's calendar, and for its organizers changing and cancelling it. */
function EventActions({ event, role, when }: { event: Event; role: EventRole; when: EventWhen }) {
  const t = useT();
  const calendar = useEventCalendar(event);
  const open = event.state !== "cancelled" && !when.ended;
  return (
    <Stack gap="2">
      <Cluster gap="2" align="center">
        {event.state !== "unscheduled" && (
          <Button onPress={calendar.save} isPending={calendar.isPending}>
            {t("client:events.calendar")}
          </Button>
        )}
        {role !== "guest" && open && <ButtonLink href={eventEditPath(event.id)}>{t("client:events.edit")}</ButtonLink>}
        {role === "organizer" && open && <CancelButton event={event} />}
      </Cluster>
      {calendar.error !== undefined && (
        <InlineError alert size="inherit">
          {t("client:events.calendar.failed", { reason: calendar.error })}
        </InlineError>
      )}
    </Stack>
  );
}

/** Cancelling the event, after a confirmation that takes the reason to show. */
function CancelButton({ event }: { event: Event }) {
  const t = useT();
  const cancel = useCancelEvent(event);
  const [isOpen, setIsOpen] = useState(false);
  const [note, setNote] = useState("");
  return (
    <>
      <Button
        variant="danger"
        onPress={() => {
          cancel.reset();
          setIsOpen(true);
        }}
      >
        {t("client:events.cancel")}
      </Button>
      <ConfirmDialog
        isOpen={isOpen}
        onOpenChange={setIsOpen}
        title={t("client:events.cancel.title")}
        description={t("client:events.cancel.description")}
        confirmLabel={t("client:events.cancel.confirm")}
        cancelLabel={t("client:events.cancel.back")}
        onConfirm={(close) => {
          cancel.run(note, close);
        }}
        isPending={cancel.isPending}
        errorMessage={cancel.error}
      >
        <TextArea
          label={t("client:events.cancel.note")}
          description={t("client:events.cancel.note.description")}
          value={note}
          onChange={setNote}
          rows={3}
          maxLength={eventLimits.cancelNote}
        />
      </ConfirmDialog>
    </>
  );
}

/** The words of each answer, as the choices of the user's answer. */
const answerLabels: Readonly<Record<EventAnswer, ClientMessageKey>> = {
  going: "client:events.answer.going",
  maybe: "client:events.answer.maybe",
  declined: "client:events.answer.declined",
};

/** The user's answer, saved at once; closed once the event started, and while its date is polled. */
function AnswerSection({ event, when }: { event: Event; when: EventWhen }) {
  const t = useT();
  const answer = useEventAnswer(event, when.started);
  const full = event.capacity > 0 && event.going_count >= event.capacity && event.viewer.answer !== "going";
  const hint = !answer.open
    ? t(event.state === "unscheduled" ? "client:events.answer.unscheduled" : "client:events.answer.closed")
    : t("client:events.answer.public");
  return (
    <Section title={t("client:events.answer.title")}>
      <Stack gap="3">
        <RadioGroup
          aria-label={t("client:events.answer.title")}
          orientation="horizontal"
          value={answer.answer ?? null}
          onChange={(value) => {
            const picked = eventAnswers.find((a) => a === value);
            if (picked !== undefined) answer.change(picked);
          }}
          isDisabled={!answer.open || answer.isPending}
          description={hint}
        >
          {eventAnswers.map((value) => (
            <Radio key={value} value={value} isDisabled={value === "going" && full}>
              {t(answerLabels[value])}
            </Radio>
          ))}
        </RadioGroup>
        {full && answer.open && <Hint>{t("client:events.answer.full")}</Hint>}
        {answer.error !== undefined && (
          <InlineError alert size="inherit">
            {t("client:events.answer.failed", { reason: answer.error })}
          </InlineError>
        )}
      </Stack>
    </Section>
  );
}

/** The words of each place that the user does not see. */
const hiddenPlaceLabels: Readonly<Record<EventPlaceKind, ClientMessageKey>> = {
  physical: "client:events.place.hidden.physical",
  url: "client:events.place.hidden.url",
  voice: "client:events.place.hidden.voice",
};

/** Where the event is: the places the user sees, and the kinds of those shown only to approved attendees. */
function PlacesSection({ event }: { event: Event }) {
  const t = useT();
  if (event.places.length === 0 && event.hidden_places.length === 0) return null;
  return (
    <Section title={t("client:events.place.title")}>
      <Stack gap="4">
        {event.places.map((place) => (
          <Place key={place.kind} place={place} />
        ))}
        {event.hidden_places.map((kind) => (
          <Text as="p" key={kind} tone="muted">
            {t(hiddenPlaceLabels[kind])}
          </Text>
        ))}
      </Stack>
    </Section>
  );
}

/** The host of `url` for the words that say a link leaves the site; the URL itself when it has none. */
function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}

/**
 * A link that leaves the site: it opens in a new tab, the server never fetched it, and a line under it names the site
 * it goes to, so that nobody follows it unaware (ADR-0161 §6).
 */
function LeavingLink({ url, children }: { url: string; children: ReactNode }) {
  const t = useT();
  return (
    <Stack gap="1">
      <Text as="p" wrap="anywhere">
        <ExternalLink href={url} rel="nofollow" variant="standalone">
          {children}
        </ExternalLink>
      </Text>
      <Hint>{t("client:events.place.leaves_site", { host: hostOf(url) })}</Hint>
    </Stack>
  );
}

function Place({ place }: { place: EventPlace }) {
  const t = useT();
  switch (place.kind) {
    case "physical":
      return (
        <Stack gap="1">
          <Text as="p" size="sm" tone="muted">
            {t("client:events.place.physical")}
          </Text>
          <Text as="p" weight="semibold" dir="auto" wrap="anywhere">
            {place.name}
          </Text>
          {place.address !== undefined && place.address !== "" && (
            <Text as="p" whitespace="preLine" dir="auto" wrap="anywhere">
              {place.address}
            </Text>
          )}
          {place.url !== undefined && place.url !== "" && (
            <LeavingLink url={place.url}>{t("client:events.place.map")}</LeavingLink>
          )}
        </Stack>
      );
    case "url":
      return (
        <Stack gap="1">
          <Text as="p" size="sm" tone="muted">
            {t("client:events.place.url")}
          </Text>
          {place.url !== undefined && (
            <LeavingLink url={place.url}>
              <bdi>{place.name ?? place.url}</bdi>
            </LeavingLink>
          )}
        </Stack>
      );
    case "voice":
      return (
        <Stack gap="1">
          <Text as="p" size="sm" tone="muted">
            {t("client:events.place.voice")}
          </Text>
          <Text as="p">{t("client:events.place.voice.description")}</Text>
        </Stack>
      );
  }
}

/** What the event is about: the organizer's plain text, with only its URLs as links, which leave the site. */
function DescriptionSection({ description }: { description: string }) {
  const t = useT();
  const parts: ReactNode[] = [];
  let at = 0;
  for (const token of textTokens(description)) {
    if (token.kind !== "url" || !/^https?:\/\//iu.test(token.text)) continue;
    if (token.start > at) parts.push(description.slice(at, token.start));
    parts.push(
      <ExternalLink key={token.start} href={token.text} rel="nofollow ugc">
        {token.text}
      </ExternalLink>,
    );
    at = token.end;
  }
  if (at < description.length) parts.push(description.slice(at));
  return (
    <Section title={t("client:events.description.title")}>
      <Text as="p" whitespace="preLine" dir="auto" wrap="anywhere">
        {parts}
      </Text>
    </Section>
  );
}
