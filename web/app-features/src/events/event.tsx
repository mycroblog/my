// SPDX-License-Identifier: AGPL-3.0-only

// An event's page, at /events/{id} (ADR-0161 §5–§10, §15; FR-941–943, FR-946, FR-948, FR-951, FR-952), for a reader of
// this server who sees it:
//
// - Its head: back to the events, its title, and a place at its end for what its organizers may do with it.
// - Whether it was cancelled (and why, with the organizer's note) or ended, said once; when it is, in the reader's zone
//   and, where the event's own zone is another, its time there with the zone's place; who sees it, and how many go,
//   of its capacity where it has one.
// - Its description, a URL in it a link that says it leaves the site and where.
// - Its places (places.tsx), the voice room's state among them; while its room is open, the way in at the frame's foot.
// - Once its date is fixed, the reader's answer (going, maybe, not going), which saves as it is pressed and changes
//   until the start; going is refused at capacity, and the line under the answers says why an answer cannot be given;
//   and the calendar file of the event to download, a cancelled one's too. While its date is polled, the poll
//   (poll.tsx) in their place.
// - Its organizers, and who is going and maybe (people.tsx).
// - For another reader, More in its head, which reports the event and its organizer (manage.tsx, report.tsx).
// - For its organizers (manage.tsx): the menu in its head, which leads to changing it, its invitations and its
//   co-organizers, and cancels it, asked first; the approval of each attendee going or maybe (people.tsx); the poll's
//   dates added, withdrawn and confirmed from its table (poll.tsx); and why its voice room could not open, in the
//   organizer's own words (places.tsx).
//
// For a reader of its board (§17), what follows the head is two tabs of one screen, Details (all of the above under the
// head) and Board (board.tsx), the address the event's with a search alone (`?tab=board`); a reader who may not read
// it gets no tabs and no sign of the board. When a saved answer gives or takes the board, the tabs come or go at once
// and the focus stays on the answer pressed; a reader who loses the board on its tab is on the page without tabs, the
// address the event's, the focus on the title.
//
// The page is drawn again as the event starts and as it ends. An event the reader does not see, or that is not there
// (the server's 404), says so. What came of a press that the page does not show in the control's own line (a
// download, a join that failed) is said once, in the one polite live region the page keeps from the start, a new
// element of it each time; the focus stays on the control pressed.

import { nameOf, useEventWhen, useGoingCount } from "@my/app-components";
import {
  eventBoardClosesAt,
  eventBoardQuery,
  eventPhase,
  eventQuery,
  eventRefusal,
  eventRoom,
  useEvent,
  useEventWrites,
  type AppData,
  type EventAnswer,
  type EventPhase,
  type EventWrites,
  type ServerEvent,
} from "@my/app-data";
import {
  AnswerChoice,
  BackButton,
  Box,
  Button,
  EventDescription,
  EventTime,
  FeedNotice,
  focusFrameHeading,
  FrameFoot,
  GroupHeading,
  HeadcountMark,
  Icon,
  ModalPresence,
  Row,
  SaveState,
  ScreenStack,
  ScreenTitle,
  Spoken,
  Stack,
  TabRow,
  useLocale,
  type Answer,
} from "@my/design-system";
import { useEffect, useEffectEvent, useId, useLayoutEffect, useRef, useState } from "react";
import { ReportDialog } from "../report";
import { saveFile } from "../save-file";
import { Board } from "./board";
import { descriptionParts } from "./description";
import { CancelDialog, EventMenu } from "./manage";
import { Attendees, Organizers } from "./people";
import { hasPlaces, Places, RoomFoot, type JoinRoom, type Say } from "./places";
import { Poll, PollAskDialog, type PollAsk } from "./poll";
import { focusWhenFree, Retry, useEventStrings, type T } from "./parts";

/**
 * What the page's loader waits for: the event, from the cache unless a write marked it stale; on the Board tab, the
 * board's first page too, for a reader of it.
 */
export async function loadEvent(data: AppData, id: string, tab?: EventTab): Promise<void> {
  const event = await data.queryClient.query({ ...eventQuery(data, id), staleTime: Infinity }).catch(() => undefined);
  if (tab !== "board" || event?.viewer.board.readable !== true) return;
  await data.queryClient.infiniteQuery({ ...eventBoardQuery(data, id), staleTime: Infinity }).catch(() => undefined);
}

/** The title of the event of `id` once it is known, for the screen's heading and the document's title. */
export function useEventTitle(id: string, signedIn: boolean): string | undefined {
  return useEvent(id, signedIn).event?.title;
}

const answerOf: Readonly<Record<EventAnswer, Answer>> = { going: "yes", maybe: "maybe", declined: "no" };
const eventAnswerOf: Readonly<Record<Answer, EventAnswer>> = { yes: "going", maybe: "maybe", no: "declined" };

/** Whether `event`, in `phase`, takes no new going: its going reached its capacity, the reader's own not among them. */
const isFull = (event: ServerEvent) =>
  event.capacity > 0 && event.going_count >= event.capacity && event.viewer.answer !== "going";

/**
 * The moment the page draws at, drawn again as the event starts, as it ends and as its board's 7 days after the end
 * pass, so that what follows from it (the answers closed at the start, the voice room, that it ended, the board
 * read-only) changes with the time and not with the next read.
 */
function useMoment(event: ServerEvent): number {
  const [now, setNow] = useState(() => Date.now());
  const next = [Date.parse(event.starts_at), Date.parse(event.ends_at), eventBoardClosesAt(event)]
    .filter((at) => at > now)
    .toSorted((a, b) => a - b)[0];
  useEffect(() => {
    if (next === undefined) return;
    // A timer waits 2^31 − 1 ms at most: a later moment is waited for again from there.
    const timer = setTimeout(
      () => {
        setNow(Date.now());
      },
      Math.min(next - Date.now() + 50, 2_147_483_647),
    );
    return () => {
      clearTimeout(timer);
    };
  }, [next, now]);
  return now;
}

/**
 * That the event was cancelled or ended, once, above its date, marked as its line in a list marks it (the glyph and
 * the word); under a cancellation, who or what cancelled it and the organizer's note, as the page's text.
 */
function State({ event, phase, t }: { event: ServerEvent; phase: EventPhase; t: T }) {
  if (phase !== "canceled" && phase !== "ended") return null;
  const note = event.cancellation?.note;
  return (
    <Stack gap="entry-line">
      <Row align="center" gap="entry-caption" weight="label" text="body">
        <Icon name={phase === "canceled" ? "eventCancelled" : "eventEnded"} place="action" />
        <span>{t("events.mark", { state: phase })}</span>
      </Row>
      {phase === "canceled" && (
        <Box as="p" text="body" dir="auto">
          {t("events.canceled", { reason: event.cancellation?.reason ?? "organizer" })}
        </Box>
      )}
      {phase === "canceled" && note !== undefined && note !== "" && (
        <Box as="p" text="body" keeps="lines" dir="auto">
          {note}
        </Box>
      )}
    </Stack>
  );
}

/** When the event is, and who sees it, and how many go: of its capacity where it has one. */
function When({ event, phase, t }: { event: ServerEvent; phase: EventPhase; t: T }) {
  const { locale } = useLocale();
  const when = useEventWhen(event);
  const going = useGoingCount(event.going_count, phase === "ended");
  const organizer = nameOf(event.organizer);
  const n = (v: number) => new Intl.NumberFormat(locale).format(v);
  const count =
    event.capacity > 0
      ? {
          shown: `${n(event.going_count)}/${n(event.capacity)}`,
          label: t("events.capacity", { going: event.going_count, capacity: event.capacity }),
        }
      : going;
  return (
    <Stack gap="entry-line">
      {phase === "polling" ? (
        <Box as="p" text="body" dir="auto">
          {t("events.date_polled")}
        </Box>
      ) : (
        <Box text="body">
          <EventTime {...when} />
        </Box>
      )}
      <Row wrap align="center" gapX="entry-caption" text="caption" tone="muted">
        <span dir="auto">{t("events.visibility", { visibility: event.visibility, organizer })}</span>
        {(event.capacity > 0 || event.going_count > 0) && <HeadcountMark count={count} />}
      </Row>
    </Stack>
  );
}

/** What came of the reader's last answer: on its way, saved, or refused (with the refusal's words). */
type Saving =
  | { readonly state: "saving" }
  | { readonly state: "saved" }
  | { readonly state: "failed"; readonly answer: EventAnswer; readonly words?: string };

/** The words of a refused answer to the event, by what the server said; none for a failure to try again. */
function refusedWords(error: unknown, t: T): string | undefined {
  switch (eventRefusal(error)) {
    case "full":
      return t("events.answer_refused_full");
    case "closed":
      return t("events.answer_refused_closed");
    case "unscheduled":
      return t("events.answer_refused_unscheduled");
    default:
      return undefined;
  }
}

/**
 * The line under the answers: why they cannot be given now, where the page says it nowhere else (an event cancelled
 * or ended says so at its top).
 */
function answerNote(event: ServerEvent, phase: EventPhase, t: T): string | undefined {
  if (phase === "started") return t("events.answer_started");
  if (phase === "upcoming" && isFull(event)) return t("events.answer_full");
  return undefined;
}

/**
 * The reader's answer to the event: saves as it is pressed, while the event is to come. What came of it (`saving`) is
 * held by the page, so that it stays as the tabs come or go above it; `onPressed` hears each answer pressed.
 */
function Answering({
  event,
  phase,
  writes,
  saving,
  setSaving,
  onPressed,
  t,
}: {
  event: ServerEvent;
  phase: EventPhase;
  writes: EventWrites;
  saving: Saving | null;
  setSaving: (s: Saving) => void;
  onPressed: () => void;
  t: T;
}) {
  const answer = (chosen: EventAnswer) => {
    onPressed();
    setSaving({ state: "saving" });
    writes.answer(chosen).then(
      () => {
        setSaving({ state: "saved" });
      },
      (error: unknown) => {
        const words = refusedWords(error, t);
        setSaving(
          words === undefined ? { state: "failed", answer: chosen } : { state: "failed", answer: chosen, words },
        );
      },
    );
  };
  const retry = saving?.state === "failed" && saving.words === undefined ? saving.answer : undefined;
  return (
    <Stack gap="entry-line">
      <GroupHeading>{t("events.your_answer")}</GroupHeading>
      <AnswerChoice
        label={t("events.your_answer")}
        names={{
          yes: t("events.answer", { answer: "going" }),
          maybe: t("events.answer", { answer: "maybe" }),
          no: t("events.answer", { answer: "declined" }),
        }}
        value={event.viewer.answer === undefined ? null : answerOf[event.viewer.answer]}
        onChange={(chosen) => {
          answer(eventAnswerOf[chosen]);
        }}
        isDisabled={phase !== "upcoming"}
        disabled={isFull(event) ? ["yes"] : []}
        note={answerNote(event, phase, t)}
        save={
          saving === null ? null : (
            <SaveState
              state={saving.state}
              onRetry={
                retry === undefined
                  ? undefined
                  : () => {
                      answer(retry);
                    }
              }
            >
              {saving.state === "failed" ? saving.words : undefined}
            </SaveState>
          )
        }
      />
    </Stack>
  );
}

/** The calendar file of the event, to download: the button stays, and what came of it is said beside it. */
function Calendar({ writes, say, t }: { writes: EventWrites; say: Say; t: T }) {
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<string | null>(null);
  const download = () => {
    if (busy) return;
    setBusy(true);
    setResult(null);
    writes.calendar().then(
      (file) => {
        saveFile(file.blob, file.name);
        const words = t("events.calendar_saved", { name: file.name });
        setResult(words);
        say(words);
        setBusy(false);
      },
      (error: unknown) => {
        const words = t(
          eventRefusal(error) === "unscheduled" ? "events.calendar_unscheduled" : "events.calendar_failed",
        );
        setResult(words);
        say(words);
        setBusy(false);
      },
    );
  };
  return (
    <Stack gap="entry-line">
      <Box>
        <Button size="target" busy={busy} onPress={download}>
          {t("events.calendar")}
        </Button>
      </Box>
      {/* For the eye: the page's live region says it. */}
      {result !== null && (
        <Box as="p" tone="muted" dir="auto" aria-hidden="true">
          {result}
        </Box>
      )}
    </Stack>
  );
}

/** The tabs under the event's head, for a reader of its board. */
export type EventTab = "details" | "board";

/**
 * Keeps the focus on the answer pressed as the tabs come or go above the answers once it is saved (the answers are
 * drawn anew under another parent): `pressed` marks an answer pressed, and once the board's access changed, the answer
 * chosen (else the first) takes the focus again, while nothing else took it meanwhile.
 */
function useAnswerKept(readable: boolean, answers: string) {
  const pressed = useRef(false);
  const before = useRef(readable);
  useLayoutEffect(() => {
    if (before.current === readable) return;
    before.current = readable;
    if (!pressed.current) return;
    pressed.current = false;
    const active = document.activeElement;
    if (active !== null && active !== document.body) return;
    const group = document.getElementById(answers);
    (group?.querySelector<HTMLElement>("input:checked") ?? group?.querySelector<HTMLElement>("input"))?.focus();
  }, [readable, answers]);
  return () => {
    pressed.current = true;
  };
}

/** The event as the reader sees it; `refetch` reads it again. */
function Shown({
  event,
  tab,
  onTab,
  join,
  say,
  refetch,
  onBack,
  t,
}: {
  event: ServerEvent;
  tab: EventTab;
  onTab: (tab: EventTab) => void;
  join: JoinRoom;
  say: Say;
  refetch: () => void;
  onBack: () => void;
  t: T;
}) {
  const writes = useEventWrites(event.id);
  const now = useMoment(event);
  const phase = eventPhase(event, now);
  const room = eventRoom(event, now);
  const title = useRef<HTMLHeadingElement>(null);
  const readable = event.viewer.board.readable;
  const [saving, setSaving] = useState<Saving | null>(null);
  const answers = useId();
  const pressedAnswer = useAnswerKept(readable, answers);
  // A reader who loses the board on its tab is on the page without tabs, the address the event's, the focus on the
  // screen's h1, once the composer that may hold it is gone; one who opens the Board tab's address without the board is
  // on the event's address.
  const hadBoard = useRef(readable);
  const leaveBoard = useEffectEvent(() => {
    onTab("details");
  });
  useLayoutEffect(() => {
    const lost = hadBoard.current && !readable;
    hadBoard.current = readable;
    if (readable || tab !== "board") return;
    leaveBoard();
    if (lost)
      focusWhenFree(() => {
        focusFrameHeading(document);
      });
  }, [readable, tab]);
  const [cancelling, setCancelling] = useState(false);
  const [reporting, setReporting] = useState(false);
  // The cancellation was done: the title takes the focus once the dialog is out of the document.
  const cancelled = useRef(false);
  const poll = useRef<HTMLDivElement>(null);
  const [pollAsk, setPollAsk] = useState<PollAsk | null>(null);
  // What a done confirmation of the poll leaves the focus to, once its dialog is out of the document: the title over
  // the date it fixed, or the poll's first date's menu after a date was withdrawn.
  const pollDone = useRef<PollAsk["op"] | null>(null);
  // What the event is, the reader's answer and who goes: the page itself, or its Details tab for a reader of the board.
  const details = (
    <>
      {event.description !== "" && <EventDescription parts={descriptionParts(event.description)} />}
      {hasPlaces(event) && (
        <Stack gap="entry-line">
          <GroupHeading>{t("events.where")}</GroupHeading>
          <Places event={event} now={now} refetch={refetch} t={t} />
        </Stack>
      )}
      {phase === "polling" ? (
        <Stack gap="entry-line">
          <GroupHeading>{t("events.poll_heading")}</GroupHeading>
          <div ref={poll}>
            <Poll event={event} writes={writes} say={say} onAsk={setPollAsk} t={t} />
          </div>
        </Stack>
      ) : (
        <Stack gap="entry-room">
          <Stack id={answers}>
            <Answering
              event={event}
              phase={phase}
              writes={writes}
              saving={saving}
              setSaving={setSaving}
              onPressed={pressedAnswer}
              t={t}
            />
          </Stack>
          {/* A cancelled event's file says it is cancelled, which updates the entry a calendar imported before. */}
          {phase !== "ended" && <Calendar writes={writes} say={say} t={t} />}
        </Stack>
      )}
      <Organizers event={event} t={t} />
      <Attendees event={event} phase={phase} say={say} t={t} />
    </>
  );
  return (
    <ScreenStack>
      <ScreenTitle
        ref={title}
        back={<BackButton onPress={onBack}>{t("events.back")}</BackButton>}
        actions={
          <EventMenu
            event={event}
            onCancel={() => {
              setCancelling(true);
            }}
            onReport={() => {
              setReporting(true);
            }}
            t={t}
          />
        }
      >
        {event.title}
      </ScreenTitle>
      <State event={event} phase={phase} t={t} />
      <When event={event} phase={phase} t={t} />
      {readable ? (
        <TabRow
          label={event.title}
          tabs={[
            { id: "details", name: t("events.tab", { tab: "details" }) },
            { id: "board", name: t("events.tab", { tab: "board" }) },
          ]}
          current={tab}
          onChange={onTab}
        >
          {tab === "board" ? <Board event={event} now={now} t={t} /> : <ScreenStack>{details}</ScreenStack>}
        </TabRow>
      ) : (
        details
      )}
      {room.state === "open" && <RoomFoot roomId={room.roomId} join={join} say={say} t={t} />}
      <ModalPresence>
        {reporting && (
          <ReportDialog
            key="report"
            subject={{ kind: "event", account: event.organizer, id: event.id, name: event.title }}
            onClose={() => {
              setReporting(false);
            }}
          />
        )}
        {cancelling && (
          <CancelDialog
            key="cancel"
            writes={writes}
            onCancelled={() => {
              cancelled.current = true;
              setCancelling(false);
              say(t("events.cancel.done"));
            }}
            onClosed={() => {
              // Cancelled already, from elsewhere: the page reads the event again, and says so.
              cancelled.current = true;
              setCancelling(false);
              refetch();
              say(t("events.cancel.closed"));
            }}
            onClose={() => {
              setCancelling(false);
            }}
            onGone={() => {
              if (!cancelled.current) return;
              cancelled.current = false;
              title.current?.focus();
            }}
            t={t}
          />
        )}
        {pollAsk !== null && (
          <PollAskDialog
            key={`poll-${pollAsk.op}`}
            ask={pollAsk}
            writes={writes}
            onDone={(outcome) => {
              setPollAsk(null);
              if (outcome === "cancelled") return;
              if (outcome === "done") {
                pollDone.current = pollAsk.op;
                say(t("events.poll.done", { op: pollAsk.op, date: pollAsk.date }));
              } else say(outcome.failed);
            }}
            onGone={() => {
              const done = pollDone.current;
              pollDone.current = null;
              if (done === "confirm") title.current?.focus();
              else if (done === "withdraw")
                poll.current?.querySelector<HTMLElement>("[role=table] button:not([disabled])")?.focus();
            }}
            t={t}
          />
        )}
      </ModalPresence>
    </ScreenStack>
  );
}

/** The page's one polite live region, there from the start: what came of a press, a new element each time. */
function Said({ told }: { told: { words: string; n: number } | null }) {
  return <div role="status">{told !== null && <Spoken key={told.n}>{told.words}</Spoken>}</div>;
}

/**
 * The page of the event of `id` for a reader signed in, on its tab `tab` (the address's) for a reader of its board.
 * `onTab` opens the other tab in its place, and the Details tab when the reader may not read the board; `join` joins
 * its open voice room from the reader's press and shows the room; `onBack` goes to the events.
 */
export function EventScreen({
  id,
  tab = "details",
  onTab,
  join,
  onBack,
}: {
  id: string;
  tab?: EventTab | undefined;
  onTab: (tab: EventTab) => void;
  join: JoinRoom;
  onBack: () => void;
}) {
  const t = useEventStrings();
  const { event, error, gone, refetch } = useEvent(id);
  const [told, setTold] = useState<{ words: string; n: number } | null>(null);
  const say: Say = (words) => {
    setTold((before) => ({ words, n: (before?.n ?? 0) + 1 }));
  };
  const back = <BackButton onPress={onBack}>{t("events.back")}</BackButton>;
  const page = (() => {
    if (gone)
      return (
        <ScreenStack>
          {back}
          <FeedNotice lead>{t("events.gone")}</FeedNotice>
        </ScreenStack>
      );
    if (event !== undefined)
      return (
        <Shown event={event} tab={tab} onTab={onTab} join={join} say={say} refetch={refetch} onBack={onBack} t={t} />
      );
    if (error !== null)
      return (
        <ScreenStack>
          {back}
          <FeedNotice alert action={<Retry onPress={refetch} t={t} />}>
            {t("events.event_failed")}
          </FeedNotice>
        </ScreenStack>
      );
    return (
      <ScreenStack busy>
        {back}
        <FeedNotice>{t("events.event_loading")}</FeedNotice>
      </ScreenStack>
    );
  })();
  return (
    <>
      <Said told={told} />
      {page}
    </>
  );
}

/**
 * The page of an event for a visitor who is not signed in: only this server's people see its events, and the sign-in
 * (`onSignIn`), which leads back to the event.
 */
export function EventVisitor({ onSignIn }: { onSignIn: () => void }) {
  const t = useEventStrings();
  return (
    // The screen's one sign-in, at the frame's foot where the thumb is, as a voice room's visitor has it.
    <FrameFoot>
      <Stack gap="entry-line">
        <Box as="p" text="body">
          {t("events.local_only")}
        </Box>
        <Stack>
          <Button kind="primary" size="form" onPress={onSignIn}>
            {t("events.sign_in")}
          </Button>
        </Stack>
      </Stack>
    </FrameFoot>
  );
}
