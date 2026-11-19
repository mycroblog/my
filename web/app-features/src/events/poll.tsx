// SPDX-License-Identifier: AGPL-3.0-only

// The date poll of an event on its page (ADR-0161 §7, FR-948, FR-952): its candidate dates as a table, a date a row,
// how many can go, may and cannot, and the reader's own answer, which saves as it is pressed; under the table, each
// date whose answer failed, with its own Try again, and what came of the last one pressed; under it, who answered each date, read once the reader opens it. Until a date is confirmed there is no
// answer to the event and no calendar file: the poll is what the reader answers.

import { AccountLine, eventWhen, useEventWhen } from "@my/app-components";
import {
  eventRefusal,
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
  CandidateRow,
  CandidateTable,
  Disclosure,
  EventTime,
  FeedNotice,
  GroupHeading,
  RowItem,
  RowList,
  SaveState,
  Stack,
  useLocale,
  useTimeZone,
  type Answer,
} from "@my/design-system";
import { useState, type ReactNode } from "react";
import { MoreOf, Retry, type T } from "./parts";

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
}: {
  event: ServerEvent;
  candidate: EventCandidate;
  closed: boolean;
  onAnswer: (vote: Answer) => void;
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
    />
  );
}

/** Who answered a date, each way: read while it is open. */
function Ballots({ event, candidate, t }: { event: ServerEvent; candidate: EventCandidate; t: T }) {
  const list = useEventBallots(event.id, candidate.id);
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
  return (
    <Stack gap="entry-room">
      {votes.map((vote) =>
        of(vote).length === 0 ? null : (
          <Stack key={vote} gap="entry-line">
            <GroupHeading>{t("events.vote_group", { vote, count: of(vote).length })}</GroupHeading>
            <RowList label={t("events.vote_group", { vote, count: of(vote).length })}>
              {of(vote).map((b) => (
                <RowItem key={b.account.id}>
                  <AccountLine account={b.account} />
                </RowItem>
              ))}
            </RowList>
          </Stack>
        ),
      )}
      <MoreOf list={list} t={t} />
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

/** The poll of `event`: its table, what came of each answer to a date, and who answered each date. */
export function Poll({ event, writes, t }: { event: ServerEvent; writes: EventWrites; t: T }): ReactNode {
  const { locale } = useLocale();
  const timeZone = useTimeZone();
  const list = useEventCandidates(event.id);
  // What came of the answer to each date, so that a failed one is never hidden by a later one to another date.
  const [saving, setSaving] = useState<ReadonlyMap<string, Saving>>(new Map());
  const [last, setLast] = useState<string | null>(null);
  const closed = event.state === "cancelled";
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
            />
          ))}
        </CandidateTable>
      </Stack>
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
