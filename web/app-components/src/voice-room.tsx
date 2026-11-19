// SPDX-License-Identifier: AGPL-3.0-only

// A voice chat room as a line of a list (ADR-0100 §8, FR-928): the rooms' screen, a hashtag's page and the entry of a
// profile draw it alike, the design system's RoomLine, from the room the API sends (VoiceRoom) or the entry of an
// account's profile (VoiceRoomEntry), whose shapes are written here: the package depends on no API client.

import { CustomEmoji, HeadcountMark, RoomLine, Row, useLocale, type Headcount } from "@my/design-system";
import type { ReactNode } from "react";
import { nameOf } from "./account";
import { stringsIn } from "./locale";
import { voiceRoomPath } from "./paths";
import type { PostAccount } from "./status";

/** The emoji before a room's name (the API's VoiceRoomIcon): a Unicode emoji, or a custom emoji with its image. */
export type RoomIcon = {
  name: string;
  emoji?: { shortcode: string; url: string; static_url: string } | undefined;
};

/** An open room, as its line reads it (the API's VoiceRoom). */
export type RoomShown = {
  id: string;
  name: string;
  icon?: RoomIcon | undefined;
  host: PostAccount;
  participants_count: number;
  participants: readonly { account: PostAccount; role: "speaker" | "listener"; hosting: boolean }[];
};

/** The open room where an account hosts or speaks, as its profile shows it (the API's VoiceRoomEntry). */
export type RoomEntryShown = {
  id: string;
  name: string;
  icon?: RoomIcon | undefined;
  role: "host" | "speaker";
  participants_count: number;
};

/** The icon of a room as it is drawn: its Unicode emoji, or its custom emoji's image; nothing for none. */
export function roomIcon(icon: RoomIcon | undefined): ReactNode {
  if (icon === undefined) return undefined;
  if (icon.emoji === undefined) return icon.name;
  return <CustomEmoji shortcode={icon.emoji.shortcode} url={icon.emoji.url} staticUrl={icon.emoji.static_url} />;
}

/** How many are in a room, as its figure and its words, in the reader's locale. */
export function useHeadcount(count: number): Headcount {
  const { locale } = useLocale();
  const { t } = stringsIn(locale);
  return { shown: new Intl.NumberFormat(locale).format(count), label: t("room.count", { count }) };
}

/**
 * A room going on as a line that leads to it: its icon, its name, who hosts it, how many are in it, and the faces of
 * who speaks, the host first.
 */
export function VoiceRoomLine({ room }: { room: RoomShown }) {
  const { t } = stringsIn(useLocale().locale);
  const count = useHeadcount(room.participants_count);
  const speakers = room.participants
    .filter((p) => p.role === "speaker")
    .toSorted((a, b) => Number(b.hosting) - Number(a.hosting))
    .map((p) => ({ name: nameOf(p.account), src: p.account.avatar?.small.url }));
  return (
    <RoomLine
      href={voiceRoomPath(room.id)}
      name={room.name}
      icon={roomIcon(room.icon)}
      caption={t("room.hosted_by", { host: nameOf(room.host) })}
      count={count}
      speakers={speakers}
    />
  );
}

/**
 * Who hosts a room and how many are in it, as a line under its name in a short list (the column beside the main one):
 * the same words and headcount as a room's line of the rooms' screen.
 */
export function RoomCaption({ room }: { room: Pick<RoomShown, "host" | "participants_count"> }) {
  const { t } = stringsIn(useLocale().locale);
  return (
    <Row as="span" inline wrap align="baseline" gapX="entry-caption">
      <span>{t("room.hosted_by", { host: nameOf(room.host) })}</span>
      <HeadcountMark count={useHeadcount(room.participants_count)} />
    </Row>
  );
}

/** The room where the account of a profile hosts or speaks now, as a line that leads to it. */
export function VoiceRoomEntryLine({ entry }: { entry: RoomEntryShown }) {
  const { t } = stringsIn(useLocale().locale);
  const count = useHeadcount(entry.participants_count);
  return (
    <RoomLine
      href={voiceRoomPath(entry.id)}
      name={entry.name}
      icon={roomIcon(entry.icon)}
      caption={t(entry.role === "host" ? "room.entry_hosting" : "room.entry_speaking")}
      count={count}
    />
  );
}
