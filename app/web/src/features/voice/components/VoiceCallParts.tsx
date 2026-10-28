// SPDX-License-Identifier: AGPL-3.0-only
// The parts of a call that the room's page and the mini-player share (ADR-0100 §5, §6; ADR-0101 §7): the call's
// state in a few words, who is talking, the mini-player's microphone toggle, what failed (why the browser refused the
// microphone, leaving), the button that plays the sound the browser blocked, and leaving, which asks the host of a
// room with others in it for the next host or the end of the room.
import {
  Button,
  CallDockButton,
  ConfirmDialog,
  InlineError,
  LeaveIcon,
  MicrophoneIcon,
  Radio,
  RadioGroup,
} from "@my/ui";
import { useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import type { VoiceCallControls } from "../hooks/useVoiceCallControls";
import { participantName, type VoiceMicError } from "../hooks/voiceCall";

const phaseLabels: Readonly<Record<VoiceCallControls["phase"], ClientMessageKey>> = {
  connecting: "client:voice.call.connecting",
  live: "client:voice.call.live",
  reconnecting: "client:voice.call.reconnecting",
  ended: "client:voice.call.connecting",
};

const micErrors: Readonly<Record<VoiceMicError, ClientMessageKey>> = {
  denied: "client:voice.call.microphone_denied",
  missing: "client:voice.call.microphone_missing",
  busy: "client:voice.call.microphone_busy",
  other: "client:voice.call.microphone_failed",
};

/** The call's state in a few words ("Connected"). */
export function callPhaseText(t: ReturnType<typeof useT>, controls: VoiceCallControls): string {
  return t(phaseLabels[controls.phase]);
}

/**
 * Who is talking ("Haruka is talking", "Haruka and 2 others are talking") while the call is connected and someone
 * talks; otherwise the call's state ("Connected", "Reconnecting…").
 */
export function callStatusText(t: ReturnType<typeof useT>, controls: VoiceCallControls): string {
  const [first] = controls.talking;
  if (controls.phase !== "live" || first === undefined) return callPhaseText(t, controls);
  return t("client:voice.bar.talking", { name: participantName(first), others: controls.talking.length - 1 });
}

/** The microphone toggle of a speaker in the mini-player; CallErrors says why it could not turn on. */
export function MicrophoneControl({ controls }: { controls: VoiceCallControls }) {
  const t = useT();
  if (!controls.isSpeaker) return null;
  return (
    <CallDockButton
      label={t("client:voice.call.microphone")}
      size="sm"
      icon={<MicrophoneIcon filled={controls.micOn} />}
      isSelected={controls.micOn}
      onChange={controls.setMicrophone}
      isDisabled={controls.micPending}
    />
  );
}

/** Why the microphone could not turn on: the browser's refusal, or the server's. */
function MicrophoneErrors({ controls }: { controls: VoiceCallControls }) {
  const t = useT();
  return (
    <>
      {controls.micError !== undefined && (
        <InlineError size="inherit" alert>
          {t(micErrors[controls.micError])}
        </InlineError>
      )}
      {controls.micRefused !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:voice.call.action_failed", { reason: controls.micRefused })}
        </InlineError>
      )}
    </>
  );
}

/** Whether something failed in the call: the microphone (of a speaker), leaving, or `actionError`. */
export function hasCallErrors(controls: VoiceCallControls, actionError?: string): boolean {
  return (
    (controls.isSpeaker && (controls.micError !== undefined || controls.micRefused !== undefined)) ||
    controls.leaveError !== undefined ||
    actionError !== undefined
  );
}

/**
 * What failed in the call (the microphone, leaving, and on the room's page the user's last operation, `actionError`),
 * for the notice of the mini-player or the dock; null for nothing.
 */
export function CallErrors({
  controls,
  actionError,
}: {
  controls: VoiceCallControls;
  actionError?: string | undefined;
}) {
  const t = useT();
  if (!hasCallErrors(controls, actionError)) return null;
  return (
    <>
      {controls.isSpeaker && <MicrophoneErrors controls={controls} />}
      {controls.leaveError !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:voice.call.leave_failed", { reason: controls.leaveError })}
        </InlineError>
      )}
      {actionError !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:voice.call.action_failed", { reason: actionError })}
        </InlineError>
      )}
    </>
  );
}

/** The button that plays the sound the browser blocked; nothing while it plays. `sm` in the mini-player. */
export function PlaybackButton({ controls, size = "md" }: { controls: VoiceCallControls; size?: "sm" | "md" }) {
  const t = useT();
  if (!controls.playbackBlocked) return null;
  return (
    <Button variant="primary" size={size} onPress={controls.resumePlayback}>
      {t("client:voice.call.play_audio")}
    </Button>
  );
}

/**
 * The leave button and its dialog: the host of a room with others in it hands it over or ends it (ADR-0100 §6). It is
 * the last control of the room page's dock (`look="dock"`) or of the mini-player (`look="mini"`); CallErrors says why
 * leaving failed.
 */
export function LeaveCallButton({ controls, look }: { controls: VoiceCallControls; look: "dock" | "mini" }) {
  const t = useT();
  const [asking, setAsking] = useState(false);
  const onPress = () => {
    if (controls.mustChoose) setAsking(true);
    else controls.leave(undefined);
  };
  const isPending = controls.isLeaving && !asking;
  return (
    <>
      {look === "dock" ? (
        <CallDockButton
          label={t("client:voice.call.leave")}
          icon={<LeaveIcon />}
          showLabel
          variant="danger"
          onPress={onPress}
          isPending={isPending}
        />
      ) : (
        <CallDockButton
          label={t("client:voice.call.leave")}
          icon={<LeaveIcon />}
          size="sm"
          variant="danger-soft"
          onPress={onPress}
          isPending={isPending}
        />
      )}
      {controls.mustChoose && (
        <HostLeaveDialog
          controls={controls}
          isOpen={asking}
          onClose={() => {
            setAsking(false);
            controls.resetLeave();
          }}
        />
      )}
    </>
  );
}

function HostLeaveDialog({
  controls,
  isOpen,
  onClose,
}: {
  controls: VoiceCallControls;
  isOpen: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const [choice, setChoice] = useState("");
  return (
    <ConfirmDialog
      isOpen={isOpen}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t("client:voice.call.host_leave.title")}
      description={t("client:voice.call.host_leave.description")}
      confirmLabel={t("client:voice.call.leave")}
      cancelLabel={t("client:voice.call.stay")}
      tone="danger"
      isPending={controls.isLeaving}
      errorMessage={controls.leaveError}
      onConfirm={() => {
        controls.leave(choice === "end" ? { end: true } : { nextHostId: choice });
      }}
    >
      <RadioGroup label={t("client:voice.call.host_leave.choice")} value={choice} onChange={setChoice} isRequired>
        {controls.nextHosts.map((participant) => (
          <Radio key={participant.account.id} value={participant.account.id}>
            <span dir="auto">
              {t("client:voice.call.host_leave.next_host", { name: participantName(participant) })}
            </span>
          </Radio>
        ))}
        <Radio value="end">{t("client:voice.call.host_leave.end")}</Radio>
      </RadioGroup>
    </ConfirmDialog>
  );
}
