// SPDX-License-Identifier: AGPL-3.0-only
// The page of a room (/voice/{id}, ADR-0100 §9): to a signed-in user who sees the room, the stage (VoiceStage): the
// room's head with its name as the page's heading, the speakers and the listeners, and the dock of the call; on wide
// screens the third column adds the raised hands and about the room (VoiceRoomAside). To a visitor who does not see
// the room, its summary: that only the users of this server join, with the way to sign in; a room that ended says
// so, by its name when the summary has one, with the way to the rooms open now.
import { ButtonLink, Callout, Link, RegionError, Stack, Text } from "@my/ui";
import type { ReactNode } from "react";
import { useT } from "../../../lib/i18n";
import { signInPath } from "../../../lib/paths";
import { voiceRoomPath } from "../api/voice";
import { useVoiceRoomPage } from "../hooks/useVoiceRoomPage";
import { RoomName } from "./RoomName";
import { AccountLink } from "./VoiceRoomAside";
import { VoiceStageSkeleton } from "./VoiceSkeletons";
import { VoiceStage } from "./VoiceStage";

export type VoiceRoomViewProps = {
  id: string;
  /** Renders the page heading (the route's level-one heading) around its text. */
  renderHeading: (text: ReactNode) => ReactNode;
};

export function VoiceRoomView({ id, renderHeading }: VoiceRoomViewProps) {
  const t = useT();
  const page = useVoiceRoomPage(id);
  switch (page.status) {
    case "loading":
      return (
        <>
          {renderHeading(t("client:voice.room.title"))}
          <VoiceStageSkeleton />
        </>
      );
    case "error":
      return (
        <>
          {renderHeading(t("client:voice.room.title"))}
          <RegionError
            heading={t("client:voice.room.failed")}
            reason={page.reason}
            retryLabel={t("client:voice.list.retry")}
            onRetry={page.retry}
            isRetrying={page.isRetrying}
          />
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
          <Stack gap="3">
            {page.name !== undefined && (
              <Text as="p" dir="auto" wrap="anywhere">
                {page.name}
              </Text>
            )}
            {/* The next step: the rooms open now, where the user may also start one. */}
            <Text as="p">{t("client:voice.room.ended.next")}</Text>
            <div>
              <ButtonLink href="/voice" variant="primary">
                {t("client:voice.rail.all")}
              </ButtonLink>
            </div>
          </Stack>
        </>
      );
    case "summary": {
      const { summary } = page;
      return (
        <>
          {renderHeading(
            summary.name === undefined ? (
              t("client:voice.room.title")
            ) : (
              <RoomName name={summary.name} icon={summary.icon} />
            ),
          )}
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
                <Link href={signInPath(voiceRoomPath(id))} variant="standalone">
                  {t("client:voice.room.sign_in")}
                </Link>
              </Callout>
            )}
          </Stack>
        </>
      );
    }
    case "room":
      return <VoiceStage room={page.room} isHost={page.isHost} renderHeading={renderHeading} />;
  }
}
