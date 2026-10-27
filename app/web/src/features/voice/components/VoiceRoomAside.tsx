// SPDX-License-Identifier: AGPL-3.0-only
// The third column of a room's page on wide screens (web-client/09-app-shell.md §9): for the host and the co-hosts in
// the call, the raised hands with Allow and Dismiss for each (below xl, the dock's Hands button opens the same list);
// and "About the room": its host, who sees and joins it, and who speaks without asking, with the host's button to
// the room's settings. Each block is an `aside` named by its heading.
import { AccountItem, AsidePanel, Button, Cluster, DescriptionItem, DescriptionList, Hint, InlineError, Stack } from "@my/ui";
import { useState } from "react";
import { handleOf } from "../../../lib/handle";
import { useT } from "../../../lib/i18n";
import { imageSources } from "../../../lib/images";
import { profilePath } from "../../../lib/paths";
import type { VoiceAccount, VoiceRoom, VoiceRoomParticipant } from "../api/voice";
import { useVoiceCall } from "../hooks/useVoiceCall";
import { participantOperations, useVoiceModeration, type VoiceModeration } from "../hooks/useVoiceModeration";
import { useVoiceRoomPage } from "../hooks/useVoiceRoomPage";
import { raisedHands } from "../hooks/useVoiceStage";
import { participantName } from "../hooks/voiceCall";
import { speakerLabels, visibilityLabels, VoiceRoomFormDialog } from "./VoiceRoomFormDialog";

type AccountLike = Pick<VoiceAccount, "id" | "acct" | "username" | "display_name" | "avatar">;

/** An account as a link to its profile, as wide as its text. */
export function AccountLink({ account }: { account: AccountLike }) {
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

export type RaisedHandListProps = {
  /** The participants with a raised hand. */
  hands: readonly VoiceRoomParticipant[];
  /** The user in the call, whose rights decide the buttons. */
  self: VoiceRoomParticipant | undefined;
  moderation: VoiceModeration;
};

/** The raised hands, each with the buttons that let them speak or dismiss the hand; a line when there are none. */
export function RaisedHandList({ hands, self, moderation }: RaisedHandListProps) {
  const t = useT();
  if (hands.length === 0) return <Hint>{t("client:voice.hands.empty")}</Hint>;
  return (
    <Stack as="ul" gap="3">
      {hands.map((participant) => {
        const operations = participantOperations(self, participant);
        const name = participantName(participant);
        const pending = moderation.pending?.accountId === participant.account.id ? moderation.pending.operation : undefined;
        return (
          <li key={participant.account.id}>
            <AccountItem
              name={name}
              handle={handleOf(participant.account.acct)}
              avatar={imageSources(participant.account.avatar)}
              avatarSeed={participant.account.id}
              href={profilePath(participant.account.acct)}
              trailingPlacement="below"
              trailing={
                <Cluster gap="2">
                  {operations.includes("dismiss") && (
                    <Button
                      variant="ghost"
                      size="sm"
                      aria-label={t("client:voice.hands.dismiss.label", { name })}
                      isPending={pending === "dismiss"}
                      onPress={() => {
                        moderation.operate(participant, "dismiss");
                      }}
                    >
                      {t("client:voice.hands.dismiss")}
                    </Button>
                  )}
                  {operations.includes("grant") && (
                    <Button
                      variant="primary"
                      size="sm"
                      aria-label={t("client:voice.hands.allow.label", { name })}
                      isPending={pending === "grant"}
                      onPress={() => {
                        moderation.operate(participant, "grant");
                      }}
                    >
                      {t("client:voice.hands.allow")}
                    </Button>
                  )}
                </Cluster>
              }
            />
          </li>
        );
      })}
    </Stack>
  );
}

/** The blocks of the third column on the page of the room `id`, while the page shows the room. */
export function VoiceRoomAside({ id }: { id: string }) {
  const page = useVoiceRoomPage(id);
  if (page.status !== "room") return null;
  return <RoomAside room={page.room} isHost={page.isHost} />;
}

function RoomAside({ room, isHost }: { room: VoiceRoom; isHost: boolean }) {
  const t = useT();
  const call = useVoiceCall(room);
  return (
    <>
      {call.status === "in-call" && call.manages && <RaisedHandsPanel room={room} self={call.self} />}
      <AsidePanel heading={t("client:voice.room.details")}>
        <DescriptionList>
          <DescriptionItem term={t("client:voice.room.hosted_by")}>
            <AccountLink account={room.host} />
          </DescriptionItem>
          <DescriptionItem term={t("client:voice.form.visibility")}>{t(visibilityLabels[room.visibility])}</DescriptionItem>
          <DescriptionItem term={t("client:voice.form.speakers")}>{t(speakerLabels[room.speakers])}</DescriptionItem>
        </DescriptionList>
        {isHost && <SettingsButton room={room} />}
      </AsidePanel>
    </>
  );
}

function RaisedHandsPanel({ room, self }: { room: VoiceRoom; self: VoiceRoomParticipant | undefined }) {
  const t = useT();
  const moderation = useVoiceModeration(room);
  const hands = raisedHands(room);
  return (
    <AsidePanel heading={t("client:voice.hands.heading", { count: hands.length })}>
      {moderation.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:voice.call.action_failed", { reason: moderation.error })}
        </InlineError>
      )}
      <RaisedHandList hands={hands} self={self} moderation={moderation} />
    </AsidePanel>
  );
}

function SettingsButton({ room }: { room: VoiceRoom }) {
  const t = useT();
  const [isOpen, setIsOpen] = useState(false);
  return (
    <div>
      <Button
        size="sm"
        onPress={() => {
          setIsOpen(true);
        }}
      >
        {t("client:voice.room.change_settings")}
      </Button>
      <VoiceRoomFormDialog room={room} isOpen={isOpen} onOpenChange={setIsOpen} />
    </div>
  );
}
