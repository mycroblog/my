// SPDX-License-Identifier: AGPL-3.0-only
// The parts of a call that the room's page and the call bar share (ADR-0100 §5, §6; ADR-0101 §7): the call's state
// in a few words, the microphone switch with why the browser refused it, the button that plays the sound the browser
// blocked, and leaving, which asks the host of a room with others in it for the next host or the end of the room.
import { Button, ConfirmDialog, InlineError, Radio, RadioGroup, Switch } from "@my/ui";
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

/** The microphone switch of a speaker; with `withErrors`, why it could not turn on (otherwise CallErrors says it). */
export function MicrophoneControl({ controls, withDescription, withErrors }: { controls: VoiceCallControls; withDescription: boolean; withErrors: boolean }) {
  const t = useT();
  if (!controls.isSpeaker) return null;
  return (
    <>
      <Switch
        isSelected={controls.micOn}
        onChange={controls.setMicrophone}
        isDisabled={controls.micPending}
        {...(withDescription && { description: t("client:voice.call.microphone.description") })}
      >
        {t("client:voice.call.microphone")}
      </Switch>
      {withErrors && <MicrophoneErrors controls={controls} />}
    </>
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

/** What failed in the call (the microphone, leaving), for the notice of the call bar; null for nothing. */
export function CallErrors({ controls }: { controls: VoiceCallControls }) {
  const t = useT();
  if (controls.micError === undefined && controls.micRefused === undefined && controls.leaveError === undefined) return null;
  return (
    <>
      {controls.isSpeaker && <MicrophoneErrors controls={controls} />}
      {controls.leaveError !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:voice.call.leave_failed", { reason: controls.leaveError })}
        </InlineError>
      )}
    </>
  );
}

/** The button that plays the sound the browser blocked; nothing while it plays. */
export function PlaybackButton({ controls }: { controls: VoiceCallControls }) {
  const t = useT();
  if (!controls.playbackBlocked) return null;
  return (
    <Button variant="primary" onPress={controls.resumePlayback}>
      {t("client:voice.call.play_audio")}
    </Button>
  );
}

/** The leave button and its dialog: the host of a room with others in it hands it over or ends it (ADR-0100 §6). */
export function LeaveCallButton({ controls, withError = true }: { controls: VoiceCallControls; withError?: boolean }) {
  const t = useT();
  const [asking, setAsking] = useState(false);
  return (
    <>
      <Button
        variant="danger"
        onPress={() => {
          if (controls.mustChoose) setAsking(true);
          else controls.leave(undefined);
        }}
        isPending={controls.isLeaving && !asking}
      >
        {t("client:voice.call.leave")}
      </Button>
      {withError && controls.leaveError !== undefined && !asking && (
        <InlineError size="inherit" alert>
          {t("client:voice.call.leave_failed", { reason: controls.leaveError })}
        </InlineError>
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

function HostLeaveDialog({ controls, isOpen, onClose }: { controls: VoiceCallControls; isOpen: boolean; onClose: () => void }) {
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
            <span dir="auto">{t("client:voice.call.host_leave.next_host", { name: participantName(participant) })}</span>
          </Radio>
        ))}
        <Radio value="end">{t("client:voice.call.host_leave.end")}</Radio>
      </RadioGroup>
    </ConfirmDialog>
  );
}
