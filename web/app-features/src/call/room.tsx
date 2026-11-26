// SPDX-License-Identifier: AGPL-3.0-only

// A voice chat room's page, at /voice/{id} (ADR-0100 §5, §9, §10; ADR-0101 §7; FR-924, FR-930), for the listener:
//
// - Before joining: the room's title (its icon, its name, how many are in it of its maximum, marked once it is full),
//   who hosts it and its description, that it is full where it is, who is in it (the speakers on its stage, the
//   listeners under them), and Join, the screen's one action, at its foot under the sentence that joining shows the
//   reader's name in that list (and that it leaves the room the reader is in, if any), and why a join failed.
// - In it, the call of calls.ts: the participants as the room's stream has them, who talks ringed, the microphones off
//   and the raised hands marked, the reactions rising over the stage (a blocked participant's hidden), and the
//   reader's controls at its foot: a speaker's microphone and Stop speaking; a listener's Speak where they may speak
//   without asking, else their hand; a reaction from the emoji picker of ADR-0091; and Leave. Under the stage, the
//   choice of where the sound plays, where the browser lets the page choose it, kept with the call. What the call goes
//   through is said under the host: more people in the room than its maximum, which was lowered; connecting, the connection lost, a host who muted the reader, a microphone the
//   browser refused, the sound the browser holds back (with the press that plays it), a press that did not finish, a
//   leave the server refused to a host. The shell shows no call bar on this page: the room's controls are its own.
// - For its host and co-hosts (host.tsx): a participant's tile opens what they may do with them, the room's menu in
//   its head leads to its settings and invitations and ends it, and the host's Leave asks who hosts next or to end it
//   while others are in the room.
// - A participant the reader blocks, or who blocks the reader, is shown and marked, never hidden.
// - For a reader who is not its host: More in its head (or the last group of a co-host's menu), which reports the room
//   and its host to the moderators (report.tsx), before joining and in it.
// - After the call: why the reader is no longer in it, when it was not their own doing (room-end.ts), over the room as
//   it is now, or over that it ended.
// - A room the reader does not see or that ended: what its summary says (ended, with its name where the server shows
//   it, or the call knew it). A visitor who is not signed in gets the summary alone, that only this server's people
//   join, and the sign-in that leads into the room.
//
// What changes while the page shows is said once, in the one polite live region the page keeps from the start (a new
// element of it each time); the lines shown say it again for the eye, and are no live region of their own. After a
// join the focus goes to the room's controls, and after a leave to Join, or to the way back where the room is gone.

import { nameOf, roomIcon, useHeadcount, voiceInvitationsPath, voiceRoomSettingsPath } from "@my/app-components";
import {
  instanceQuery,
  isVoiceLeaveRefused,
  useCustomEmojis,
  useInstance,
  useViewer,
  useVoiceRoom,
  useVoiceRoomEndedHere,
  useVoiceRoomSummary,
  voiceRoomQuery,
  type AppData,
  type VoiceRoom,
  type VoiceRoomParticipant,
  type VoiceSelfOperation,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  CustomEmoji,
  EmojiPicker,
  EmojiPickerPopover,
  EmojiPickerSheet,
  FeedNotice,
  FormAlert,
  FrameFoot,
  GroupHeading,
  ModalPresence,
  ParticipantGrid,
  ParticipantTile,
  ReactionFlow,
  RoomControl,
  RoomControls,
  RoomTitle,
  Row,
  ScreenHead,
  ScreenStack,
  Select,
  Spoken,
  Stack,
  TextButton,
  TextLink,
  useLocale,
  type PickerEmoji,
  type Reaction,
} from "@my/design-system";
import { audioOutputs, watchMediaDevices, type AudioOutput } from "@my/platform";
import { ProblemError } from "@mycroblog/mykit";
import { useEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import { usePickerData, rememberReaction } from "../emoji/picker";
import { emojiStrings } from "../emoji/strings";
import { stringsIn } from "../locale";
import { ReportDialog } from "../report";
import { CallUnavailableError, useVoiceCall, useVoiceCallState } from "./calls";
import {
  EndRoomDialog,
  HostAskDialog,
  hostsOthers,
  LeaveDialog,
  opKey,
  ParticipantMenu,
  RoomMenu,
  type AskedOperation,
  type HostFailure,
} from "./host";
import { clearLeaveAsk, useLeaveAsk } from "./leave-ask";
import { joinRoom, sawRoomEnd, useRoomEnd, type RoomEnd } from "./room-end";
import type { VoiceCall, VoiceCallState } from "./voice-call";

type T = ReturnType<typeof stringsIn>["t"];

/** Whether the server has the voice chat on, as the instance says: no room is read while it is off. */
export async function voiceOn(data: AppData): Promise<boolean> {
  const instance = await data.queryClient.query({ ...instanceQuery(data), staleTime: "static" }).catch(() => undefined);
  return instance?.voice_enabled !== false;
}

/** What the room's loader waits for: the room, from the cache unless a write marked it stale; nothing while off. */
export async function loadVoiceRoom(data: AppData, id: string): Promise<void> {
  if (!(await voiceOn(data))) return;
  await data.queryClient.query({ ...voiceRoomQuery(data, id), staleTime: Infinity }).catch(() => undefined);
}

/** The name of the room of `id` once it is known, for the screen's heading and the document's title. */
export function useVoiceRoomTitle(id: string, signedIn: boolean): string | undefined {
  const state = useVoiceCallState();
  const off = useVoiceOff();
  const inIt = state !== null && state.roomId === id;
  const { room } = useVoiceRoom(id, signedIn && !inIt && !off);
  const { summary } = useVoiceRoomSummary(id, !signedIn && !off);
  return inIt ? state.room.name : (room?.name ?? summary?.name);
}

/** Whether the voice chat is off on the server: its pages say so and show nothing else. */
export function useVoiceOff(): boolean {
  return useInstance()?.voice_enabled === false;
}

/** A reaction's emoji as it is drawn: a custom emoji of this server by its `:shortcode:`, else the Unicode emoji. */
function useReactionEmoji(): (emoji: string) => ReactNode {
  const custom = useCustomEmojis();
  return (emoji) => {
    const code = /^:([\w+-]+):$/u.exec(emoji)?.[1];
    const found = code === undefined ? undefined : custom.find((e) => e.shortcode === code);
    return found === undefined ? (
      emoji
    ) : (
      <CustomEmoji shortcode={found.shortcode} url={found.url} staticUrl={found.static_url} />
    );
  };
}

/** Moves the focus into `place` (its first button) once `now` turns true: after the reader's join or leave. */
function useFocusInto(place: RefObject<HTMLElement | null>, now: boolean): void {
  useEffect(() => {
    if (now) place.current?.querySelector<HTMLElement>("button:not([disabled]), a[href]")?.focus();
  }, [place, now]);
}

/**
 * What the call knows of the participants beyond the room: who talks, whose microphone is on, who the reader is; the
 * call, which a host's operation goes through; and what says an operation that did not finish.
 */
type Live = {
  readonly state: VoiceCallState;
  readonly call?: VoiceCall | undefined;
  readonly onHostFailed?: ((failure: HostFailure | null) => void) | undefined;
  readonly onHostAsk?: ((op: AskedOperation, target: VoiceRoomParticipant) => void) | undefined;
};

/**
 * The participants of `room`: the speakers on its stage, under the reactions while in the call, then the listeners;
 * in the call, the tile of a participant the reader hosts or co-hosts for opens what they may do with them.
 */
function Participants({ room, live, t }: { room: VoiceRoom; live: Live | undefined; t: T }) {
  const state = live?.state;
  const viewer = useViewer();
  const emojiOf = useReactionEmoji();
  const selfId = state?.self?.account.id ?? viewer?.id;
  const stage = room.participants.filter((p) => p.role === "speaker");
  const listeners = room.participants.filter((p) => p.role === "listener");
  const tile = (p: VoiceRoomParticipant, size: "stage" | "listener") => {
    const own = p.account.id === selfId;
    const talking =
      state !== undefined &&
      ((own && state.microphone && state.talking.has(state.session)) ||
        (p.session !== undefined && state.talking.has(p.session)));
    const micOff =
      state !== undefined && p.role === "speaker" && (own ? !state.microphone : state.mics.get(p.account.id) !== true);
    const shown = {
      name: nameOf(p.account),
      src: p.account.avatar?.small.url,
      size,
      self: own ? t("voice.self") : undefined,
      role: p.hosting ? t("voice.role_host") : p.cohosting ? t("voice.role_cohost") : undefined,
      speaking: talking ? t("voice.speaking") : undefined,
      muted: micOff ? t("voice.mic_off") : undefined,
      hand: p.hand_raised ? t("voice.hand") : undefined,
      blocked: p.blocked ? t("voice.blocked") : undefined,
    };
    return live?.call === undefined || live.onHostFailed === undefined || live.onHostAsk === undefined ? (
      <ParticipantTile key={p.account.id} {...shown} />
    ) : (
      <ParticipantMenu
        key={p.account.id}
        call={live.call}
        room={room}
        self={state?.self}
        target={p}
        tile={shown}
        onAsk={live.onHostAsk}
        onFailed={live.onHostFailed}
        t={t}
      />
    );
  };
  const blocked = new Set(room.participants.filter((p) => p.blocked).map((p) => p.account.id));
  // A participant the reader blocks, or who blocks the reader, sends no reaction the reader sees (FR-930).
  const reactions: Reaction[] = (state?.reactions ?? [])
    .filter((r) => !blocked.has(r.participant))
    .map((r) => ({ id: String(r.id), emoji: emojiOf(r.emoji) }));
  const stageGrid = (
    <ParticipantGrid label={t("voice.speakers")} size="stage">
      {stage.map((p) => tile(p, "stage"))}
    </ParticipantGrid>
  );
  return (
    <>
      <Stack gap="entry-line">
        <GroupHeading>{t("voice.speakers")}</GroupHeading>
        {state === undefined ? (
          stageGrid
        ) : (
          // The call drops a reaction once it has shown its while.
          <ReactionFlow reactions={reactions} onDone={() => undefined}>
            {stageGrid}
          </ReactionFlow>
        )}
      </Stack>
      <Stack gap="entry-line">
        <GroupHeading>{t("voice.listeners")}</GroupHeading>
        {listeners.length === 0 ? (
          <Box as="p" tone="muted">
            {t("voice.no_listeners")}
          </Box>
        ) : (
          <ParticipantGrid label={t("voice.listeners")} size="listener">
            {listeners.map((p) => tile(p, "listener"))}
          </ParticipantGrid>
        )}
      </Stack>
    </>
  );
}

/**
 * The report of the room, for a reader who is not its host: what opens it, for the room's menu (none for its host),
 * and the panel while it is open.
 */
function useRoomReport(room: VoiceRoom): { onReport: (() => void) | undefined; dialog: ReactNode } {
  const viewer = useViewer()?.id;
  const [open, setOpen] = useState(false);
  const offered = viewer !== undefined && viewer !== room.host.id;
  return {
    onReport: offered
      ? () => {
          setOpen(true);
        }
      : undefined,
    dialog: (
      <ModalPresence>
        {open && (
          <ReportDialog
            key="report"
            subject={{ kind: "room", account: room.host, id: room.id, name: room.name }}
            onClose={() => {
              setOpen(false);
            }}
          />
        )}
      </ModalPresence>
    ),
  };
}

/** The back button to the rooms. */
function Back({ onBack, t }: { onBack: () => void; t: T }) {
  return <BackButton onPress={onBack}>{t("voice.back")}</BackButton>;
}

/**
 * The head of a room's page: back to the rooms, then its title with how many are in it of its maximum, and the room's
 * menu at its end.
 */
function Head({ room, onBack, menu, t }: { room: VoiceRoom; onBack: () => void; menu?: ReactNode; t: T }) {
  const count = useHeadcount(room.participants_count, room.max_participants);
  const title = <RoomTitle icon={roomIcon(room.icon)} name={room.name} count={count} />;
  return (
    <ScreenHead back={<Back onBack={onBack} t={t} />}>
      {menu === undefined || menu === null ? (
        title
      ) : (
        <Row align="start" gap="entry-caption">
          <Box grow>{title}</Box>
          <Box rigid>{menu}</Box>
        </Row>
      )}
    </ScreenHead>
  );
}

/**
 * Who hosts the room, while the host is not on its stage (whose tile says Host), and its description as the host
 * wrote it.
 */
function About({ room, t }: { room: VoiceRoom; t: T }) {
  const hostShown = room.participants.some((p) => p.hosting);
  if (hostShown && room.description === "") return null;
  return (
    <Stack gap="hair">
      {!hostShown && (
        <Box as="p" text="body" tone="muted" dir="auto">
          {t("voice.hosted_by", { host: nameOf(room.host) })}
        </Box>
      )}
      {room.description !== "" && (
        <Box as="p" text="body" keeps="lines" dir="auto">
          {room.description}
        </Box>
      )}
    </Stack>
  );
}

/**
 * What a room in the call says while more people are in it than its maximum, after its maximum was lowered; null
 * while it is not. Its host reads how to let new people join, while the maximum in force is the one they set (below
 * the server's), and the way to Room settings after it (`host`); everyone else, and the host under the server's lower
 * maximum, reads that no one was removed.
 */
function useOverMax(state: VoiceCallState | null, t: T): { words: string; host: boolean } | null {
  const serverMax = useInstance()?.voice_max_participants;
  if (state === null || state.room.participants_count <= state.room.max_participants) return null;
  const max = state.room.max_participants;
  const host = state.self?.hosting === true && serverMax !== undefined && max < serverMax;
  return { words: t("voice.over_max", { reader: host ? "host" : "other", max }), host };
}

/** A line of what the call goes through, for the eye: the live region says it to assistive technology. */
function Line({ children, action }: { children: string; action?: ReactNode }) {
  return (
    <Row wrap align="baseline" gapX="entry-caption">
      <Box as="p" dir="auto">
        {children}
      </Box>
      {action}
    </Row>
  );
}

/** The sentence that says why the call in a room ended. */
export function endSentence({ room, end }: RoomEnd, t: T): string {
  const kind = ["removed", "replaced", "ended", "lost"].includes(end.kind) ? end.kind : "other";
  return t("voice.end", { kind, reason: end.reason, room });
}

/** Why a join failed, as the screen says it. */
type JoinFailure = "full" | "hosting" | "unavailable" | "other";

function joinFailureOf(error: unknown): JoinFailure {
  if (error instanceof CallUnavailableError) return "unavailable";
  if (error instanceof ProblemError && error.problem.type === "/api/v1/problems/voice-room-full") return "full";
  if (error instanceof ProblemError && error.problem.type === "/api/v1/problems/voice-hosting-elsewhere")
    return "hosting";
  return "other";
}

const joinWords = {
  full: "voice.join_full",
  hosting: "voice.join_hosting",
  unavailable: "voice.join_unavailable",
  other: "voice.join_failed",
} as const;

/** What a part of the page says once, in the page's live region. */
type Say = (words: string) => void;

/** The room before the reader joins it: who is in it, and Join at its foot. */
function BeforeJoin({
  data,
  room,
  end,
  focus,
  onBack,
  onJoined,
  say,
  t,
}: {
  data: AppData;
  room: VoiceRoom;
  end: RoomEnd | null;
  /** The reader left the room here: the focus goes to Join. */
  focus: boolean;
  onBack: () => void;
  onJoined: () => void;
  say: Say;
  t: T;
}) {
  const other = useVoiceCallState();
  const report = useRoomReport(room);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<JoinFailure | null>(null);
  const foot = useRef<HTMLDivElement>(null);
  useFocusInto(foot, focus);
  const join = () => {
    if (busy) return;
    setBusy(true);
    setFailure(null);
    // Inside the press: the call's sound starts there, or the browser holds it back until another.
    joinRoom(data, room.id)
      .then(onJoined, (error: unknown) => {
        const failed = joinFailureOf(error);
        setFailure(failed);
        say(t(joinWords[failed]));
      })
      .finally(() => {
        setBusy(false);
      });
  };
  return (
    <ScreenStack>
      <Head
        room={room}
        onBack={onBack}
        menu={
          <RoomMenu
            self={undefined}
            settingsHref={voiceRoomSettingsPath(room.id)}
            invitationsHref={voiceInvitationsPath(room.id)}
            onEnd={() => undefined}
            onReport={report.onReport}
            t={t}
          />
        }
        t={t}
      />
      {report.dialog}
      {end !== null && <Line>{endSentence(end, t)}</Line>}
      <About room={room} t={t} />
      {/* The room's state, before who is in it; Join stays, for a place someone may have just left. */}
      {room.participants_count >= room.max_participants && (
        <FormAlert regular announce={false}>
          {t("voice.join_full")}
        </FormAlert>
      )}
      <Participants room={room} live={undefined} t={t} />
      <FrameFoot>
        <div ref={foot}>
          {/* One group: its lines at the body's leading, as far apart as a group's parts are. */}
          <Stack gap="screen-field-gap">
            <Box as="p" text="body" dir="auto">
              {t("voice.join_note")}
            </Box>
            {other !== null && other.roomId !== room.id && (
              <Box as="p" text="body" tone="muted" dir="auto">
                {t("voice.join_moves", { room: other.room.name })}
              </Box>
            )}
            {/* The page's live region says it: the notice is for the eye. */}
            {failure !== null && (
              <FormAlert regular announce={false}>
                {t(joinWords[failure])}
              </FormAlert>
            )}
            {/* The screen's one action, the foot's whole width, where the thumb is. */}
            <Stack>
              <Button kind="primary" size="form" busy={busy} onPress={join}>
                {t("voice.join")}
              </Button>
            </Stack>
          </Stack>
        </div>
      </FrameFoot>
    </ScreenStack>
  );
}

/** The output of sound each call plays on, as the reader chose it: a session cannot be asked which it plays on. */
const outputOf = new WeakMap<VoiceCall, string>();

/** The choice of where the room's sound plays, where the browser lets the page choose it, kept with the call. */
function Output({ call, t }: { call: VoiceCall; t: T }) {
  const [outputs, setOutputs] = useState<readonly AudioOutput[]>([]);
  const [chosen, setChosen] = useState(() => outputOf.get(call) ?? "");
  useEffect(() => {
    let live = true;
    const read = () => {
      void audioOutputs().then((found) => {
        if (live) setOutputs(found.filter((o) => o.id !== "" && o.id !== "default"));
      });
    };
    read();
    const unwatch = watchMediaDevices(read);
    return () => {
      live = false;
      unwatch();
    };
  }, []);
  const options = [
    { value: "", label: t("voice.output_default") },
    ...outputs.map((o, i) => ({ value: o.id, label: o.label || t("voice.output_unnamed", { number: i + 1 }) })),
  ];
  return (
    <Stack gap="entry-line">
      <GroupHeading>{t("voice.output")}</GroupHeading>
      <Box>
        <Select
          label={t("voice.output")}
          options={options}
          value={chosen}
          onChange={(id) => {
            const before = chosen;
            setChosen(id);
            call.audio.setOutputDevice(id).then(
              () => {
                outputOf.set(call, id);
              },
              () => {
                // The sound still plays where it played: the choice shows that.
                setChosen(before);
              },
            );
          }}
        />
      </Box>
    </Stack>
  );
}

/** The reaction control: it opens the emoji picker (a sheet on a phone, else a popover), and a pick is sent. */
function ReactControl({ call, sheet, t, onFailed }: { call: VoiceCall; sheet: boolean; t: T; onFailed: () => void }) {
  const { locale } = useLocale();
  const { preload, onOpen, ahead, ...picker } = usePickerData(locale);
  const [open, setOpen] = useState(false);
  useEffect(() => ahead(), [ahead]);
  const show = (shown: boolean) => {
    if (shown) {
      preload();
      onOpen();
    }
    setOpen(shown);
  };
  const pick = (emoji: PickerEmoji, shown: string, { keep }: { keep: boolean }) => {
    if (!keep) setOpen(false);
    rememberReaction("image" in emoji ? emoji.key.slice(1, -1) : shown);
    call.react(shown).catch(onFailed);
  };
  const label = t("voice.react");
  const content = <EmojiPicker {...picker} fill={sheet} autoFocus onSelect={pick} />;
  if (sheet)
    return (
      <>
        <RoomControl
          icon="react"
          label={label}
          onPress={() => {
            show(true);
          }}
        />
        <EmojiPickerSheet
          open={open}
          onClose={() => {
            setOpen(false);
          }}
          title={label}
          closeLabel={emojiStrings(locale, []).sheet.close}
        >
          {content}
        </EmojiPickerSheet>
      </>
    );
  return (
    <EmojiPickerPopover
      label={label}
      trigger={<RoomControl icon="react" label={label} />}
      open={open}
      onOpenChange={show}
      placement="top start"
    >
      {content}
    </EmojiPickerPopover>
  );
}

/** What the call goes through now, each as the sentence that says it: the connection, a host's mute, the microphone. */
function callStatus(state: VoiceCallState, t: T): string[] {
  const said: string[] = [];
  if (state.phase === "connecting") said.push(t("voice.connecting"));
  else if (state.phase === "reconnecting" || state.connection === "reconnecting" || state.connection === "lost")
    said.push(t("voice.reconnecting"));
  if (state.playbackBlocked) said.push(t("voice.hear_held"));
  if (state.mutedBy !== null && !state.micWanted) {
    const by = state.room.participants.find((p) => p.account.id === state.mutedBy);
    said.push(by === undefined ? t("voice.muted_by_someone") : t("voice.muted_by", { name: nameOf(by.account) }));
  }
  if (state.micFailure !== null) said.push(t("voice.mic_failed", { reason: state.micFailure }));
  return said;
}

/** A dialog of the host's over the room, and whom it is about. */
type Asking =
  { readonly kind: "leave" | "end" } | { readonly kind: AskedOperation; readonly target: VoiceRoomParticipant };

/** The sentence of a host's operation that did not finish. */
function failedWords(failure: HostFailure, t: T): string {
  return t("voice.host.failed", { op: opKey(failure.op), name: failure.name, cause: failure.cause });
}

/** A press of the reader's in the room that did not finish, as `voice.act_failed` names it. */
type FailedAction = "speak" | "listen" | "raise" | "lower" | "leave" | "reaction" | "end";

const failedOf: Readonly<Record<VoiceSelfOperation, FailedAction>> = {
  speak: "speak",
  listen: "listen",
  "raise-hand": "raise",
  "lower-hand": "lower",
};

/** The room while the reader is in it: the call's participants, what it goes through, and the reader's controls. */
function InRoom({
  call,
  state,
  sheet,
  focus,
  onBack,
  onLeaving,
  say,
  t,
}: {
  call: VoiceCall;
  state: VoiceCallState;
  sheet: boolean;
  /** The reader joined here: the focus goes to the room's controls. */
  focus: boolean;
  onBack: () => void;
  /** The reader pressed Leave: the focus is to go to Join once the room shows without them. */
  onLeaving: () => void;
  say: Say;
  t: T;
}) {
  const { room, self } = state;
  const overMax = useOverMax(state, t);
  // Which of the reader's presses failed last, as the line names it.
  const [failed, setFailed] = useState<FailedAction | null>(null);
  const [hostFailed, setHostFailed] = useState<HostFailure | null>(null);
  // The host's dialogs: who hosts next as they leave, ending the room, removing someone or handing the room over. The
  // call's bar of another screen asks for the first as it leads here (leave-ask.ts).
  const [asking, setAsking] = useState<Asking | null>(null);
  const askedFromBar = useLeaveAsk(room.id);
  const report = useRoomReport(room);
  const leaving = asking?.kind === "leave" || (askedFromBar && asking === null);
  const controls = useRef<HTMLDivElement>(null);
  const speaker = self?.role === "speaker";
  const connecting = state.phase === "connecting" || self === undefined;
  // Once the controls can be pressed: a disabled one takes no focus.
  useFocusInto(controls, focus && !connecting);
  const fail = (action: FailedAction) => {
    setFailed(action);
    say(t("voice.act_failed", { action }));
  };
  const act = (operation: VoiceSelfOperation) => {
    setFailed(null);
    call.act(operation).catch(() => {
      fail(failedOf[operation]);
    });
  };
  const hostFail = (failure: HostFailure | null) => {
    setHostFailed(failure);
    if (failure !== null) say(failedWords(failure, t));
  };
  /** The room's controls take the focus once a dialog has left: what it acted from may be gone with it. */
  const focusControls = () => {
    requestAnimationFrame(() => {
      controls.current?.querySelector<HTMLElement>("button:not([disabled])")?.focus();
    });
  };
  const leave = (end?: true) => {
    setFailed(null);
    // A host with others in the room chooses who hosts next, or ends it.
    if (end === undefined && hostsOthers(room, self)) {
      setAsking({ kind: "leave" });
      return;
    }
    onLeaving();
    call.leave(end === undefined ? undefined : { end }).catch((error: unknown) => {
      // Someone joined since the press: the host chooses who hosts next.
      if (isVoiceLeaveRefused(error)) setAsking({ kind: "leave" });
      else fail("leave");
    });
  };
  const own = speaker ? (
    <>
      <RoomControl
        icon="microphone"
        label={t(state.micWanted ? "voice.mute" : "voice.unmute")}
        on={!state.micWanted}
        isDisabled={connecting || state.micPending}
        onPress={() => {
          call.setMicrophone(!state.micWanted).catch(() => undefined);
        }}
      />
      <RoomControl
        icon="hear"
        label={t("voice.listen")}
        isDisabled={connecting}
        onPress={() => {
          act("listen");
        }}
      />
    </>
  ) : self !== undefined && room.can_speak ? (
    <RoomControl
      icon="microphone"
      label={t("voice.speak")}
      isDisabled={connecting}
      onPress={() => {
        act("speak");
      }}
    />
  ) : (
    <RoomControl
      icon="hand"
      label={t(self?.hand_raised === true ? "voice.lower_hand" : "voice.raise_hand")}
      on={self?.hand_raised === true}
      isDisabled={connecting}
      onPress={() => {
        act(self?.hand_raised === true ? "lower-hand" : "raise-hand");
      }}
    />
  );
  return (
    <ScreenStack busy={state.phase === "connecting"}>
      <Head
        room={room}
        onBack={onBack}
        menu={
          <RoomMenu
            self={self}
            settingsHref={voiceRoomSettingsPath(room.id)}
            invitationsHref={voiceInvitationsPath(room.id)}
            onEnd={() => {
              setAsking({ kind: "end" });
            }}
            onReport={report.onReport}
            t={t}
          />
        }
        t={t}
      />
      {report.dialog}
      <About room={room} t={t} />
      {overMax !== null && (
        <FormAlert
          regular
          announce={false}
          action={
            // The host's sentence leads to where the maximum is raised, as the room's menu does.
            overMax.host ? (
              <TextLink href={voiceRoomSettingsPath(room.id)}>{t("voice.settings.title")}</TextLink>
            ) : undefined
          }
        >
          {overMax.words}
        </FormAlert>
      )}
      {callStatus(state, t).map((words) => (
        <Line
          key={words}
          action={
            words === t("voice.hear_held") ? (
              <TextButton
                onPress={() => {
                  call.resumePlayback().catch(() => undefined);
                }}
              >
                {t("voice.hear_room")}
              </TextButton>
            ) : undefined
          }
        >
          {words}
        </Line>
      ))}
      {failed !== null && <Line>{t("voice.act_failed", { action: failed })}</Line>}
      {hostFailed !== null && <Line>{failedWords(hostFailed, t)}</Line>}
      <Participants
        room={room}
        live={{
          state,
          call,
          onHostFailed: hostFail,
          onHostAsk: (op, target) => {
            setAsking({ kind: op, target });
          },
        }}
        t={t}
      />
      {call.audio.capabilities.outputSelection && <Output call={call} t={t} />}
      <FrameFoot>
        <div ref={controls}>
          <RoomControls
            label={t("voice.controls")}
            end={
              <RoomControl
                icon="leave"
                label={t("voice.leave")}
                onPress={() => {
                  leave();
                }}
              />
            }
          >
            {own}
            <ReactControl
              call={call}
              sheet={sheet}
              t={t}
              onFailed={() => {
                fail("reaction");
              }}
            />
          </RoomControls>
        </div>
      </FrameFoot>
      <ModalPresence onExited={focusControls}>
        {leaving && self !== undefined && (
          <LeaveDialog
            key="leave"
            call={call}
            room={room}
            self={self}
            onLeft={onLeaving}
            onClose={() => {
              setAsking(null);
              clearLeaveAsk();
            }}
            t={t}
          />
        )}
        {asking?.kind === "end" && (
          <EndRoomDialog
            key="end"
            end={() => call.endRoom()}
            onEnding={onLeaving}
            onClose={() => {
              setAsking(null);
            }}
            onFailed={() => {
              fail("end");
            }}
            t={t}
          />
        )}
        {(asking?.kind === "remove" || asking?.kind === "hand-over") && (
          <HostAskDialog
            key={asking.kind}
            call={call}
            op={asking.kind}
            target={asking.target}
            onDone={(outcome) => {
              const op = asking.kind;
              const name = nameOf(asking.target.account);
              setAsking(null);
              if (outcome === "done") say(t("voice.host.done", { op: opKey(op), name }));
              else if (outcome !== "cancelled") hostFail(outcome);
            }}
            t={t}
          />
        )}
      </ModalPresence>
    </ScreenStack>
  );
}

/**
 * A room the reader does not see, or that ended: that it ended, under its name where the call or the summary knows
 * it, and why the reader left it, if they did; or that it is not open to them.
 */
function Gone({
  id,
  end,
  focus,
  onBack,
  t,
}: {
  id: string;
  end: RoomEnd | null;
  focus: boolean;
  onBack: () => void;
  t: T;
}) {
  const endedHere = end?.end.kind === "ended";
  const { summary, error, refetch } = useVoiceRoomSummary(id, !endedHere);
  const head = useRef<HTMLDivElement>(null);
  useFocusInto(head, focus);
  const back = (
    <div ref={head}>
      <Back onBack={onBack} t={t} />
    </div>
  );
  if (!endedHere && summary === undefined && error === null)
    return (
      <ScreenStack busy>
        {back}
        <FeedNotice>{t("voice.loading")}</FeedNotice>
      </ScreenStack>
    );
  if (!endedHere && summary === undefined && !(error instanceof ProblemError && error.status === 404))
    return (
      <ScreenStack>
        {back}
        <FeedNotice alert action={<TextButton onPress={refetch}>{t("voice.retry")}</TextButton>}>
          {t("voice.failed")}
        </FeedNotice>
      </ScreenStack>
    );
  const ended = endedHere || summary?.state === "ended";
  const name = end !== null && endedHere ? end.room : summary?.name;
  return (
    <ScreenStack>
      {ended && name !== undefined ? (
        <ScreenHead back={back}>
          <RoomTitle icon={endedHere ? undefined : roomIcon(summary?.icon)} name={name} />
        </ScreenHead>
      ) : (
        back
      )}
      {/* Why the room ended says that it ended; else the page says it alone. */}
      {end !== null && <Line>{endSentence(end, t)}</Line>}
      {!endedHere && <FeedNotice lead>{t(ended ? "voice.ended" : "voice.unavailable")}</FeedNotice>}
    </ScreenStack>
  );
}

/** What the page did last that moves the focus: the reader joined, or left. */
type Moved = "joined" | "left" | null;

/**
 * The page's one polite live region, there from the start: what the call goes through now and what came of a press,
 * each a new element as it comes, so that it is read once.
 */
function Said({ status, told }: { status: readonly string[]; told: { words: string; n: number } | null }) {
  return (
    <div role="status">
      {status.map((words) => (
        <Spoken key={words}>{words}</Spoken>
      ))}
      {told !== null && <Spoken key={told.n}>{told.words}</Spoken>}
    </div>
  );
}

/**
 * The page of the room of `id` for a reader signed in: before they join, while they are in it, and once it ended or
 * when they do not see it. `sheet`: the emoji picker opens as a sheet (a phone's viewport), else as a popover;
 * `onBack` goes to the rooms.
 */
export function VoiceRoomScreen({
  data,
  id,
  sheet,
  onBack,
}: {
  /** The session's data, which a join is made on. */
  data: AppData;
  id: string;
  sheet: boolean;
  onBack: () => void;
}) {
  const { t } = stringsIn(useLocale().locale);
  const off = useVoiceOff();
  const call = useVoiceCall();
  const state = useVoiceCallState();
  const end = useRoomEnd(id);
  const [moved, setMoved] = useState<Moved>(null);
  const [told, setTold] = useState<{ words: string; n: number } | null>(null);
  const say: Say = (words) => {
    setTold((before) => ({ words, n: (before?.n ?? 0) + 1 }));
  };
  const inIt = call !== null && state !== null && state.roomId === id && state.end === null;
  // A room whose call ended with the room is gone, whatever the cache still holds of it.
  // A room the reader ended here is gone at once, whatever the next read says.
  const endedByReader = useVoiceRoomEndedHere(id);
  const endedHere = end?.end.kind === "ended";
  const { room, error, refetch } = useVoiceRoom(id, !off && !inIt && !endedHere && !endedByReader);
  const endSaid = end === null ? null : endSentence(end, t);
  useEffect(() => {
    if (end !== null) sawRoomEnd(end.roomId);
  }, [end]);
  const overMax = useOverMax(inIt ? state : null, t);
  const status = [
    ...(endSaid === null || inIt ? [] : [endSaid]),
    ...(inIt ? callStatus(state, t) : []),
    ...(overMax === null ? [] : [overMax.words]),
  ];
  const page = (() => {
    if (off) return <FeedNotice lead>{t("voice.off")}</FeedNotice>;
    if (inIt)
      return (
        <InRoom
          call={call}
          state={state}
          sheet={sheet}
          focus={moved === "joined"}
          onBack={onBack}
          onLeaving={() => {
            setMoved("left");
          }}
          say={say}
          t={t}
        />
      );
    if (endedHere || endedByReader || (error instanceof ProblemError && error.status === 404))
      return <Gone id={id} end={end} focus={moved === "left"} onBack={onBack} t={t} />;
    if (error !== null)
      return (
        <FeedNotice alert action={<TextButton onPress={refetch}>{t("voice.retry")}</TextButton>}>
          {t("voice.failed")}
        </FeedNotice>
      );
    if (room !== undefined)
      return (
        <BeforeJoin
          data={data}
          room={room}
          end={end}
          focus={moved === "left"}
          onBack={onBack}
          onJoined={() => {
            setMoved("joined");
          }}
          say={say}
          t={t}
        />
      );
    return (
      <ScreenStack busy>
        <FeedNotice>{t("voice.loading")}</FeedNotice>
      </ScreenStack>
    );
  })();
  return (
    <>
      <Said status={status} told={told} />
      {page}
    </>
  );
}

/**
 * The page of the room of `id` for a visitor who is not signed in, or of another server (ADR-0100 §9): the room's
 * summary where the server shows it (its name, and its host and description while it is open), never who is in it;
 * that only this server's people join, and the sign-in (`onSignIn`), which leads into the room.
 */
export function VoiceRoomVisitor({ id, onSignIn }: { id: string; onSignIn: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  const off = useVoiceOff();
  const { summary, error, refetch } = useVoiceRoomSummary(id, !off);
  if (off || (error instanceof ProblemError && error.status === 404))
    return <FeedNotice lead>{t(off ? "voice.off" : "voice.unavailable")}</FeedNotice>;
  if (summary === undefined)
    return error === null ? (
      <ScreenStack busy>
        <FeedNotice>{t("voice.loading")}</FeedNotice>
      </ScreenStack>
    ) : (
      <FeedNotice alert action={<TextButton onPress={refetch}>{t("voice.retry")}</TextButton>}>
        {t("voice.failed")}
      </FeedNotice>
    );
  const ended = summary.state === "ended";
  return (
    <ScreenStack>
      {summary.name !== undefined && (
        <Stack gap="hair">
          <RoomTitle icon={roomIcon(summary.icon)} name={summary.name} />
          {!ended && (
            <Box as="p" tone="muted" dir="auto">
              {summary.host === undefined
                ? t("voice.open_now")
                : t("voice.open_hosted_by", { host: nameOf(summary.host) })}
            </Box>
          )}
          {!ended && summary.description !== undefined && summary.description !== "" && (
            <Box as="p" keeps="lines" dir="auto">
              {summary.description}
            </Box>
          )}
        </Stack>
      )}
      {ended ? (
        <FeedNotice lead>{t("voice.ended")}</FeedNotice>
      ) : (
        <FrameFoot>
          <Stack gap="entry-line">
            <Box as="p">{t("voice.local_only")}</Box>
            <Stack>
              <Button kind="primary" size="form" onPress={onSignIn}>
                {t("voice.sign_in_join")}
              </Button>
            </Stack>
          </Stack>
        </FrameFoot>
      )}
    </ScreenStack>
  );
}
