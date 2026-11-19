// SPDX-License-Identifier: AGPL-3.0-only

// The column beside the main one, from the frame's `wide` (ADR-0110 §11, ADR-0125 §4, ADR-0100 §8, FR-951, ADR-0067
// §22, ADR-0086 §18): what is around the reader, in sections on the page, in the decider's order (ADR-0125 §4): the
// unread announcements the reader sees now, when there are any (not on the announcements' own screen, which lists
// them; the menu leads to all of them); the first open voice rooms; the first upcoming events; the latest notifications
// (not on their own screen); and the funding meter when the instance shows it here, which leads to the costs page.
// The server's pages end it, as they end the menu's sheet. Under `wide`, a line above the home timeline counts the
// unread announcements instead.

import { costsPagePath, eventPath, RoomCaption, roomIcon, useEventTime, voiceRoomPath } from "@my/app-components";
import { FundingMeterView, LatestNotifications } from "@my/app-features";
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
  RelativeTime,
  SectionLine,
  SectionList,
} from "@my/design-system";
import { useStrings } from "../locale";

/** The links to the server's pages: about, and the terms and the privacy policy when the instance has them. */
export function ServerFooter({ instance }: { instance: Instance | undefined }) {
  const t = useStrings();
  return (
    <FrameFooter
      label={t("app.footer")}
      links={[
        { href: "/about", label: t("app.footer_about"), icon: "about" },
        ...(instance?.terms_url === undefined
          ? []
          : [{ href: instance.terms_url, label: t("app.footer_terms"), icon: "file" as const }]),
        ...(instance?.privacy_policy_url === undefined
          ? []
          : [{ href: instance.privacy_policy_url, label: t("app.footer_privacy"), icon: "file" as const }]),
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

/** An event of the column: its title and when it starts, or, while its date is polled, that it is (as its list says). */
function EventLine({ event }: { event: ServerEvent }) {
  const t = useStrings();
  const time = useEventTime(event);
  return (
    <SectionLine
      href={eventPath(event.id)}
      title={event.title}
      caption={event.state === "unscheduled" ? t("app.event_date_polled") : time}
    />
  );
}

function RoomLine({ room }: { room: VoiceRoom }) {
  return (
    <SectionLine
      href={voiceRoomPath(room.id)}
      title={room.name}
      // A room without an icon emoji shows the room's glyph, as its line in the list does.
      {...(room.icon === undefined ? { icon: "room" as const } : { glyph: roomIcon(room.icon) })}
      caption={<RoomCaption room={room} />}
    />
  );
}

/**
 * The funding meter of this month, as the costs page shows it; it leads to the costs page, whose month's costs it is
 * of (FR-905).
 */
function Funding({ meter }: { meter: FundingMeter }) {
  const t = useStrings();
  return (
    <FrameSection id="around-funding" icon="funding" href={costsPagePath} heading={t("app.funding")}>
      <FundingMeterView meter={meter} />
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
