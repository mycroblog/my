// SPDX-License-Identifier: AGPL-3.0-only

// The places of an event on its page (ADR-0161 §6, §10, FR-942, FR-951, FR-952), the voice chat room first: a place in
// person with its name, its address and its map; a place online by its name; each link leaving the site said so and
// where. A place the reader does not see (shown to the attendees the organizers approve) shows its kind alone, and
// that. The voice room says where it stands: before the start, that it opens then; while it opens, the event is read
// again until it says what came of it; once opened, that it is open, its way in at the frame's foot (joining from the
// press, as the room's own Join does), or that it ended; why it could not open.

import {
  eventRoom,
  useVoiceRoom,
  type EventPlace,
  type EventPlaceKind,
  type EventRoomUnavailableReason,
  type ServerEvent,
} from "@my/app-data";
import { useHeadcount } from "@my/app-components";
import { Box, Button, FrameFoot, HeadcountMark, Icon, PlaceLine, PlaceList, Row, Stack } from "@my/design-system";
import { ProblemError } from "@mycroblog/mykit";
import { useEffect, useRef, useState } from "react";
import type { T } from "./parts";

/** Joins the open voice chat room of `roomId` from the reader's press, then shows it; rejects when the join failed. */
export type JoinRoom = (roomId: string) => Promise<void>;

/** What a part of the page says once, in the page's live region. */
export type Say = (words: string) => void;

const order: readonly EventPlaceKind[] = ["voice", "physical", "url"];

/** The host a place's URL leads to; the whole URL where it has none. */
function hostOf(url: string): string {
  try {
    return new URL(url).host || url;
  } catch {
    return url;
  }
}

/** Whether the room of `roomId` ended: the server no longer has it open (its 404). */
function useRoomEnded(roomId: string): boolean {
  const { error } = useVoiceRoom(roomId);
  return error instanceof ProblemError && error.status === 404;
}

/** The open room of the voice place, as a place: where it stands, its way in at the frame's foot (`RoomFoot`). */
function OpenRoom({ roomId, t }: { roomId: string; t: T }) {
  const ended = useRoomEnded(roomId);
  return (
    <PlaceLine
      kind="voice"
      label={t("events.place_voice")}
      note={t(ended ? "events.room_ended" : "events.room_open")}
    />
  );
}

/**
 * The way into the event's open room, the page's one action at the frame's foot, where the thumb is, as the room's
 * own page has it: the room's glyph and name with how many are in it, then Join, which joins from the press. Nothing
 * once the room ended.
 */
export function RoomFoot({ roomId, join, say, t }: { roomId: string; join: JoinRoom; say: Say; t: T }) {
  const { room } = useVoiceRoom(roomId);
  const ended = useRoomEnded(roomId);
  const count = useHeadcount(room?.participants_count ?? 0);
  const [busy, setBusy] = useState(false);
  if (ended) return null;
  return (
    <FrameFoot>
      <Stack gap="entry-line">
        <Row align="center" gap="entry-caption">
          <Icon name="room" place="action" />
          <Box grow text="body" dir="auto">
            {room?.name ?? t("events.place_voice")}
          </Box>
          {room !== undefined && <HeadcountMark count={count} />}
        </Row>
        <Stack>
          <Button
            kind="primary"
            size="form"
            busy={busy}
            onPress={() => {
              if (busy) return;
              setBusy(true);
              // Inside the press: the call's sound starts there, or the browser holds it back until another.
              join(roomId).then(
                () => {
                  setBusy(false);
                },
                () => {
                  setBusy(false);
                  say(t("events.join_failed"));
                },
              );
            }}
          >
            {t("events.join")}
          </Button>
        </Stack>
      </Stack>
    </FrameFoot>
  );
}

/** How often the page reads the event again while its room is opening: the server's tick opens it within a minute. */
const openingReadMs = 10_000;

/** The voice place while its room is opening: the event is read again until it says what came of it. */
function Opening({ label, refetch, t }: { label: string; refetch: () => void; t: T }) {
  // The read of this render, which the interval calls: a new function each render must not restart the interval.
  const read = useRef(refetch);
  useEffect(() => {
    read.current = refetch;
  });
  useEffect(() => {
    const timer = setInterval(() => {
      read.current();
    }, openingReadMs);
    return () => {
      clearInterval(timer);
    };
  }, []);
  return <PlaceLine kind="voice" label={label} note={t("events.room_opening")} />;
}

/** The voice place: where its room stands at `now`. */
function VoicePlace({ event, now, refetch, t }: { event: ServerEvent; now: number; refetch: () => void; t: T }) {
  const room = eventRoom(event, now);
  const label = t("events.place_voice");
  switch (room.state) {
    case "open":
      return <OpenRoom roomId={room.roomId} t={t} />;
    case "opens":
      return <PlaceLine kind="voice" label={label} note={t("events.room_opens")} />;
    case "opening":
      return <Opening label={label} refetch={refetch} t={t} />;
    case "unavailable":
      return (
        <PlaceLine
          kind="voice"
          label={label}
          unavailable
          note={t("events.room_unavailable", { reason: room.reason satisfies EventRoomUnavailableReason })}
        />
      );
    case "none":
      return <PlaceLine kind="voice" label={label} note={t("events.room_cancelled")} />;
  }
}

/** A place in person or online, as the reader sees it. */
function ShownPlace({ place, t }: { place: EventPlace; t: T }) {
  if (place.kind === "physical")
    return (
      <PlaceLine
        kind="physical"
        label={t("events.place_physical")}
        name={place.name}
        address={place.address}
        link={
          place.url === undefined || place.url === ""
            ? undefined
            : { href: place.url, text: t("events.map"), host: hostOf(place.url) }
        }
      />
    );
  const url = place.url ?? "";
  return (
    <PlaceLine
      kind="url"
      label={t("events.place_url")}
      name={place.name}
      link={{ href: url, text: place.name ?? url, host: hostOf(url) }}
    />
  );
}

/** Whether `event` has a place to show, seen or not. */
export const hasPlaces = (event: ServerEvent) => event.places.length > 0 || event.hidden_places.length > 0;

/**
 * The places of `event` at `now`, the voice room first, each the reader does not see by its kind alone; `refetch`
 * reads the event again, while its room is opening.
 */
export function Places({ event, now, refetch, t }: { event: ServerEvent; now: number; refetch: () => void; t: T }) {
  return (
    <PlaceList label={t("events.where")}>
      {order.map((kind) => {
        const place = event.places.find((p) => p.kind === kind);
        if (place !== undefined)
          return kind === "voice" ? (
            <VoicePlace key={kind} event={event} now={now} refetch={refetch} t={t} />
          ) : (
            <ShownPlace key={kind} place={place} t={t} />
          );
        if (!event.hidden_places.includes(kind)) return null;
        return <PlaceLine key={kind} kind={kind} label={t("events.place_hidden", { kind })} hidden />;
      })}
    </PlaceList>
  );
}
