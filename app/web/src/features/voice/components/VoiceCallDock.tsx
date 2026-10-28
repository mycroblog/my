// SPDX-License-Identifier: AGPL-3.0-only
// The dock of a room's page (ADR-0100 §5–§7, §10; ADR-0101 §7), at the bottom of the page's column (ShellDock).
// Before joining: the notice that joining lists the user's name with the join button (回答 31); for a user in the
// room from another device, the button that joins here; for a user in another room's call, that joining leaves it,
// with "Move here". In the call, a toolbar (CallDock): a speaker's microphone (CallMicrophoneButton: coral and filled
// while on, slashed while off, named by what a press does) and "Just listen" (also in the menu of the user's own
// avatar), or a listener's "Speak" and raised hand; reactions (the picker of
// ADR-0091); for the host and the co-hosts, inviting and the raised hands ("Hands", with their count, opening their
// list); and leaving at the end, where the host of a room with others in it chooses the next host or ends the room.
// Under it: the sound that the browser blocked, with the button that plays it, and what failed.
import {
  Button,
  CallDock,
  CallDockButton,
  CallMicrophoneButton,
  Card,
  Cluster,
  ConfirmDialog,
  InlineError,
  InviteIcon,
  ListenIcon,
  MicrophoneIcon,
  Popover,
  PopoverTrigger,
  Radio,
  RadioGroup,
  RaiseHandIcon,
  Stack,
  Text,
  TextField,
} from "@my/ui";
import { useState } from "react";
import { useT } from "../../../lib/i18n";
import { ReactionPickerButton } from "../../emoji";
import type { VoiceRole } from "../api/call";
import type { VoiceRoom, VoiceRoomParticipant } from "../api/voice";
import type { VoiceCallControl } from "../hooks/useVoiceCall";
import type { VoiceCallControls } from "../hooks/useVoiceCallControls";
import { callEndText } from "../hooks/useVoiceCallProvider";
import { useVoiceInvite, type VoiceModeration } from "../hooks/useVoiceModeration";
import {
  CallErrors,
  CallNotice,
  hasCallErrors,
  LeaveCallButton,
  microphoneLabels,
  PlaybackButton,
} from "./VoiceCallParts";
import { RaisedHandList } from "./VoiceRoomAside";

type Outside = Extract<VoiceCallControl, { status: "outside" }>;
type InCall = Extract<VoiceCallControl, { status: "in-call" }>;

/** The name of the join button: "Join here" from another device, "Move here" from another room's call, or "Join". */
function joinLabel(t: ReturnType<typeof useT>, call: Outside): string {
  if (call.inRoomElsewhere || call.end?.kind === "replaced") return t("client:voice.call.join_here");
  if (call.otherRoom !== undefined) return t("client:voice.room.move");
  return t("client:voice.room.join");
}

/** The join notice and the join button, in the dock's place before joining. */
export function JoinDock({ call }: { call: Outside }) {
  const t = useT();
  return (
    <Card padding="4">
      <Stack gap="3">
        {call.end !== undefined && <Text as="p">{callEndText(t, call.end)}</Text>}
        {!call.supported ? (
          <Text as="p">{t("client:voice.call.unsupported_browser")}</Text>
        ) : (
          <Cluster gap="3" align="center" justify="between">
            {call.inRoomElsewhere ? (
              <Text as="p">{t("client:voice.room.joined")}</Text>
            ) : call.otherRoom !== undefined ? (
              // Joining here ends the call of another room: say so before, and name the button by it.
              <Stack gap="1">
                <Text as="p" wrap="anywhere">
                  {t("client:voice.room.move_notice", { room: `⁨${call.otherRoom.name}⁩` })}
                </Text>
                <Text as="p">{t("client:voice.room.listed_notice")}</Text>
              </Stack>
            ) : (
              <Text as="p">{t("client:voice.room.listed_notice")}</Text>
            )}
            {call.end?.kind !== "removed" && (
              <Button variant="primary" onPress={call.join} isPending={call.isJoining}>
                {joinLabel(t, call)}
              </Button>
            )}
          </Cluster>
        )}
        {call.joinError !== undefined && (
          <InlineError size="inherit" alert>
            {t("client:voice.room.join_failed", { reason: call.joinError })}
          </InlineError>
        )}
      </Stack>
    </Card>
  );
}

export type CallDockControlsProps = {
  room: VoiceRoom;
  call: InCall;
  controls: VoiceCallControls;
  moderation: VoiceModeration;
  /** The raised hands, for the host and the co-hosts. */
  hands: readonly VoiceRoomParticipant[];
};

/** The controls of the call. */
export function CallDockControls({ room, call, controls, moderation, hands }: CallDockControlsProps) {
  const t = useT();
  const raised = call.self?.hand_raised === true;
  const notice =
    controls.notice !== undefined || controls.playbackBlocked || hasCallErrors(controls, call.actionError) ? (
      <Stack gap="2">
        <CallNotice controls={controls} />
        {controls.playbackBlocked && (
          <Cluster gap="3" align="center" justify="between">
            <Text as="p">{t("client:voice.call.playback_blocked")}</Text>
            <PlaybackButton controls={controls} />
          </Cluster>
        )}
        <CallErrors controls={controls} actionError={call.actionError} />
      </Stack>
    ) : undefined;
  return (
    <CallDock
      label={t("client:voice.dock.label")}
      end={<LeaveCallButton controls={controls} look="dock" />}
      notice={notice}
    >
      {call.isSpeaker ? (
        <>
          <CallMicrophoneButton
            isOn={controls.micOn}
            onChange={controls.setMicrophone}
            isPending={controls.micPending}
            labels={microphoneLabels(t)}
            showLabel
          />
          <CallDockButton
            label={t("client:voice.call.listen")}
            icon={<ListenIcon />}
            onPress={() => {
              call.operate("listen");
            }}
            isPending={call.pendingOperation === "listen"}
          />
        </>
      ) : (
        <>
          <CallDockButton
            label={t("client:voice.call.speak")}
            icon={<MicrophoneIcon />}
            showLabel
            onPress={() => {
              call.operate("speak");
            }}
            isPending={call.pendingOperation === "speak"}
          />
          <CallDockButton
            label={t("client:voice.call.raise_hand")}
            icon={<RaiseHandIcon filled={raised} />}
            isSelected={raised}
            onChange={(on) => {
              call.operate(on ? "raise-hand" : "lower-hand");
            }}
            selectedTone="hand"
          />
        </>
      )}
      <ReactionPickerButton onSelect={call.react} appearance="dock" />
      {call.manages && <InviteButton room={room} />}
      {call.manages && <HandsButton self={call.self} moderation={moderation} hands={hands} />}
    </CallDock>
  );
}

/** The raised hands, with their count, which open their list above the dock. */
function HandsButton({
  self,
  moderation,
  hands,
}: {
  self: VoiceRoomParticipant | undefined;
  moderation: VoiceModeration;
  hands: readonly VoiceRoomParticipant[];
}) {
  const t = useT();
  return (
    <PopoverTrigger>
      <CallDockButton
        label={t("client:voice.dock.hands")}
        icon={<RaiseHandIcon />}
        showLabel
        count={
          hands.length === 0
            ? undefined
            : { value: hands.length, label: t("client:voice.hands.count", { count: hands.length }) }
        }
      />
      <Popover title={t("client:voice.hands.heading", { count: hands.length })} placement="top">
        <RaisedHandList hands={hands} self={self} moderation={moderation} />
      </Popover>
    </PopoverTrigger>
  );
}

/** Invites a local user as a listener or a speaker (ADR-0100 §7). */
function InviteButton({ room }: { room: VoiceRoom }) {
  const t = useT();
  const invite = useVoiceInvite(room);
  const [isOpen, setIsOpen] = useState(false);
  return (
    <>
      <CallDockButton
        label={t("client:voice.invite.button")}
        icon={<InviteIcon />}
        onPress={() => {
          invite.reset();
          setIsOpen(true);
        }}
      />
      <ConfirmDialog
        isOpen={isOpen}
        onOpenChange={setIsOpen}
        title={t("client:voice.invite.title")}
        description={t("client:voice.invite.description")}
        confirmLabel={t("client:voice.invite.submit")}
        cancelLabel={t("client:voice.call.cancel")}
        tone="primary"
        isPending={invite.isPending}
        errorMessage={
          invite.error === undefined ? undefined : t("client:voice.invite.failed", { reason: invite.error })
        }
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
            <Radio
              key={role}
              value={role}
              description={role === "speaker" ? t("client:voice.invite.role.speaker.description") : undefined}
            >
              {role === "speaker" ? t("client:voice.invite.role.speaker") : t("client:voice.invite.role.listener")}
            </Radio>
          ))}
        </RadioGroup>
      </ConfirmDialog>
    </>
  );
}
