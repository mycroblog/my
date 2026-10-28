// SPDX-License-Identifier: AGPL-3.0-only
// The mini-player (#1345 answer 45, web-client/09-app-shell.md §9): while the user is in a call, every page but the
// room's own ends with a small floating card docked at the bottom of the page's column, clear of the tab bar and the
// floating new-post button: the stacked avatars of the speakers (who is talking first, ringed), the room's name as the
// link back to its page over who is talking (or, while nobody talks or the call is not connected, the badge of the
// call's connection), the microphone of a speaker, the button that plays the sound the browser blocked, and last,
// apart, leaving (the host's choice of the next host, or anyone else's confirmation), with what failed under them. It is a region named "Voice chat call", after the page's content in the focus order.
import { ChangeNotice, Link, MiniPlayer, type MiniPlayerAvatar, ShellDock, Stack } from "@my/ui";
import { useState } from "react";
import { useT } from "../../../lib/i18n";
import { imageSources } from "../../../lib/images";
import { voiceRoomPath } from "../api/voice";
import type { VoiceCallControls } from "../hooks/useVoiceCallControls";
import { useVoiceCallBar, useVoiceCallEndNotice } from "../hooks/useVoiceCallControls";
import { participantName, type VoiceCall } from "../hooks/voiceCall";
import {
  CallConnection,
  CallErrors,
  CallNotice,
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
  const ended = useVoiceCallEndNotice();
  // The call whose end the user dismissed: its notice does not come back.
  const [dismissed, setDismissed] = useState<VoiceCall | null>(null);
  if (controls === null) {
    if (ended === null || ended.call === dismissed) return null;
    // The call ended without the user leaving it: a card stays in the mini-player's place to say how, with the room's
    // name as the way back to it, until it is dismissed.
    return (
      <ShellDock clearsTabBar={clearsTabBar}>
        <MiniPlayer
          label={t("client:voice.bar.label")}
          avatars={[]}
          title={
            <Link href={voiceRoomPath(ended.room.id)} variant="standalone">
              <span dir="auto">{ended.room.name}</span>
            </Link>
          }
          notice={
            <ChangeNotice
              tone="warning"
              dismissLabel={t("client:shell.notice.dismiss")}
              onDismiss={() => {
                setDismissed(ended.call);
              }}
            >
              {ended.text}
            </ChangeNotice>
          }
        />
      </ShellDock>
    );
  }
  const errors = hasCallErrors(controls);
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
        status={
          controls.phase === "live" && controls.talking.length > 0 ? (
            <span dir="auto">{callStatusText(t, controls)}</span>
          ) : (
            <CallConnection controls={controls} />
          )
        }
        end={<LeaveCallButton controls={controls} look="mini" />}
        notice={
          controls.notice !== undefined || errors ? (
            <Stack gap="2">
              <CallNotice controls={controls} />
              {errors && <CallErrors controls={controls} />}
            </Stack>
          ) : undefined
        }
      >
        <PlaybackButton controls={controls} size="sm" />
        <MicrophoneControl controls={controls} />
      </MiniPlayer>
    </ShellDock>
  );
}
