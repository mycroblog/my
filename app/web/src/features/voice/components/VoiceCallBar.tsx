// SPDX-License-Identifier: AGPL-3.0-only
// The mini-player (#1345 answer 45, web-client/09-app-shell.md §9): while the user is in a call, every page but the
// room's own ends with a small floating card docked at the bottom of the page's column, clear of the tab bar and the
// floating new-post button: the stacked avatars of the speakers (who is talking first, ringed), the room's name as the
// link back to its page over who is talking (or the call's state while it is not connected), the microphone of a
// speaker, the button that plays the sound the browser blocked, and leaving (with the host's choice), with what
// failed under them. It is a region named "Voice chat call", after the page's content in the focus order.
import { Link, MiniPlayer, type MiniPlayerAvatar, ShellDock } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { imageSources } from "../../../lib/images";
import { voiceRoomPath } from "../api/voice";
import type { VoiceCallControls } from "../hooks/useVoiceCallControls";
import { useVoiceCallBar } from "../hooks/useVoiceCallControls";
import { participantName } from "../hooks/voiceCall";
import {
  CallErrors,
  callStatusText,
  hasCallErrors,
  LeaveCallButton,
  MicrophoneControl,
  PlaybackButton,
} from "./VoiceCallParts";

export type VoiceCallBarProps = {
  /** Whether the page has the bottom tab bar below md (the app's frame), not the admin frame. */
  clearsTabBar?: boolean;
};

/** The speakers to stack: those talking first (so that the ringed one shows), each in the room's order. */
function speakerAvatars(controls: VoiceCallControls): MiniPlayerAvatar[] {
  const talking = new Set(controls.talking);
  const speakers = controls.room.participants.filter((p) => p.role === "speaker");
  const ordered = [...speakers.filter((p) => talking.has(p)), ...speakers.filter((p) => !talking.has(p))];
  return ordered.map((p) => ({
    id: p.account.id,
    name: participantName(p),
    sources: imageSources(p.account.avatar),
    speaking: controls.phase === "live" && talking.has(p),
  }));
}

export function VoiceCallBar({ clearsTabBar = true }: VoiceCallBarProps) {
  const t = useT();
  const controls = useVoiceCallBar();
  if (controls === null) return null;
  return (
    <ShellDock clearsTabBar={clearsTabBar}>
      <MiniPlayer
        label={t("client:voice.bar.label")}
        avatars={speakerAvatars(controls)}
        title={
          <Link href={voiceRoomPath(controls.room.id)} variant="standalone">
            <span dir="auto">{controls.room.name}</span>
          </Link>
        }
        status={<span dir="auto">{callStatusText(t, controls)}</span>}
        notice={hasCallErrors(controls) ? <CallErrors controls={controls} /> : undefined}
      >
        <PlaybackButton controls={controls} size="sm" />
        <MicrophoneControl controls={controls} />
        <LeaveCallButton controls={controls} look="mini" />
      </MiniPlayer>
    </ShellDock>
  );
}
