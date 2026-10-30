// SPDX-License-Identifier: AGPL-3.0-only
// The lists of open rooms (ADR-0100 §8): the list page's body with the button that starts a room, the panel of the
// third column on wide screens, and the section at the top of a hashtag's page. Each room is a card in the look of its
// stage: the "Live" pill with how many people are in it and how long it has been open ("Joined" for the room of the
// user's call), its name after its emoji icon (a link to its page), the stacked avatars of its speakers (the host first) beside the host,
// and on the pages its description and hashtags (links to their pages). Every list shows nothing while the server has
// voice chat off.
import {
  AsidePanel,
  type AvatarStackItem,
  Button,
  EmptyState,
  Link,
  MicrophoneIcon,
  RegionError,
  Section,
  Stack,
  Text,
  VoiceIcon,
  VoiceRoomCard,
} from "@my/ui";
import { useState } from "react";
import { useT } from "../../../lib/i18n";
import { imageSources } from "../../../lib/images";
import { profilePath } from "../../../lib/paths";
import { hashtagPath, type VoiceAccount, type VoiceRoom, voiceRoomPath } from "../api/voice";
import { useJoinedVoiceRoomId } from "../hooks/useVoiceCallControls";
import { useCanStartVoiceRoom, useTagVoiceRooms, useVoiceRail, useVoiceRooms } from "../hooks/useVoiceRooms";
import { useOpenFor } from "../hooks/useVoiceStage";
import { RoomIcon } from "./RoomName";
import { VoiceRoomFormDialog } from "./VoiceRoomFormDialog";
import { VoiceRoomsSkeleton } from "./VoiceSkeletons";

type VoiceRoomItemProps = { room: VoiceRoom; headingLevel: 2 | 3; surface?: "card" | "plain" };

/** The name an account shows: its display name, or its username without one. */
function accountName(account: VoiceAccount): string {
  return account.display_name || account.username;
}

/** The avatar of an account in a room's stack. */
function stackItem(account: VoiceAccount): AvatarStackItem {
  return { id: account.id, name: accountName(account), sources: imageSources(account.avatar) };
}

/**
 * The speakers to stack on a room's card: the host first, then the co-hosts, then the other speakers, each in the
 * order they joined.
 */
function speakerStack(room: VoiceRoom): AvatarStackItem[] {
  const others = room.participants.filter(
    (participant) => participant.role === "speaker" && participant.account.id !== room.host.id,
  );
  const ordered = [
    ...others.filter((participant) => participant.cohost),
    ...others.filter((participant) => !participant.cohost),
  ];
  return [stackItem(room.host), ...ordered.map((participant) => stackItem(participant.account))];
}

/** One room as an `li` of a list. */
function VoiceRoomItem({ room, headingLevel, surface = "card" }: VoiceRoomItemProps) {
  const t = useT();
  const openFor = useOpenFor(room.opened_at);
  const joinedRoomId = useJoinedVoiceRoomId();
  return (
    <VoiceRoomCard
      name={room.name}
      icon={room.icon && <RoomIcon icon={room.icon} />}
      href={voiceRoomPath(room.id)}
      headingLevel={headingLevel}
      surface={surface}
      liveLabel={t("client:voice.room.live")}
      stats={t("client:voice.room.stats", {
        people: t("client:voice.room.participants_count", { count: room.participants_count }),
        duration: openFor,
      })}
      joinedLabel={joinedRoomId === room.id ? t("client:voice.room.joined_badge") : undefined}
      speakers={speakerStack(room)}
      host={
        <Link href={profilePath(room.host.acct)} variant="standalone">
          <span dir="auto">{accountName(room.host)}</span>
        </Link>
      }
      hostLabel={t("client:voice.role.host")}
      description={room.description}
      tags={room.hashtags.map((tag) => ({ label: t("client:tag.title", { tag }), href: hashtagPath(tag) }))}
      tagsLabel={t("client:voice.form.hashtags")}
    />
  );
}

/**
 * The list page's body (`/voice`): the button that starts a room, for a user who may open rooms, then the open rooms the
 * user sees.
 */
export function VoiceRoomList() {
  const t = useT();
  const canStart = useCanStartVoiceRoom();
  const [formOpen, setFormOpen] = useState(false);
  return (
    <Stack gap="6">
      {canStart && (
        <div>
          <Button
            variant="primary"
            onPress={() => {
              setFormOpen(true);
            }}
          >
            <MicrophoneIcon />
            {t("client:voice.start")}
          </Button>
          <VoiceRoomFormDialog isOpen={formOpen} onOpenChange={setFormOpen} />
        </div>
      )}
      <Rooms />
    </Stack>
  );
}

/** The open rooms of the list page, with its states. */
function Rooms() {
  const t = useT();
  const list = useVoiceRooms();
  switch (list.status) {
    case "unavailable":
      return <EmptyState heading={t("client:voice.unavailable")} />;
    case "loading":
      return <VoiceRoomsSkeleton />;
    case "error":
      return (
        <RegionError
          heading={t("client:voice.list.failed")}
          reason={list.reason}
          retryLabel={t("client:voice.list.retry")}
          onRetry={list.retry}
          isRetrying={list.isRetrying}
        />
      );
    case "ready":
      return (
        <>
          {list.rooms.length === 0 ? (
            <EmptyState
              illustration={<VoiceIcon />}
              heading={t("client:voice.list.empty")}
              description={t("client:voice.list.empty.description")}
            />
          ) : (
            <Stack as="ul" gap="3" aria-label={t("client:voice.list.label")}>
              {list.rooms.map((room) => (
                <VoiceRoomItem key={room.id} room={room} headingLevel={2} />
              ))}
            </Stack>
          )}
          {list.moreError !== undefined && (
            <RegionError heading={t("client:voice.list.more_failed")} reason={list.moreError} />
          )}
          {list.hasMore && (
            <div>
              <Button onPress={list.loadMore} isPending={list.isLoadingMore}>
                {t("client:voice.list.more")}
              </Button>
            </div>
          )}
        </>
      );
  }
}

/**
 * The panel of the third column (web-client/09-app-shell.md §9): the first rooms of the list and the link to all of
 * them. Absent while voice chat is off and while no room is open.
 */
export function VoiceRail() {
  const t = useT();
  const rooms = useVoiceRail();
  if (rooms.length === 0) return null;
  return (
    <AsidePanel heading={t("client:voice.rail.title")}>
      <Stack as="ul" gap="4">
        {rooms.map((room) => (
          <VoiceRoomItem key={room.id} room={room} headingLevel={3} surface="plain" />
        ))}
      </Stack>
      <Text as="p" size="sm">
        <Link href="/voice" variant="standalone">
          {t("client:voice.rail.all")}
        </Link>
      </Text>
    </AsidePanel>
  );
}

/** The open rooms with `tag`, at the top of the hashtag's page (回答 10); absent when there are none. */
export function TagVoiceRooms({ tag }: { tag: string }) {
  const t = useT();
  const rooms = useTagVoiceRooms(tag);
  if (rooms.length === 0) return null;
  // The hashtag is the user's text, in any script (web-client/README.md Y10).
  return (
    <Section title={<span dir="auto">{t("client:voice.tag.title", { tag })}</span>}>
      <Stack as="ul" gap="3">
        {rooms.map((room) => (
          <VoiceRoomItem key={room.id} room={room} headingLevel={3} />
        ))}
      </Stack>
    </Section>
  );
}
