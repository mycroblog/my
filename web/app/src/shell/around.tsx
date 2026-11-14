// SPDX-License-Identifier: AGPL-3.0-only

// The column beside the main one, from the frame's `wide` (ADR-0110 §11, ADR-0125 §4, ADR-0100 §8, FR-951, ADR-0067
// §22, ADR-0086 §18): what is around the reader, in sections on the page, in the decider's order (ADR-0125 §4): the
// unread announcements the reader sees now, when there are any (not on the announcements' own screen, which lists
// them; the menu leads to all of them); the first open voice rooms; the first upcoming events; the latest notifications
// (not on their own screen); and the funding meter when the instance shows it here. The server's pages end it, as they
// end the menu's sheet. Under `wide`, a line above the home timeline counts the unread announcements instead.

import { LatestNotifications } from "@my/app-features";
import {
  useAnnouncements,
  useOpenVoiceRooms,
  useUpcomingEvents,
  type Announcement,
  type FundingMeter,
  type Instance,
  type ServerEvent,
  type VoiceRoom,
} from "@my/app-data";
import {
  FrameAside,
  FrameFold,
  FrameFooter,
  FrameSection,
  Meter,
  RelativeTime,
  SectionLine,
  SectionList,
  useLocale,
} from "@my/design-system";
import { useStrings } from "../locale";

/** The links to the server's pages: about, and the terms and the privacy policy when the instance has them. */
export function ServerFooter({ instance }: { instance: Instance | undefined }) {
  const t = useStrings();
  return (
    <FrameFooter
      label={t("app.footer")}
      links={[
        { href: "/about", label: t("app.footer_about") },
        ...(instance?.terms_url === undefined ? [] : [{ href: instance.terms_url, label: t("app.footer_terms") }]),
        ...(instance?.privacy_policy_url === undefined
          ? []
          : [{ href: instance.privacy_policy_url, label: t("app.footer_privacy") }]),
      ]}
    />
  );
}

/** The announcements the reader sees now, the unread first. */
function AnnouncementLines({ items }: { items: readonly Announcement[] }) {
  const t = useStrings();
  return (
    <SectionList>
      {items.map((a) => (
        <SectionLine
          key={a.id}
          href={`/announcements/${a.id}`}
          title={a.title === "" ? t("app.announcement_untitled") : a.title}
          caption={<RelativeTime date={a.published_at} />}
          unread={a.read ? undefined : t("app.unread")}
          read={a.read}
        />
      ))}
    </SectionList>
  );
}

/**
 * The unread announcements folded to one line above the home timeline, under `wide`, where no column beside it holds
 * them: how many, a link to their list, so that they never push the timeline down (ADR-0125 §4); nothing when every
 * one the reader sees now is read.
 */
export function UnreadAnnouncements() {
  const t = useStrings();
  const unread = useAnnouncements()?.filter((a) => !a.read).length ?? 0;
  if (unread === 0) return null;
  return (
    <FrameFold icon="announcements" href="/announcements">
      {t("app.announcements_unread", { count: unread })}
    </FrameFold>
  );
}

/** The place name of an IANA time zone (`Asia/Tokyo` is Tokyo). */
const placeOf = (zone: string) => (zone.split("/").at(-1) ?? zone).replaceAll("_", " ");

/**
 * When an event starts, in the reader's time zone, and in the event's own with its place's name when that is another
 * (FR-951).
 */
function useEventTime(event: ServerEvent): string {
  const t = useStrings();
  const { locale } = useLocale();
  const at = new Date(event.starts_at);
  const here = new Intl.DateTimeFormat(locale, {
    weekday: "short",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(at);
  let there: string;
  try {
    if (new Intl.DateTimeFormat().resolvedOptions().timeZone === event.time_zone) return here;
    there = new Intl.DateTimeFormat(locale, { hour: "numeric", minute: "2-digit", timeZone: event.time_zone }).format(
      at,
    );
  } catch {
    // A time zone the browser does not know: the reader's time alone.
    return here;
  }
  return t("app.event_elsewhere", { here, there, zone: placeOf(event.time_zone) });
}

function EventLine({ event }: { event: ServerEvent }) {
  return <SectionLine href={`/events/${event.id}`} title={event.title} caption={useEventTime(event)} />;
}

function RoomLine({ room }: { room: VoiceRoom }) {
  const t = useStrings();
  return (
    <SectionLine
      href={`/voice/${room.id}`}
      title={room.name}
      caption={t("app.room_caption", {
        host: room.host.display_name || room.host.username,
        count: room.participants_count,
      })}
    />
  );
}

/** The funding meter of this month: the share of its costs covered, the month, and whether its costs may change. */
function Funding({ meter }: { meter: FundingMeter }) {
  const t = useStrings();
  const { locale } = useLocale();
  const month = new Intl.DateTimeFormat(locale, { month: "long", timeZone: "UTC" }).format(new Date(meter.starts_on));
  const caption = t("app.funding_caption", { covered: meter.covered ? "yes" : "no", month });
  return (
    <FrameSection id="around-funding" icon="funding" href="/about#funding" heading={t("app.funding")}>
      <Meter
        value={meter.percent / 100}
        label={t("app.funding_label")}
        caption={
          meter.estimated ? (
            // The estimate is a sentence of its own, on its own line under the caption.
            <>
              {caption}
              <br />
              {t("app.funding_estimated")}
            </>
          ) : (
            caption
          )
        }
      />
    </FrameSection>
  );
}

/**
 * The column beside the main one: its sections, for a reader signed in or not, at `pathname`, by what the instance
 * offers, and the server's pages at its end.
 */
export function Around({
  signedIn,
  pathname,
  instance,
}: {
  signedIn: boolean;
  pathname: string;
  instance: Instance | undefined;
}) {
  const t = useStrings();
  const unread = useAnnouncements(signedIn && pathname !== "/announcements")?.filter((a) => !a.read);
  const rooms = useOpenVoiceRooms(signedIn && instance?.voice_enabled === true);
  const events = useUpcomingEvents(signedIn);
  const meter = instance?.funding_meter;
  return (
    <FrameAside label={t("app.around")}>
      {unread !== undefined && unread.length > 0 && (
        <FrameSection
          id="around-announcements"
          icon="announcements"
          href="/announcements"
          heading={t("app.announcements")}
          all={{ text: t("app.around_all"), name: t("app.around_all_announcements") }}
        >
          <AnnouncementLines items={unread} />
        </FrameSection>
      )}
      {rooms !== undefined && rooms.items.length > 0 && (
        <FrameSection
          id="around-rooms"
          icon="room"
          href="/voice"
          heading={t("app.voice_rooms")}
          all={rooms.more ? { text: t("app.around_all"), name: t("app.around_all_rooms") } : undefined}
        >
          <SectionList>
            {rooms.items.map((room) => (
              <RoomLine key={room.id} room={room} />
            ))}
          </SectionList>
        </FrameSection>
      )}
      {events !== undefined && events.items.length > 0 && (
        <FrameSection
          id="around-events"
          icon="events"
          href="/events"
          heading={t("app.events")}
          all={events.more ? { text: t("app.around_all"), name: t("app.around_all_events") } : undefined}
        >
          <SectionList>
            {events.items.map((event) => (
              <EventLine key={event.id} event={event} />
            ))}
          </SectionList>
        </FrameSection>
      )}
      {signedIn && pathname !== "/notifications" && (
        <FrameSection
          id="around-notifications"
          icon="notifications"
          href="/notifications"
          heading={t("app.notifications")}
          all={{ text: t("app.around_all"), name: t("app.around_all_notifications") }}
        >
          <LatestNotifications instance={instance?.name ?? ""} />
        </FrameSection>
      )}
      {meter?.rail === true && <Funding meter={meter} />}
      <ServerFooter instance={instance} />
    </FrameAside>
  );
}
