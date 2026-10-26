// SPDX-License-Identifier: AGPL-3.0-only
// The call of a room's page (ADR-0100 §5, §6, §10; ADR-0101 §7). Before joining: the notice that joining lists the
// user's name with the join button (回答 31), or, for a user in the room from another device, the button that joins
// here. In the call (the app's call, which goes on across pages with the call bar): the state of the connection, the
// user's microphone (a switch, only for a speaker), speaking or listening, raising a hand, reactions (the picker of
// ADR-0091), inviting (the host and the co-hosts), and leaving, where the host of a room with others in it chooses the
// next host or ends the room. Everyone in the room is listed
// with their role, a raised hand, a microphone that is on, the mark of a block between them and the user (回答 23),
// and the reactions they just sent; the host and the co-hosts have a menu of what they may do to each participant.
import {
  AccountItem,
  Button,
  Callout,
  Cluster,
  ConfirmDialog,
  CustomEmoji,
  Heading,
  Hint,
  InlineError,
  Menu,
  MenuItem,
  MenuTrigger,
  MoreIcon,
  Radio,
  RadioGroup,
  Stack,
  Tag,
  Text,
  TextField,
  VisuallyHidden,
} from "@my/ui";
import { useId, useState } from "react";
import { handleOf } from "../../../lib/handle";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { imageSources } from "../../../lib/images";
import { profilePath } from "../../../lib/paths";
import { ReactionPickerButton, useCustomEmojiImages } from "../../emoji";
import type { VoiceParticipantOperation, VoiceRole } from "../api/call";
import type { VoiceRoom, VoiceRoomParticipant } from "../api/voice";
import { type VoiceCallControl, useVoiceCall } from "../hooks/useVoiceCall";
import { useVoiceCallControls } from "../hooks/useVoiceCallControls";
import { callEndText } from "../hooks/useVoiceCallProvider";
import { participantOperations, useVoiceInvite, useVoiceModeration, type VoiceModeration } from "../hooks/useVoiceModeration";
import type { VoiceReactionShown } from "../hooks/voiceCall";
import { participantName } from "../hooks/voiceCall";
import { callPhaseText, LeaveCallButton, MicrophoneControl, PlaybackButton } from "./VoiceCallParts";

type InCall = Extract<VoiceCallControl, { status: "in-call" }>;

/** The label of a participant's place in the room: the host, a co-host, or a speaker or a listener. */
function roleLabel(participant: VoiceRoomParticipant): ClientMessageKey {
  if (participant.host) return "client:voice.role.host";
  if (participant.cohost) return "client:voice.role.cohost";
  return participant.role === "speaker" ? "client:voice.role.speaker" : "client:voice.role.listener";
}

const operationLabels: Readonly<Record<VoiceParticipantOperation, ClientMessageKey>> = {
  grant: "client:voice.moderate.grant",
  dismiss: "client:voice.moderate.dismiss",
  revoke: "client:voice.moderate.revoke",
  mute: "client:voice.moderate.mute",
  remove: "client:voice.moderate.remove",
  cohost: "client:voice.moderate.cohost",
  uncohost: "client:voice.moderate.uncohost",
  "hand-over": "client:voice.moderate.hand_over",
};

export function VoiceCallPanel({ room }: { room: VoiceRoom }) {
  const call = useVoiceCall(room);
  return (
    <>
      {call.status === "outside" ? <JoinCallout call={call} /> : <CallControls call={call} room={room} />}
      <Participants room={room} call={call.status === "in-call" ? call : undefined} />
    </>
  );
}

function JoinCallout({ call }: { call: Extract<VoiceCallControl, { status: "outside" }> }) {
  const t = useT();
  return (
    <>
      <Callout>
        {call.end !== undefined && <p>{callEndText(t, call.end)}</p>}
        {!call.supported ? (
          <p>{t("client:voice.call.unsupported_browser")}</p>
        ) : (
          <>
            <p>{call.inRoomElsewhere ? t("client:voice.room.joined") : t("client:voice.room.listed_notice")}</p>
            {call.end?.kind !== "removed" && (
              <Button variant="primary" onPress={call.join} isPending={call.isJoining}>
                {call.inRoomElsewhere || call.end?.kind === "replaced" ? t("client:voice.call.join_here") : t("client:voice.room.join")}
              </Button>
            )}
          </>
        )}
      </Callout>
      {call.joinError !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:voice.room.join_failed", { reason: call.joinError })}
        </InlineError>
      )}
    </>
  );
}

function CallControls({ call, room }: { call: InCall; room: VoiceRoom }) {
  const t = useT();
  const headingId = useId();
  const controls = useVoiceCallControls();
  return (
    <Stack as="section" gap="3" aria-labelledby={headingId}>
      <Heading level={2} id={headingId}>
        {t("client:voice.call.title")}
      </Heading>
      {controls !== null && (
        <>
          <Hint>{callPhaseText(t, controls)}</Hint>
          {controls.playbackBlocked && (
            <Callout>
              <p>{t("client:voice.call.playback_blocked")}</p>
              <PlaybackButton controls={controls} />
            </Callout>
          )}
          <MicrophoneControl controls={controls} withDescription />
        </>
      )}
      <Cluster gap="2" align="center">
        {call.isSpeaker ? (
          <Button
            onPress={() => {
              call.operate("listen");
            }}
            isPending={call.pendingOperation === "listen"}
          >
            {t("client:voice.call.listen")}
          </Button>
        ) : (
          <>
            <Button
              onPress={() => {
                call.operate("speak");
              }}
              isPending={call.pendingOperation === "speak"}
            >
              {t("client:voice.call.speak")}
            </Button>
            {call.self?.hand_raised === true ? (
              <Button
                variant="ghost"
                onPress={() => {
                  call.operate("lower-hand");
                }}
                isPending={call.pendingOperation === "lower-hand"}
              >
                {t("client:voice.call.lower_hand")}
              </Button>
            ) : (
              <Button
                variant="ghost"
                onPress={() => {
                  call.operate("raise-hand");
                }}
                isPending={call.pendingOperation === "raise-hand"}
              >
                {t("client:voice.call.raise_hand")}
              </Button>
            )}
          </>
        )}
        <ReactionPickerButton onSelect={call.react} />
        {call.manages && <InviteButton room={room} />}
        {controls !== null && <LeaveCallButton controls={controls} />}
      </Cluster>
      {call.actionError !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:voice.call.action_failed", { reason: call.actionError })}
        </InlineError>
      )}
    </Stack>
  );
}

/** Invites a local user as a listener or a speaker (ADR-0100 §7). */
function InviteButton({ room }: { room: VoiceRoom }) {
  const t = useT();
  const invite = useVoiceInvite(room);
  const [isOpen, setIsOpen] = useState(false);
  return (
    <>
      <Button
        variant="ghost"
        onPress={() => {
          invite.reset();
          setIsOpen(true);
        }}
      >
        {t("client:voice.invite.button")}
      </Button>
      <ConfirmDialog
        isOpen={isOpen}
        onOpenChange={setIsOpen}
        title={t("client:voice.invite.title")}
        description={t("client:voice.invite.description")}
        confirmLabel={t("client:voice.invite.submit")}
        cancelLabel={t("client:voice.call.cancel")}
        tone="primary"
        isPending={invite.isPending}
        errorMessage={invite.error === undefined ? undefined : t("client:voice.invite.failed", { reason: invite.error })}
        onConfirm={(close) => {
          invite.submit(close);
        }}
      >
        <TextField
          label={t("client:voice.invite.username")}
          value={invite.username}
          onChange={invite.setUsername}
          isRequired
          autoComplete="off"
          autoCorrect="off"
          spellCheck="false"
        />
        <RadioGroup
          label={t("client:voice.invite.role")}
          value={invite.role}
          onChange={(value) => {
            invite.setRole(value === "speaker" ? "speaker" : "listener");
          }}
        >
          {(["listener", "speaker"] as const satisfies readonly VoiceRole[]).map((role) => (
            <Radio key={role} value={role} description={role === "speaker" ? t("client:voice.invite.role.speaker.description") : undefined}>
              {role === "speaker" ? t("client:voice.invite.role.speaker") : t("client:voice.invite.role.listener")}
            </Radio>
          ))}
        </RadioGroup>
      </ConfirmDialog>
    </>
  );
}

/** Everyone in the room, in the order they joined, each with their role (回答 31) and, in the call, what they do. */
function Participants({ room, call }: { room: VoiceRoom; call: InCall | undefined }) {
  const t = useT();
  const headingId = useId();
  return (
    <Stack as="section" gap="3" aria-labelledby={headingId}>
      <Heading level={2} id={headingId}>
        {t("client:voice.room.participants")}
      </Heading>
      <Hint>{t("client:voice.room.participants_count", { count: room.participants.length })}</Hint>
      {call === undefined ? <ParticipantList room={room} call={undefined} moderation={undefined} /> : <ModeratedParticipants room={room} call={call} />}
    </Stack>
  );
}

function ModeratedParticipants({ room, call }: { room: VoiceRoom; call: InCall }) {
  const moderation = useVoiceModeration(room);
  const t = useT();
  return (
    <>
      {moderation.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:voice.call.action_failed", { reason: moderation.error })}
        </InlineError>
      )}
      <ParticipantList room={room} call={call} moderation={moderation} />
    </>
  );
}

function ParticipantList({ room, call, moderation }: { room: VoiceRoom; call: InCall | undefined; moderation: VoiceModeration | undefined }) {
  return (
    <Stack as="ul" gap="3">
      {room.participants.map((participant) => (
        <li key={participant.account.id}>
          <ParticipantRow participant={participant} call={call} moderation={moderation} />
        </li>
      ))}
    </Stack>
  );
}

function ParticipantRow({ participant, call, moderation }: { participant: VoiceRoomParticipant; call: InCall | undefined; moderation: VoiceModeration | undefined }) {
  const t = useT();
  const id = participant.account.id;
  const isSelf = call?.self?.account.id === id;
  const micOn = call?.mics.get(id) === true && participant.role === "speaker";
  const reactions = call?.reactions.get(id) ?? [];
  const operations = call === undefined ? [] : participantOperations(call.self, participant);
  const name = participantName(participant);
  const [removing, setRemoving] = useState(false);
  const markers = (
    <>
      {isSelf && <Tag>{t("client:voice.participant.you")}</Tag>}
      {participant.hand_raised && <Tag>{t("client:voice.participant.hand_raised")}</Tag>}
      {micOn && <Tag>{t("client:voice.participant.mic_on")}</Tag>}
      {participant.blocked && <Tag>{t("client:voice.participant.blocked")}</Tag>}
      {reactions.length > 0 && <Reactions reactions={reactions} />}
    </>
  );
  return (
    <>
      <AccountItem
        name={name}
        handle={handleOf(participant.account.acct)}
        avatar={imageSources(participant.account.avatar)}
        avatarSeed={id}
        href={profilePath(participant.account.acct)}
        trailingPlacement="below"
        trailing={
          <Cluster gap="1" align="center">
            {markers}
            <Tag>{t(roleLabel(participant))}</Tag>
            {operations.length > 0 && moderation !== undefined && (
              <MenuTrigger>
                <Button variant="subtle" size="toolbar" aria-label={t("client:voice.moderate.menu", { name })} isPending={moderation.pending?.accountId === id}>
                  <MoreIcon />
                </Button>
                <Menu aria-label={t("client:voice.moderate.menu", { name })} placement="bottom end">
                  {operations.map((operation) => (
                    <MenuItem
                      key={operation}
                      onAction={() => {
                        if (operation === "remove") setRemoving(true);
                        else moderation.operate(participant, operation);
                      }}
                    >
                      {t(operationLabels[operation])}
                    </MenuItem>
                  ))}
                </Menu>
              </MenuTrigger>
            )}
          </Cluster>
        }
      />
      {moderation !== undefined && (
        <ConfirmDialog
          isOpen={removing}
          onOpenChange={setRemoving}
          title={t("client:voice.moderate.remove.title", { name })}
          description={t("client:voice.moderate.remove.description")}
          confirmLabel={t("client:voice.moderate.remove")}
          cancelLabel={t("client:voice.call.cancel")}
          tone="danger"
          onConfirm={(close) => {
            moderation.operate(participant, "remove");
            close();
          }}
        />
      )}
    </>
  );
}

/** The reactions a participant just sent, which fade after a few seconds. */
function Reactions({ reactions }: { reactions: readonly VoiceReactionShown[] }) {
  const t = useT();
  const images = useCustomEmojiImages();
  return (
    <Cluster as="span" gap="1" align="center">
      <VisuallyHidden>{t("client:voice.participant.reacted")}</VisuallyHidden>
      {reactions.map((reaction) => {
        const shortcode = /^:([^\s:]+):$/u.exec(reaction.emoji)?.[1];
        const image = shortcode === undefined ? undefined : images.get(shortcode);
        return image === undefined ? (
          <Text key={reaction.id} as="span" size="base" dir="auto">
            {reaction.emoji}
          </Text>
        ) : (
          <CustomEmoji key={reaction.id} emoji={image} />
        );
      })}
    </Cluster>
  );
}
