// SPDX-License-Identifier: AGPL-3.0-only
// The lists of open rooms (ADR-0100 §8): the list page's body with the button that starts a room, the panel of the
// third column on wide screens, and the section at the top of a hashtag's page. Each room is its name (a link to its
// page), its host, how many people are in it, and on the pages its description and hashtags. Every list shows
// nothing while the server has voice chat off.
import { AccountItem, AsidePanel, Button, EmptyState, Heading, Link, RegionError, Spinner, Stack, Text, VoiceRoomCard } from "@my/ui";
import { useId, useState } from "react";
import { handleOf } from "../../../lib/handle";
import { useT } from "../../../lib/i18n";
import { imageSources } from "../../../lib/images";
import { profilePath } from "../../../lib/paths";
import { type VoiceRoom, voiceRoomPath } from "../api/voice";
import { useCanStartVoiceRoom, useTagVoiceRooms, useVoiceRail, useVoiceRooms } from "../hooks/useVoiceRooms";
import { VoiceRoomFormDialog } from "./VoiceRoomFormDialog";

type VoiceRoomItemProps = { room: VoiceRoom; headingLevel: 2 | 3; surface?: "card" | "plain" };

/** One room as an `li` of a list. */
function VoiceRoomItem({ room, headingLevel, surface = "card" }: VoiceRoomItemProps) {
  const t = useT();
  const hostName = room.host.display_name || room.host.username;
  return (
    <VoiceRoomCard
      name={room.name}
      href={voiceRoomPath(room.id)}
      headingLevel={headingLevel}
      surface={surface}
      host={
        <AccountItem
          name={hostName}
          handle={handleOf(room.host.acct)}
          avatar={imageSources(room.host.avatar)}
          avatarSeed={room.host.id}
          href={profilePath(room.host.acct)}
          layout="inline"
        />
      }
      participants={t("client:voice.room.participants_count", { count: room.participants_count })}
      description={room.description}
      tags={room.hashtags.map((tag) => t("client:tag.title", { tag }))}
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
      return (
        <div aria-busy="true">
          <Spinner label={t("client:voice.list.loading")} />
        </div>
      );
    case "error":
      return (
        <RegionError heading={t("client:voice.list.failed")} reason={list.reason} retryLabel={t("client:voice.list.retry")} onRetry={list.retry} isRetrying={list.isRetrying} />
      );
    case "ready":
      return (
        <>
          {list.rooms.length === 0 ? (
            <EmptyState heading={t("client:voice.list.empty")} description={t("client:voice.list.empty.description")} />
          ) : (
            <Stack as="ul" gap="3" aria-label={t("client:voice.list.label")}>
              {list.rooms.map((room) => (
                <VoiceRoomItem key={room.id} room={room} headingLevel={2} />
              ))}
            </Stack>
          )}
          {list.moreError !== undefined && <RegionError heading={t("client:voice.list.more_failed")} reason={list.moreError} />}
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
      <Stack as="ul" gap="3">
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
  const headingId = useId();
  const rooms = useTagVoiceRooms(tag);
  if (rooms.length === 0) return null;
  return (
    <Stack as="section" gap="3" aria-labelledby={headingId}>
      <Heading level={2} id={headingId}>
        {/* The hashtag is the user's text, in any script (web-client/README.md Y10). */}
        <span dir="auto">{t("client:voice.tag.title", { tag })}</span>
      </Heading>
      <Stack as="ul" gap="3">
        {rooms.map((room) => (
          <VoiceRoomItem key={room.id} room={room} headingLevel={3} />
        ))}
      </Stack>
    </Stack>
  );
}
