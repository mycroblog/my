// SPDX-License-Identifier: AGPL-3.0-only
// The date poll of an event that has no date yet (ADR-0161 §7; web-client/screens/event.md): a table of the candidate
// dates, one row each, named by its dates in the viewer's time zone (the row header), with how many can go, maybe,
// and cannot (the column headers say which), and the user's own answer, saved at once. A screen reader reads each
// count with its date and its column. Everyone who sees the event reads who answered a candidate, since answering
// shows the user's name; the organizers also confirm a candidate (the event takes its date and the poll closes),
// withdraw one, and add one, typed as the wall clock of the event's time zone.
import {
  AccountItem,
  Button,
  CheckIcon,
  CloseIcon,
  Cluster,
  ConfirmDialog,
  Dialog,
  DialogTrigger,
  Fieldset,
  Hint,
  InlineError,
  Radio,
  RadioGroup,
  RegionError,
  Section,
  Spinner,
  Stack,
  StatusBadge,
  Table,
  TableBody,
  TableCell,
  TableColumnHeader,
  TableHead,
  TableRow,
  TableRowHeader,
  TextField,
  VisuallyHidden,
  WaitingIcon,
} from "@my/ui";
import { type ReactNode, type SyntheticEvent, useState } from "react";
import { useIntl } from "react-intl";
import { handleOf } from "../../../lib/handle";
import { useNow } from "../../../lib/clock";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { imageSources } from "../../../lib/images";
import { profilePath } from "../../../lib/paths";
import { type Event, type EventCandidate, eventLimits, type EventVote, eventVotes } from "../api/events";
import { zoneCity } from "../hooks/eventTime";
import { accountName } from "../hooks/names";
import {
  type CandidateFieldErrors,
  type EventPoll as EventPollState,
  useCandidateLabel,
  useEventBallots,
  useEventPoll,
  useEventPollOrganizing,
} from "../hooks/useEventPoll";

/** The words of each answer to a candidate: the choices of the user's answer and the badges of who answered. */
const voteLabels: Readonly<Record<EventVote, ClientMessageKey>> = {
  yes: "client:events.poll.vote.yes",
  maybe: "client:events.poll.vote.maybe",
  no: "client:events.poll.vote.no",
};

/** The icon and tone of each answer's badge, so that the answers differ by more than their color (G10). */
const voteIcons: Readonly<Record<EventVote, ReactNode>> = {
  yes: <CheckIcon />,
  maybe: <WaitingIcon />,
  no: <CloseIcon />,
};

const voteTones: Readonly<Record<EventVote, "success" | "neutral" | "danger">> = {
  yes: "success",
  maybe: "neutral",
  no: "danger",
};

/** The column header of each count. */
const countHeaders: Readonly<Record<EventVote, ClientMessageKey>> = {
  yes: "client:events.poll.count.yes",
  maybe: "client:events.poll.count.maybe",
  no: "client:events.poll.count.no",
};

const counts: Readonly<Record<EventVote, (candidate: EventCandidate) => number>> = {
  yes: (candidate) => candidate.yes_count,
  maybe: (candidate) => candidate.maybe_count,
  no: (candidate) => candidate.no_count,
};

export function EventPoll({ event, organizes }: { event: Event; organizes: boolean }) {
  const t = useT();
  const poll = useEventPoll(event);
  return (
    <Section title={t("client:events.poll.title")} description={t("client:events.poll.description")}>
      <PollBody event={event} poll={poll} organizes={organizes} />
    </Section>
  );
}

function PollBody({ event, poll, organizes }: { event: Event; poll: EventPollState; organizes: boolean }) {
  const t = useT();
  switch (poll.status) {
    case "loading":
      return <Spinner label={t("client:events.loading")} />;
    case "error":
      return (
        <RegionError
          heading={t("client:events.poll.failed")}
          reason={poll.reason}
          retryLabel={t("client:events.retry")}
          onRetry={poll.retry}
          isRetrying={poll.isRetrying}
        />
      );
    case "ready":
      return <CandidateTable event={event} poll={poll} organizes={organizes} />;
  }
}

type ReadyPoll = Extract<EventPollState, { status: "ready" }>;

function CandidateTable({ event, poll, organizes }: { event: Event; poll: ReadyPoll; organizes: boolean }) {
  const t = useT();
  const intl = useIntl();
  const label = useCandidateLabel();
  const organizing = useEventPollOrganizing(event);
  const now = useNow();
  return (
    <Stack gap="4">
      {poll.voteError !== undefined && (
        <InlineError alert size="inherit">
          {t("client:events.poll.vote_failed", { reason: poll.voteError })}
        </InlineError>
      )}
      {organizing.withdrawError !== undefined && (
        <InlineError alert size="inherit">
          {t("client:events.poll.withdraw_failed", { reason: organizing.withdrawError })}
        </InlineError>
      )}
      {poll.candidates.length === 0 ? (
        <Hint>{t("client:events.poll.empty")}</Hint>
      ) : (
        <Table caption={t("client:events.poll.caption")}>
          <TableHead>
            <TableColumnHeader>{t("client:events.poll.date")}</TableColumnHeader>
            {eventVotes.map((vote) => (
              <TableColumnHeader key={vote}>{t(countHeaders[vote])}</TableColumnHeader>
            ))}
            <TableColumnHeader>{t("client:events.poll.your_answer")}</TableColumnHeader>
            <TableColumnHeader>
              <VisuallyHidden>{t("client:events.poll.actions")}</VisuallyHidden>
            </TableColumnHeader>
          </TableHead>
          <TableBody>
            {poll.candidates.map((candidate) => {
              const date = label(candidate);
              return (
                <TableRow key={candidate.id}>
                  <TableRowHeader>
                    <time dateTime={candidate.starts_at}>{date}</time>
                  </TableRowHeader>
                  {eventVotes.map((vote) => (
                    <TableCell key={vote} numeric>
                      {intl.formatNumber(counts[vote](candidate))}
                    </TableCell>
                  ))}
                  <TableCell>
                    <RadioGroup
                      aria-label={t("client:events.poll.your_answer_of", { date })}
                      orientation="horizontal"
                      value={poll.voteOf(candidate) ?? null}
                      isDisabled={poll.votingId === candidate.id}
                      onChange={(value) => {
                        const picked = eventVotes.find((v) => v === value);
                        if (picked !== undefined) poll.vote(candidate, picked);
                      }}
                    >
                      {eventVotes.map((vote) => (
                        <Radio key={vote} value={vote}>
                          {t(voteLabels[vote])}
                        </Radio>
                      ))}
                    </RadioGroup>
                  </TableCell>
                  <TableCell>
                    <Cluster gap="2" align="center">
                      <BallotsButton event={event} candidate={candidate} date={date} />
                      {organizes && (
                        <>
                          <ConfirmCandidateButton
                            candidate={candidate}
                            date={date}
                            organizing={organizing}
                            started={Date.parse(candidate.starts_at) <= now}
                          />
                          <WithdrawCandidateButton
                            candidate={candidate}
                            date={date}
                            organizing={organizing}
                            last={poll.candidates.length === 1}
                          />
                        </>
                      )}
                    </Cluster>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}
      {organizes && poll.candidates.length < eventLimits.candidates && (
        <AddCandidate event={event} organizing={organizing} />
      )}
    </Stack>
  );
}

type Organizing = ReturnType<typeof useEventPollOrganizing>;

/** Who answered a candidate and how, in a dialog. */
function BallotsButton({ event, candidate, date }: { event: Event; candidate: EventCandidate; date: string }) {
  const t = useT();
  return (
    <DialogTrigger>
      <Button size="sm" variant="secondary" aria-label={t("client:events.poll.ballots_of", { date })}>
        {t("client:events.poll.ballots")}
      </Button>
      <Dialog title={t("client:events.poll.ballots_title", { date })} closeLabel={t("client:events.poll.close")}>
        <Ballots event={event} candidate={candidate} />
      </Dialog>
    </DialogTrigger>
  );
}

function Ballots({ event, candidate }: { event: Event; candidate: EventCandidate }) {
  const t = useT();
  const ballots = useEventBallots(event, candidate);
  switch (ballots.status) {
    case "loading":
      return <Spinner label={t("client:events.loading")} />;
    case "error":
      return (
        <RegionError
          heading={t("client:events.poll.ballots_failed")}
          reason={ballots.reason}
          retryLabel={t("client:events.retry")}
          onRetry={ballots.retry}
          isRetrying={ballots.isRetrying}
        />
      );
    case "ready":
      if (ballots.ballots.length === 0) return <Hint>{t("client:events.poll.ballots_empty")}</Hint>;
      return (
        <Stack gap="3">
          <Stack as="ul" gap="3" aria-label={t("client:events.poll.ballots")}>
            {ballots.ballots.map((ballot) => (
              <li key={ballot.account.id}>
                <AccountItem
                  name={accountName(ballot.account)}
                  handle={handleOf(ballot.account.acct)}
                  avatar={imageSources(ballot.account.avatar)}
                  avatarSeed={ballot.account.id}
                  href={profilePath(ballot.account.acct)}
                  trailingPlacement="below"
                  trailing={
                    <StatusBadge
                      tone={voteTones[ballot.vote]}
                      icon={voteIcons[ballot.vote]}
                      label={t(voteLabels[ballot.vote])}
                    />
                  }
                />
              </li>
            ))}
          </Stack>
          {ballots.hasMore && (
            <div>
              <Button onPress={ballots.loadMore} isPending={ballots.isLoadingMore}>
                {t("client:events.poll.ballots_more")}
              </Button>
            </div>
          )}
        </Stack>
      );
  }
}

type CandidateActionProps = { candidate: EventCandidate; date: string; organizing: Organizing };

/** Fixing the event's date to the candidate, after a confirmation that says who is told. */
function ConfirmCandidateButton({ candidate, date, organizing, started }: CandidateActionProps & { started: boolean }) {
  const t = useT();
  const [isOpen, setIsOpen] = useState(false);
  return (
    <>
      <Button
        size="sm"
        variant="primary"
        isDisabled={started}
        aria-label={t("client:events.poll.confirm_of", { date })}
        onPress={() => {
          organizing.resetConfirm();
          setIsOpen(true);
        }}
      >
        {t("client:events.poll.confirm")}
      </Button>
      <ConfirmDialog
        isOpen={isOpen}
        onOpenChange={setIsOpen}
        tone="primary"
        title={t("client:events.poll.confirm.title", { date })}
        description={t("client:events.poll.confirm.description")}
        confirmLabel={t("client:events.poll.confirm.confirm")}
        cancelLabel={t("client:events.poll.confirm.back")}
        onConfirm={(close) => {
          organizing.confirm(candidate, close);
        }}
        isPending={organizing.isConfirming}
        errorMessage={organizing.confirmError}
      />
    </>
  );
}

/** Withdrawing the candidate, with the answers to it, after a confirmation; the poll keeps at least one. */
function WithdrawCandidateButton({ candidate, date, organizing, last }: CandidateActionProps & { last: boolean }) {
  const t = useT();
  const [isOpen, setIsOpen] = useState(false);
  return (
    <>
      <Button
        size="sm"
        variant="subtle"
        isDisabled={last}
        isPending={organizing.withdrawingId === candidate.id}
        aria-label={t("client:events.poll.withdraw_of", { date })}
        onPress={() => {
          setIsOpen(true);
        }}
      >
        {t("client:events.poll.withdraw")}
      </Button>
      <ConfirmDialog
        isOpen={isOpen}
        onOpenChange={setIsOpen}
        title={t("client:events.poll.withdraw.title", { date })}
        description={t("client:events.poll.withdraw.description")}
        confirmLabel={t("client:events.poll.withdraw.confirm")}
        cancelLabel={t("client:events.poll.withdraw.back")}
        onConfirm={(close) => {
          close();
          organizing.withdraw(candidate);
        }}
      />
    </>
  );
}

/** Adding a candidate date, typed as the wall clock of the event's time zone. */
function AddCandidate({ event, organizing }: { event: Event; organizing: Organizing }) {
  const t = useT();
  const [startsAt, setStartsAt] = useState("");
  const [endsAt, setEndsAt] = useState("");
  const [errors, setErrors] = useState<CandidateFieldErrors>({});
  const zone = t("client:events.form.zone_note", { zone: zoneCity(event.time_zone) });
  const submit = (submitted: SyntheticEvent<HTMLFormElement>) => {
    submitted.preventDefault();
    const form = submitted.currentTarget;
    const found = organizing.add(startsAt, endsAt, () => {
      setStartsAt("");
      setEndsAt("");
    });
    setErrors(found);
    const first =
      found.starts_at !== undefined
        ? "candidate_starts_at"
        : found.ends_at !== undefined
          ? "candidate_ends_at"
          : undefined;
    if (first === undefined) return;
    const field = form.elements.namedItem(first);
    if (field instanceof HTMLElement) field.focus();
  };
  return (
    <form onSubmit={submit} noValidate>
      <Fieldset legend={t("client:events.poll.add_legend")}>
        <Cluster gap="4" align="start">
          <TextField
            type="datetime-local"
            label={t("client:events.form.starts_at")}
            description={zone}
            name="candidate_starts_at"
            value={startsAt}
            onChange={setStartsAt}
            isRequired
            {...(errors.starts_at !== undefined && { isInvalid: true, errorMessage: errors.starts_at })}
          />
          <TextField
            type="datetime-local"
            label={t("client:events.form.ends_at")}
            description={zone}
            name="candidate_ends_at"
            value={endsAt}
            onChange={setEndsAt}
            isRequired
            {...(errors.ends_at !== undefined && { isInvalid: true, errorMessage: errors.ends_at })}
          />
        </Cluster>
        {organizing.addError !== undefined && (
          <InlineError alert size="inherit">
            {t("client:events.poll.add_failed", { reason: organizing.addError })}
          </InlineError>
        )}
        <div>
          <Button type="submit" isPending={organizing.isAdding}>
            {t("client:events.poll.add")}
          </Button>
        </div>
      </Fieldset>
    </form>
  );
}
