// SPDX-License-Identifier: AGPL-3.0-only
// The page of a room (/voice/{id}, ADR-0100 §9). Before joining: the room's name as the page's heading, its host,
// description, hashtags and settings, the notice that joining lists the user's name with the join button (回答 31),
// the host's button to its settings, and everyone in it by name, speakers and listeners alike, each with their role.
// The audio of the call is not here. To a visitor who does not see the room, its summary: that only the users of this
// server join, with the way to sign in; a room that ended says so, by its name when the summary has one.
import { AccountItem, Button, Callout, Cluster, DescriptionItem, DescriptionList, Heading, Hint, InlineError, Link, RegionError, Spinner, Stack, Tag, Text } from "@my/ui";
import { type ReactNode, useId, useState } from "react";
import { handleOf } from "../../../lib/handle";
import { useT } from "../../../lib/i18n";
import { imageSources } from "../../../lib/images";
import { profilePath } from "../../../lib/paths";
import { type VoiceAccount, type VoiceRoom, type VoiceRoomParticipant, voiceRoomPath } from "../api/voice";
import { useVoiceRoomPage } from "../hooks/useVoiceRoomPage";
import { speakerLabels, visibilityLabels, VoiceRoomFormDialog } from "./VoiceRoomFormDialog";

export type VoiceRoomViewProps = {
  id: string;
  /** Renders the page heading (the route's level-one heading) around its text. */
  renderHeading: (text: ReactNode) => ReactNode;
};

type AccountLike = Pick<VoiceAccount, "id" | "acct" | "username" | "display_name" | "avatar">;

/** An account as a link to its profile, as wide as its text. */
function AccountLink({ account }: { account: AccountLike }) {
  return (
    <AccountItem
      name={account.display_name || account.username}
      handle={handleOf(account.acct)}
      avatar={imageSources(account.avatar)}
      avatarSeed={account.id}
      href={profilePath(account.acct)}
      layout="inline"
    />
  );
}

export function VoiceRoomView({ id, renderHeading }: VoiceRoomViewProps) {
  const t = useT();
  const page = useVoiceRoomPage(id);
  switch (page.status) {
    case "loading":
      return (
        <>
          {renderHeading(t("client:voice.room.title"))}
          <div aria-busy="true">
            <Spinner label={t("client:voice.room.loading")} />
          </div>
        </>
      );
    case "error":
      return (
        <>
          {renderHeading(t("client:voice.room.title"))}
          <RegionError heading={t("client:voice.room.failed")} reason={page.reason} retryLabel={t("client:voice.list.retry")} onRetry={page.retry} isRetrying={page.isRetrying} />
        </>
      );
    case "not-found":
      return (
        <>
          {renderHeading(t("client:voice.room.not_found"))}
          <Text as="p">
            <Link href="/voice" variant="standalone">
              {t("client:voice.rail.all")}
            </Link>
          </Text>
        </>
      );
    case "ended":
      return (
        <>
          {renderHeading(t("client:voice.room.ended"))}
          {page.name !== undefined && (
            <Text as="p" dir="auto" wrap="anywhere">
              {page.name}
            </Text>
          )}
        </>
      );
    case "summary": {
      const { summary } = page;
      return (
        <>
          {renderHeading(summary.name === undefined ? t("client:voice.room.title") : <span dir="auto">{summary.name}</span>)}
          <Stack gap="4">
            {summary.host !== undefined && <AccountLink account={summary.host} />}
            {summary.description !== undefined && summary.description !== "" && (
              <Text as="p" whitespace="preLine" dir="auto" wrap="anywhere">
                {summary.description}
              </Text>
            )}
            {page.signedIn ? (
              <Text as="p">{t("client:voice.room.hidden")}</Text>
            ) : (
              <Callout>
                <p>{t("client:voice.room.open")}</p>
                <p>{t("client:voice.room.local_only")}</p>
                <Link href={`/login?${new URLSearchParams({ return_to: voiceRoomPath(id) }).toString()}`} variant="standalone">
                  {t("client:voice.room.sign_in")}
                </Link>
              </Callout>
            )}
          </Stack>
        </>
      );
    }
    case "room":
      return (
        <>
          {renderHeading(<span dir="auto">{page.room.name}</span>)}
          <RoomBody room={page.room} isHost={page.isHost} />
          <Callout>
            {page.joined ? (
              <p>{t("client:voice.room.joined")}</p>
            ) : (
              <>
                <p>{t("client:voice.room.listed_notice")}</p>
                <Button variant="primary" onPress={page.join} isPending={page.isJoining}>
                  {t("client:voice.room.join")}
                </Button>
              </>
            )}
          </Callout>
          {page.joinError !== undefined && (
            <InlineError size="inherit" alert>
              {t("client:voice.room.join_failed", { reason: page.joinError })}
            </InlineError>
          )}
          <Participants participants={page.room.participants} />
        </>
      );
  }
}

/** The room's host, description, hashtags and settings, and the host's button to change them. */
function RoomBody({ room, isHost }: { room: VoiceRoom; isHost: boolean }) {
  const t = useT();
  const [formOpen, setFormOpen] = useState(false);
  return (
    <Stack gap="4">
      <Cluster gap="3" align="center">
        <Hint as="span">{t("client:voice.room.hosted_by")}</Hint>
        <AccountLink account={room.host} />
      </Cluster>
      {room.description !== "" && (
        <Text as="p" whitespace="preLine" dir="auto" wrap="anywhere">
          {room.description}
        </Text>
      )}
      {room.hashtags.length > 0 && (
        <Cluster as="ul" gap="1" aria-label={t("client:voice.form.hashtags")}>
          {room.hashtags.map((tag) => (
            <Tag key={tag} as="li" variant="pill" dir="auto">
              <Link href={`/tags/${encodeURIComponent(tag)}`} variant="standalone">
                {t("client:tag.title", { tag })}
              </Link>
            </Tag>
          ))}
        </Cluster>
      )}
      <DescriptionList columns={2}>
        <DescriptionItem term={t("client:voice.form.visibility")}>{t(visibilityLabels[room.visibility])}</DescriptionItem>
        <DescriptionItem term={t("client:voice.form.speakers")}>{t(speakerLabels[room.speakers])}</DescriptionItem>
      </DescriptionList>
      {isHost && (
        <div>
          <Button
            onPress={() => {
              setFormOpen(true);
            }}
          >
            {t("client:voice.form.edit_title")}
          </Button>
          <VoiceRoomFormDialog room={room} isOpen={formOpen} onOpenChange={setFormOpen} />
        </div>
      )}
    </Stack>
  );
}

/** The label of a participant's place in the room: the host, a co-host, or a speaker or a listener. */
function roleLabel(participant: VoiceRoomParticipant) {
  if (participant.host) return "client:voice.role.host" as const;
  if (participant.cohost) return "client:voice.role.cohost" as const;
  return participant.role === "speaker" ? ("client:voice.role.speaker" as const) : ("client:voice.role.listener" as const);
}

/** Everyone in the room, in the order they joined, each with their role (回答 31). */
function Participants({ participants }: { participants: readonly VoiceRoomParticipant[] }) {
  const t = useT();
  const headingId = useId();
  return (
    <Stack as="section" gap="3" aria-labelledby={headingId}>
      <Heading level={2} id={headingId}>
        {t("client:voice.room.participants")}
      </Heading>
      <Hint>{t("client:voice.room.participants_count", { count: participants.length })}</Hint>
      <Stack as="ul" gap="3">
        {participants.map((participant) => (
          <li key={participant.account.id}>
            <AccountItem
              name={participant.account.display_name || participant.account.username}
              handle={handleOf(participant.account.acct)}
              avatar={imageSources(participant.account.avatar)}
              avatarSeed={participant.account.id}
              href={profilePath(participant.account.acct)}
              trailing={<Tag>{t(roleLabel(participant))}</Tag>}
            />
          </li>
        ))}
      </Stack>
    </Stack>
  );
}
