// SPDX-License-Identifier: AGPL-3.0-only
// The call bar (#1345 answer 45, web-client/09-app-shell.md §9): while the user is in a call, every page but the
// room's own ends with a small bar docked at the bottom of the page's column, clear of the tab bar and the floating
// new-post button: the room's name as the link back to its page, the call's state, the microphone of a speaker, the
// button that plays the sound the browser blocked, and leaving (with the host's choice). It is a region named
// "Voice chat call", after the page's content in the focus order.
import { CallBar, Link, ShellDock } from "@my/ui";
import { useLocation } from "@tanstack/react-router";
import { useT } from "../../../lib/i18n";
import { voiceRoomPath } from "../api/voice";
import { useVoiceCallControls } from "../hooks/useVoiceCallControls";
import { callPhaseText, LeaveCallButton, MicrophoneControl, PlaybackButton } from "./VoiceCallParts";

export type VoiceCallBarProps = {
  /** Whether the page has the bottom tab bar below md (the app's frame), not the admin frame. */
  clearsTabBar?: boolean;
};

export function VoiceCallBar({ clearsTabBar = true }: VoiceCallBarProps) {
  const t = useT();
  const controls = useVoiceCallControls();
  const pathname = useLocation({ select: (location) => location.pathname });
  // The room's page has the call's controls itself.
  if (controls === null || pathname === voiceRoomPath(controls.room.id)) return null;
  return (
    <ShellDock clearsTabBar={clearsTabBar}>
      <CallBar
        label={t("client:voice.bar.label")}
        title={
          <Link href={voiceRoomPath(controls.room.id)} variant="standalone">
            <span dir="auto">{controls.room.name}</span>
          </Link>
        }
        status={callPhaseText(t, controls)}
      >
        <PlaybackButton controls={controls} />
        <MicrophoneControl controls={controls} withDescription={false} />
        <LeaveCallButton controls={controls} />
      </CallBar>
    </ShellDock>
  );
}
