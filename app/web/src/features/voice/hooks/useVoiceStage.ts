// SPDX-License-Identifier: AGPL-3.0-only
// What the stage of a room's page shows (ADR-0100 §9): the speakers and the listeners, the host and the co-hosts
// first, each with the role under the name, whether they talk now (the call's `talking`), a speaker's microphone that
// is off, a raised hand, the reaction they just sent, and what the user may do to them; the raised hands that the
// host and the co-hosts answer; how long the room has been open; and sharing the room's link.
import { useToast } from "@my/ui";
import { useNow } from "../../../lib/clock";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import type { VoiceParticipantOperation } from "../api/call";
import { type VoiceRoom, type VoiceRoomParticipant, voiceRoomPath } from "../api/voice";
import type { VoiceCallControl } from "./useVoiceCall";
import { participantOperations } from "./useVoiceModeration";
import { participantName, type VoiceReactionShown } from "./voiceCall";

type InCall = Extract<VoiceCallControl, { status: "in-call" }>;

/** One participant on the stage. */
export type VoiceStageEntry = {
  participant: VoiceRoomParticipant;
  name: string;
  /** Whether the participant is the user in the call. */
  isSelf: boolean;
  /** The line under the name: the role, and whether it is the user or someone with a block between them. */
  caption: string | undefined;
  speaking: boolean;
  /** A speaker whose microphone is off; unknown (false) outside the call. */
  muted: boolean;
  handRaised: boolean;
  /** The reaction showing now, the last one sent. */
  reaction: VoiceReactionShown | undefined;
  /** What the user may do to the participant, in the order of the menu (only the host and the co-hosts have any). */
  operations: readonly VoiceParticipantOperation[];
};

export type VoiceStage = {
  speakers: readonly VoiceStageEntry[];
  listeners: readonly VoiceStageEntry[];
};

/** The label of a participant's place in the room: the host, a co-host, or a speaker or a listener. */
function roleLabel(participant: VoiceRoomParticipant): ClientMessageKey {
  if (participant.host) return "client:voice.role.host";
  if (participant.cohost) return "client:voice.role.cohost";
  return participant.role === "speaker" ? "client:voice.role.speaker" : "client:voice.role.listener";
}

/** The host first, then the co-hosts, then everyone else, each in the order they joined. */
function rank(participant: VoiceRoomParticipant): number {
  if (participant.host) return 0;
  return participant.cohost ? 1 : 2;
}

/** The raised hands of the room, in the order they joined. */
export function raisedHands(room: VoiceRoom): readonly VoiceRoomParticipant[] {
  return room.participants.filter((participant) => participant.hand_raised);
}

/**
 * The stage of `room`. In the call (`call`), with who talks, the microphones (`micOn`: the user's own, which the
 * call knows before the server echoes it), and the reactions.
 */
export function useVoiceStage(room: VoiceRoom, call: InCall | undefined, micOn: boolean): VoiceStage {
  const t = useT();
  const selfId = call?.self?.account.id;
  const entry = (participant: VoiceRoomParticipant): VoiceStageEntry => {
    const id = participant.account.id;
    const isSelf = id === selfId;
    const speaker = participant.role === "speaker";
    // A listener's caption names only the host or a co-host: the heading of the grid says the rest.
    const parts = [
      ...(speaker || participant.host || participant.cohost ? [t(roleLabel(participant))] : []),
      ...(isSelf ? [t("client:voice.participant.you")] : []),
      ...(participant.blocked ? [t("client:voice.participant.blocked")] : []),
    ];
    const reactions = call?.reactions.get(id) ?? [];
    const on = isSelf ? micOn : call?.mics.get(id) === true;
    return {
      participant,
      name: participantName(participant),
      isSelf,
      caption: parts.length === 0 ? undefined : parts.join(t("client:voice.participant.caption_separator")),
      speaking: call !== undefined && participant.session !== undefined && call.talking.has(participant.session),
      muted: call !== undefined && speaker && !on,
      handRaised: participant.hand_raised,
      reaction: reactions.at(-1),
      operations: call === undefined ? [] : participantOperations(call.self, participant),
    };
  };
  const ordered = [...room.participants].sort((a, b) => rank(a) - rank(b));
  return {
    speakers: ordered.filter((participant) => participant.role === "speaker").map(entry),
    listeners: ordered.filter((participant) => participant.role !== "speaker").map(entry),
  };
}

const minute = 60_000;

/** How long the room has been open ("38 min", "1 hr 5 min"); it renders again as the clock ticks. */
export function useOpenFor(openedAt: string): string {
  const t = useT();
  const now = useNow();
  const minutes = Math.max(0, Math.floor((now - Date.parse(openedAt)) / minute));
  if (minutes < 60) return t("client:voice.room.open_minutes", { minutes });
  return t("client:voice.room.open_hours", { hours: Math.floor(minutes / 60), minutes: minutes % 60 });
}

/**
 * Shares the room's link: the system's share sheet where the browser has one (the Web Share API), otherwise a copy
 * to the clipboard, which a toast confirms.
 */
export function useShareRoom(room: VoiceRoom): () => void {
  const t = useT();
  const toast = useToast();
  return () => {
    const url = new URL(voiceRoomPath(room.id), window.location.origin).toString();
    const copy = () => {
      navigator.clipboard.writeText(url).then(
        () => {
          // The toast is announced too.
          toast.show({ message: t("client:voice.room.share.copied"), tone: "success" });
        },
        () => {
          toast.show({ message: t("client:voice.room.share.failed"), tone: "danger" });
        },
      );
    };
    if (!("share" in navigator)) {
      copy();
      return;
    }
    navigator.share({ title: room.name, url }).catch((error: unknown) => {
      // The user closed the share sheet: nothing to do.
      if (error instanceof DOMException && error.name === "AbortError") return;
      copy();
    });
  };
}
