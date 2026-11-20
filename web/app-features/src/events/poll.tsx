// SPDX-License-Identifier: AGPL-3.0-only

// The date poll of an event on its page (ADR-0161 §7, FR-948, FR-952): its candidate dates as a table, a date a row,
// how many can go, may and cannot, and the reader's own answer, which saves as it is pressed; under the table, each
// date whose answer failed, with its own Try again, and what came of the last one pressed; under it, who answered
// each date, read once the reader opens it. Until a date is confirmed there is no answer to the event and no calendar
// file: the poll is what the reader answers.
//
// For its organizers, each date's menu at the end of its row confirms it (which fixes the event's date and closes the
// poll) or withdraws it (the last one stays), each asked first by the page, whose dialog outlives the poll; and under
// the table, a date to add, checked as the server checks a date. What came of each is said in the page's live region.

import { AccountLine, eventWhen, localInput, localInstant, useEventWhen } from "@my/app-components";
import {
  eventLimits,
  eventRefusal,
  eventScheduleFault,
  latestEventStart,
  organizesEvent,
  useEventBallots,
  useEventCandidates,
  type EventBallot,
  type EventCandidate,
  type EventVote,
  type EventWrites,
  type ServerEvent,
} from "@my/app-data";
import {
  Box,
  Button,
  CandidateRow,
  CandidateTable,
  ConfirmDialog,
  Disclosure,
  EventTime,
  FeedNotice,
  FormAlert,
  GroupHeading,
  IconButton,
  MenuGroup,
  MenuItem,
  MenuSurface,
  Row,
  RowItem,
  RowList,
  SaveState,
  ShowMore,
  Stack,
  useAddedFocus,
  useLocale,
  useTimeZone,
  type Answer,
} from "@my/design-system";
import { useId, useState, type ReactNode } from "react";
import { useGone } from "./manage";
import { DateField, Retry, type T } from "./parts";
import type { Say } from "./places";

/** What the organizers confirm before it is done to a date: confirming it, or withdrawing it. */
export type PollAsk = {
  readonly op: "confirm" | "withdraw";
  readonly candidate: EventCandidate;
  readonly date: string;
};

/**
 * What came of the reader's answer to a date: on its way, saved, or failed, with the answer to send again and, for a
 * refusal of the server, its words.
 */
type Saving =
  | { readonly state: "saving" }
  | { readonly state: "saved" }
  | { readonly state: "failed"; readonly vote: EventVote; readonly words?: string };

const votes: readonly EventVote[] = ["yes", "maybe", "no"];

/** A date of the poll, as a row of its table. */
function DateRow({
  event,
  candidate,
  closed,
  onAnswer,
  actions,
}: {
  event: ServerEvent;
  candidate: EventCandidate;
  closed: boolean;
  onAnswer: (vote: Answer) => void;
  actions?: ReactNode;
}) {
  const { locale } = useLocale();
  const when = useEventWhen({ ...candidate, time_zone: event.time_zone });
  const n = (v: number) => new Intl.NumberFormat(locale).format(v);
  return (
    <CandidateRow
      time={<EventTime {...when} />}
      counts={{ yes: n(candidate.yes_count), maybe: n(candidate.maybe_count), no: n(candidate.no_count) }}
      answer={candidate.vote ?? null}
      onAnswer={onAnswer}
      isDisabled={closed}
      actions={actions}
    />
  );
}

/** What the organizers may do with a date: confirm it, or withdraw it, last and apart; each asked first. */
function DateActions({
  date,
  last,
  onAsk,
  t,
}: {
  /** The date as the reader reads it, which names its menu. */
  date: string;
  /** It is the poll's only date, which stays. */
  last: boolean;
  onAsk: (op: PollAsk["op"]) => void;
  t: T;
}) {
  const label = t("events.poll.menu", { date });
  return (
    <MenuSurface trigger={<IconButton icon="more" label={label} square />} label={label} placement="bottom end">
      <MenuGroup>
        <MenuItem
          id="confirm"
          icon="dateFixed"
          onAction={() => {
            onAsk("confirm");
          }}
        >
          {t("events.poll.op", { op: "confirm" })}
        </MenuItem>
      </MenuGroup>
      <MenuGroup>
        <MenuItem
          id="withdraw"
          icon="delete"
          disabled={last}
          about={last ? t("events.poll.last") : undefined}
          onAction={() => {
            onAsk("withdraw");
          }}
        >
          {t("events.poll.op", { op: "withdraw" })}
        </MenuItem>
      </MenuGroup>
    </MenuSurface>
  );
}

/**
 * The confirmation of what the organizers do to a date (confirming it, withdrawing it), held by the page so that it
 * outlives the poll that a confirmed date takes away: `onDone` says whether it was done, cancelled, or why not, in
 * words; `onGone` runs once the dialog is out of the document.
 */
export function PollAskDialog({
  ask,
  writes,
  onDone,
  onGone,
  t,
}: {
  ask: PollAsk;
  writes: EventWrites;
  onDone: (outcome: "done" | "cancelled" | { readonly failed: string }) => void;
  onGone: () => void;
  t: T;
}) {
  const [busy, setBusy] = useState(false);
  useGone(onGone);
  const { op, candidate, date } = ask;
  return (
    <ConfirmDialog
      title={t("events.poll.ask_title", { op, date })}
      cancel={t("events.poll.ask_cancel", { op })}
      confirm={t("events.poll.ask_confirm", { op })}
      busy={busy}
      onCancel={() => {
        if (!busy) onDone("cancelled");
      }}
      onConfirm={() => {
        setBusy(true);
        const write = op === "confirm" ? writes.confirmCandidate(candidate.id) : writes.withdrawCandidate(candidate.id);
        write.then(
          () => {
            onDone("done");
          },
          (error: unknown) => {
            const refusal = eventRefusal(error);
            // What made the server refuse changed the poll or the event: they are read again, as they are now.
            if (refusal !== null) writes.reread();
            const cause =
              refusal === "last-candidate"
                ? "last"
                : refusal === "scheduled"
                  ? "scheduled"
                  : refusal === "invalid"
                    ? "started"
                    : refusal === "closed"
                      ? "closed"
                      : "other";
            onDone({ failed: t("events.poll.act_failed", { op, date, cause }) });
          },
        );
      }}
    >
      {t("events.poll.ask_text", { op })}
    </ConfirmDialog>
  );
}

/** What the server refuses of a date added to a poll, in words. */
function addRefusal(error: unknown, t: T): string {
  switch (eventRefusal(error)) {
    case "invalid":
      return t("events.poll.add_refused", { cause: "invalid" });
    case "scheduled":
      return t("events.poll.add_refused", { cause: "scheduled" });
    case "closed":
      return t("events.poll.add_refused", { cause: "closed" });
    default:
      return t("events.poll.add_refused", { cause: "other" });
  }
}

/**
 * A date to add to the poll, under its table: its start and end in the event's zone, checked as the server checks a
 * date and against the dates the poll has; Add sends it, and what came of it is said in the page's live region.
 */
function AddDate({
  event,
  candidates,
  add,
  say,
  t,
}: {
  event: ServerEvent;
  candidates: readonly EventCandidate[];
  add: EventWrites["addCandidate"];
  say: Say;
  t: T;
}) {
  const id = useId();
  const { locale } = useLocale();
  const timeZone = useTimeZone();
  const zone = event.time_zone;
  const [span, setSpan] = useState(() => {
    const last = candidates.at(-1);
    const start = last === undefined ? Date.now() + 24 * 3_600_000 : Date.parse(last.starts_at) + 24 * 3_600_000;
    const length = last === undefined ? 2 * 3_600_000 : Date.parse(last.ends_at) - Date.parse(last.starts_at);
    return { start: localInput(new Date(start), zone), end: localInput(new Date(start + length), zone) };
  });
  const [faults, setFaults] = useState<{ start?: string; end?: string; form?: string }>({});
  const [busy, setBusy] = useState(false);
  if (candidates.length >= eventLimits.candidates)
    return (
      <Box as="p" text="body">
        {t("events.poll.full", { count: eventLimits.candidates })}
      </Box>
    );
  const now = new Date();
  const startsAt = localInstant(span.start, zone);
  const endsAt = localInstant(span.end, zone);
  const send = () => {
    if (busy) return;
    const fault = eventScheduleFault(startsAt, endsAt, now);
    const twice = candidates.some(
      (c) => Date.parse(c.starts_at) === startsAt.getTime() && Date.parse(c.ends_at) === endsAt.getTime(),
    );
    if (fault !== null || twice) {
      setFaults(
        fault === "starts_at"
          ? { start: t("events.form.start_rule") }
          : fault === "ends_at"
            ? { end: t("events.form.end_rule") }
            : { start: t("events.form.candidate_twice") },
      );
      return;
    }
    setFaults({});
    setBusy(true);
    const date = eventWhen(locale, timeZone, {
      starts_at: startsAt.toISOString(),
      ends_at: endsAt.toISOString(),
      time_zone: zone,
    }).shown;
    add({ starts_at: startsAt.toISOString(), ends_at: endsAt.toISOString() }).then(
      () => {
        setBusy(false);
        say(t("events.poll.added", { date }));
      },
      (error: unknown) => {
        setBusy(false);
        const words = addRefusal(error, t);
        setFaults({ form: words });
        say(words);
      },
    );
  };
  return (
    <Stack gap="entry-line" role="group" aria-label={t("events.poll.add_heading")}>
      <GroupHeading>{t("events.poll.add_heading")}</GroupHeading>
      <Row wrap gap="entry-room">
        <DateField
          id={`${id}-start`}
          label={t("events.form.starts")}
          value={span.start}
          onChange={(start) => {
            setSpan({ ...span, start });
            setFaults({});
          }}
          min={localInput(now, zone)}
          max={localInput(latestEventStart(now), zone)}
          error={faults.start}
          zoneId={`${id}-zone`}
        />
        <DateField
          id={`${id}-end`}
          label={t("events.form.ends")}
          value={span.end}
          onChange={(end) => {
            setSpan({ ...span, end });
            setFaults({});
          }}
          min={span.start}
          error={faults.end}
          zoneId={`${id}-zone`}
        />
      </Row>
      <Box as="p" id={`${id}-zone`} text="caption" tone="muted">
        {t("events.form.zone_note", { zone })}
      </Box>
      {faults.form !== undefined && <FormAlert regular>{faults.form}</FormAlert>}
      <Box>
        <Button size="target" busy={busy} onPress={send}>
          {t("events.poll.add")}
        </Button>
      </Box>
    </Stack>
  );
}

/** Who answered a date, each way: read while it is open. */
function Ballots({ event, candidate, t }: { event: ServerEvent; candidate: EventCandidate; t: T }) {
  const list = useEventBallots(event.id, candidate.id);
  const added = useAddedFocus(list.items.length);
  if (list.status === "pending") return <FeedNotice start>{t("events.ballots_loading")}</FeedNotice>;
  if (list.items.length === 0)
    return list.status === "error" ? (
      <FeedNotice alert start action={<Retry onPress={list.refetch} t={t} />}>
        {t("events.ballots_failed")}
      </FeedNotice>
    ) : (
      <FeedNotice start>{t("events.ballots_none")}</FeedNotice>
    );
  const of = (vote: EventVote): readonly EventBallot[] => list.items.filter((b) => b.vote === vote);
  // Each way is counted by the date's own counts, the server's, not by the pages read.
  const counted: Readonly<Record<EventVote, number>> = {
    yes: candidate.yes_count,
    maybe: candidate.maybe_count,
    no: candidate.no_count,
  };
  return (
    <Stack gap="entry-room">
      {votes.map((vote) =>
        of(vote).length === 0 ? null : (
          <Stack key={vote} gap="entry-line">
            <GroupHeading>{t("events.vote_group", { vote, count: counted[vote] })}</GroupHeading>
            <RowList label={t("events.vote_group", { vote, count: counted[vote] })}>
              {of(vote).map((b) => (
                <RowItem key={b.account.id} itemRef={added.item(list.items.indexOf(b))}>
                  <AccountLine account={b.account} />
                </RowItem>
              ))}
            </RowList>
          </Stack>
        ),
      )}
      <ShowMore
        more={list.hasNextPage}
        busy={list.isFetchingNextPage}
        failed={list.failedNextPage}
        onPress={added.press(list.fetchNextPage)}
      />
    </Stack>
  );
}

/** Who answered one date, folded until the reader opens it. */
function WhoAnswered({ event, candidate, t }: { event: ServerEvent; candidate: EventCandidate; t: T }) {
  const { locale } = useLocale();
  const timeZone = useTimeZone();
  const [open, setOpen] = useState(false);
  const answered = candidate.yes_count + candidate.maybe_count + candidate.no_count;
  const date = eventWhen(locale, timeZone, { ...candidate, time_zone: event.time_zone }).shown;
  return (
    <Disclosure
      summary={t("events.who_answered", { date, count: answered })}
      icon="members"
      expanded={open}
      onExpandedChange={setOpen}
    >
      {open && <Ballots event={event} candidate={candidate} t={t} />}
    </Disclosure>
  );
}

/** The words of a refused answer to a date, by what the server said. */
function refusedWords(error: unknown, t: T): string | undefined {
  const refusal = eventRefusal(error);
  if (refusal === "scheduled") return t("events.vote_scheduled");
  if (refusal === "closed") return t("events.vote_closed");
  return undefined;
}

/**
 * What the poll's save line says of each date the reader answered here: every one that failed, each with its date and
 * its own Try again, then what came of the last one pressed while it saves or saved.
 */
function SaveLine({
  candidates,
  saving,
  last,
  onRetry,
  dateOf,
  t,
}: {
  candidates: readonly EventCandidate[];
  saving: ReadonlyMap<string, Saving>;
  last: string | null;
  onRetry: (candidateId: string, vote: EventVote) => void;
  dateOf: (candidate: EventCandidate) => string;
  t: T;
}): ReactNode {
  const failed = candidates.filter((c) => saving.get(c.id)?.state === "failed");
  const latest = last === null ? undefined : saving.get(last);
  if (failed.length === 0 && (latest === undefined || latest.state === "failed")) return null;
  return (
    <Stack gap="hair">
      {failed.map((candidate) => {
        const s = saving.get(candidate.id);
        if (s?.state !== "failed") return null;
        return (
          <SaveState
            key={candidate.id}
            state="failed"
            onRetry={
              s.words === undefined
                ? () => {
                    onRetry(candidate.id, s.vote);
                  }
                : undefined
            }
          >
            {s.words ?? t("events.vote_failed", { date: dateOf(candidate) })}
          </SaveState>
        );
      })}
      {latest !== undefined && latest.state !== "failed" && <SaveState state={latest.state} />}
    </Stack>
  );
}

/**
 * The poll of `event`: its table, what came of each answer to a date, and who answered each date; for its organizers,
 * each date's menu, whose confirmations the page asks (`onAsk`), and a date to add.
 */
export function Poll({
  event,
  writes,
  say,
  onAsk,
  t,
}: {
  event: ServerEvent;
  writes: EventWrites;
  say: Say;
  onAsk: (ask: PollAsk) => void;
  t: T;
}): ReactNode {
  const { locale } = useLocale();
  const timeZone = useTimeZone();
  const list = useEventCandidates(event.id);
  // What came of the answer to each date, so that a failed one is never hidden by a later one to another date.
  const [saving, setSaving] = useState<ReadonlyMap<string, Saving>>(new Map());
  const [last, setLast] = useState<string | null>(null);
  const closed = event.state === "cancelled";
  const manages = organizesEvent(event) && !closed;
  const mark = (candidateId: string, state: Saving) => {
    setSaving((before) => new Map(before).set(candidateId, state));
  };
  const vote = (candidateId: string, answer: EventVote) => {
    setLast(candidateId);
    mark(candidateId, { state: "saving" });
    writes.vote(candidateId, answer).then(
      () => {
        mark(candidateId, { state: "saved" });
      },
      (error: unknown) => {
        const words = refusedWords(error, t);
        mark(
          candidateId,
          words === undefined ? { state: "failed", vote: answer } : { state: "failed", vote: answer, words },
        );
      },
    );
  };
  if (list.status === "pending") return <FeedNotice start>{t("events.poll_loading")}</FeedNotice>;
  if (list.status === "error" && list.items.length === 0)
    return (
      <FeedNotice alert start action={<Retry onPress={list.refetch} t={t} />}>
        {t("events.poll_failed")}
      </FeedNotice>
    );
  const dateOf = (candidate: EventCandidate) =>
    eventWhen(locale, timeZone, { ...candidate, time_zone: event.time_zone }).shown;
  return (
    <Stack gap="entry-room">
      <Stack gap="entry-line">
        <Box as="p" text="body" dir="auto">
          {t("events.poll_note")}
        </Box>
        <CandidateTable
          label={t("events.poll")}
          time={t("events.poll_date")}
          names={{
            yes: t("events.vote", { vote: "yes" }),
            maybe: t("events.vote", { vote: "maybe" }),
            no: t("events.vote", { vote: "no" }),
          }}
          yours={t("events.yours")}
          actions={manages ? t("events.poll.actions") : undefined}
          save={<SaveLine candidates={list.items} saving={saving} last={last} onRetry={vote} dateOf={dateOf} t={t} />}
        >
          {list.items.map((candidate) => (
            <DateRow
              key={candidate.id}
              event={event}
              candidate={candidate}
              closed={closed}
              onAnswer={(answer) => {
                vote(candidate.id, answer);
              }}
              actions={
                manages ? (
                  <DateActions
                    date={dateOf(candidate)}
                    last={list.items.length === 1}
                    onAsk={(op) => {
                      onAsk({ op, candidate, date: dateOf(candidate) });
                    }}
                    t={t}
                  />
                ) : undefined
              }
            />
          ))}
        </CandidateTable>
      </Stack>
      {manages && <AddDate event={event} candidates={list.items} add={writes.addCandidate} say={say} t={t} />}
      <Stack gap="entry-line">
        <GroupHeading>{t("events.answered")}</GroupHeading>
        <Stack gap="entry-line">
          {list.items.map((candidate) => (
            <WhoAnswered key={candidate.id} event={event} candidate={candidate} t={t} />
          ))}
        </Stack>
      </Stack>
    </Stack>
  );
}
