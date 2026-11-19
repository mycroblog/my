// SPDX-License-Identifier: AGPL-3.0-only

// A report as a holder of `manage_reports` opens it, at /moderation/reports/{id} (FR-709–FR-712, FR-727, ADR-0058):
// under the account it reports, the way back to the queue and what the reader does with it (take it or leave it to
// nobody; resolve it, with whether an action was taken, or reopen it); then its facts (its state, its category, the
// rules it cites, when it was filed, who filed it, who handles it, whether it went to a remote account's server, what
// the moderators had about the account); the account, with the way to act on it on its own moderation screen; the
// reporter's comment; the posts it points at as the moderators see them, a deleted one with when it was deleted; the
// voice chat room or the event it points at, every change of its settings and who took part; the moderators' notes,
// and a new one; and, for a remote account's report filed here and not sent yet, forwarding it to its server with a
// text of the reader's alone, asked first, since it is sent once. The comment and each post translate on the reader's
// request only while the server may send a report's text to its providers (ADR-0148 §13), the comment as a private
// post. What a press did is said in the screen's status, which takes the focus where the press took its button away.

import {
  AccountLine,
  eventPath,
  languageName,
  moderatedAccountPath,
  Post,
  sentenceHandleOf,
  type PostAction,
  type PostTranslation,
  type TranslationFailure,
} from "@my/app-components";
import {
  reportForwardLimit,
  reportNoteLimit,
  reportNotForwardable,
  serverTranslates,
  TranslationRefusal,
  useInstance,
  useReport,
  useReportAccount,
  useReportActions,
  useReportTranslations,
  useViewer,
  type ReportCommentTranslation,
  type ReportDetail,
  type ReportedEvent,
  type ReportedStatus,
  type ReportedVoiceRoom,
  type ReportOutcome,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  CaptionLine,
  ConfirmDialog,
  FactList,
  FeedNotice,
  FormAlert,
  LogEntry,
  LogList,
  MenuGroup,
  MenuItem,
  MenuSurface,
  ModalPresence,
  Row,
  ScreenStack,
  ScreenTitle,
  Stack,
  TextButton,
  TextField,
  TextLink,
  type Fact,
} from "@my/design-system";
import { useId, useRef, useState, type ReactNode } from "react";
import { charsOf } from "../../chars";
import { inAnotherLanguage } from "../../post-translation";
import { handleIn, ReadNotice, Titled, useMoment, useStaffStrings, type T } from "../parts";
import { categoryWords, filedBy, queueHref } from "./queue";
import type { StaffSearch } from "../entries";

/** The language tag of the reader's locale that a translation goes into. */
function readerLanguage(locale: string): string {
  const l = new Intl.Locale(locale);
  return l.script === undefined ? l.language : `${l.language}-${l.script}`;
}

/** What the reader is told of a refused translation, by why the server refused. */
const failureOf = (e: unknown): TranslationFailure => {
  const why = e instanceof TranslationRefusal ? e.reason : "failed";
  const reason = why === "denied" ? "failed" : why;
  return { reason, retry: reason === "failed" ? "server" : false };
};

export type ReportScreenProps = {
  id: string;
  /** The query of the queue the reader came from, which the way back keeps. */
  search: StaffSearch;
  /** Goes back to the queue. */
  onBack: () => void;
};

/** The report of `id`, or why it shows none. */
export function ReportScreen({ id, search, onBack }: ReportScreenProps): ReactNode {
  const { t } = useStaffStrings();
  const read = useReport(id);
  if (read.missing)
    return (
      <FeedNotice alert lead action={<TextLink href={queueHref(search)}>{t("staff.report.missing.all")}</TextLink>}>
        {t("staff.report.missing")}
      </FeedNotice>
    );
  if (read.forbidden) return <FeedNotice lead>{t("staff.reports.forbidden")}</FeedNotice>;
  if (read.report === undefined)
    return (
      <ReadNotice
        status={read.status === "error" ? "error" : "pending"}
        loading={t("staff.report.loading")}
        failed={t("staff.report.failed")}
        retry={read.refetch}
      />
    );
  return <ReportView report={read.report} onBack={onBack} />;
}

/** The title of the screen of the report of `id`, which names the account it reports; undefined until it is read. */
export function useReportTitle(id: string): string | undefined {
  const { t } = useStaffStrings();
  const { report } = useReport(id);
  return report === undefined ? undefined : t("staff.report.title", { handle: handleIn(t, report.target_account) });
}

/** What a press on the report did, said in the screen's status. */
type Said = { readonly n: number; readonly words: string; readonly alert: boolean };

function ReportView({ report, onBack }: { report: ReportDetail; onBack: () => void }) {
  const { t } = useStaffStrings();
  const actions = useReportActions(report.id);
  const me = useViewer()?.id;
  const [busy, setBusy] = useState<string | undefined>(undefined);
  const [said, setSaid] = useState<Said | undefined>(undefined);
  const status = useRef<HTMLDivElement>(null);
  const target = handleIn(t, report.target_account);
  /** Runs a write of `kind`, then says what it did, or that it failed, and moves the focus to that. */
  const run = (kind: string, write: () => Promise<unknown>, done: string) => {
    if (busy !== undefined) return;
    setBusy(kind);
    void write().then(
      () => {
        setBusy(undefined);
        setSaid((s) => ({ n: (s?.n ?? 0) + 1, words: done, alert: false }));
        requestAnimationFrame(() => status.current?.focus());
      },
      () => {
        setBusy(undefined);
        setSaid((s) => ({ n: (s?.n ?? 0) + 1, words: t("staff.report.write_failed"), alert: true }));
        requestAnimationFrame(() => status.current?.focus());
      },
    );
  };
  const resolve = (outcome: ReportOutcome) => {
    run("resolve", () => actions.resolve(outcome), t("staff.report.done.resolved"));
  };
  const mine = report.assignee_account_id !== undefined && report.assignee_account_id === me;
  const head =
    report.state === "open" ? (
      <>
        {mine ? (
          <Button
            size="timeline"
            busy={busy === "unassign"}
            onPress={() => {
              run("unassign", actions.unassign, t("staff.report.done.unassigned"));
            }}
          >
            {t("staff.report.unassign")}
          </Button>
        ) : (
          <Button
            size="timeline"
            busy={busy === "assign"}
            onPress={() => {
              run("assign", actions.assignToSelf, t("staff.report.done.assigned"));
            }}
          >
            {t("staff.report.assign")}
          </Button>
        )}
        <MenuSurface
          label={t("staff.report.resolve.menu")}
          placement="bottom end"
          trigger={
            <Button kind="primary" size="timeline" busy={busy === "resolve"}>
              {t("staff.report.resolve")}
            </Button>
          }
        >
          <MenuGroup>
            <MenuItem
              id="actioned"
              icon="moderation"
              onAction={() => {
                resolve("actioned");
              }}
            >
              {t("staff.report.resolve.actioned")}
            </MenuItem>
            <MenuItem
              id="dismissed"
              icon="stateResolved"
              onAction={() => {
                resolve("dismissed");
              }}
            >
              {t("staff.report.resolve.dismissed")}
            </MenuItem>
          </MenuGroup>
        </MenuSurface>
      </>
    ) : (
      <Button
        size="timeline"
        busy={busy === "reopen"}
        onPress={() => {
          run("reopen", actions.reopen, t("staff.report.done.reopened"));
        }}
      >
        {t("staff.report.reopen")}
      </Button>
    );
  return (
    <ScreenStack>
      <ScreenTitle back={<BackButton onPress={onBack}>{t("staff.report.back")}</BackButton>} actions={head}>
        {target}
      </ScreenTitle>
      <div ref={status} role="status" tabIndex={-1}>
        {said !== undefined &&
          (said.alert ? (
            <FormAlert key={said.n} regular>
              {said.words}
            </FormAlert>
          ) : (
            <CaptionLine key={said.n} icon="accepted">
              {said.words}
            </CaptionLine>
          ))}
      </div>
      <ReportFacts report={report} me={me} />
      <Titled title={t("staff.report.account")}>
        {report.target_account === undefined ? (
          <FeedNotice start>{t("staff.account_gone")}</FeedNotice>
        ) : (
          <AccountLine account={report.target_account} />
        )}
        <Box text="body">
          <TextLink href={moderatedAccountPath(report.target_account_id)}>{t("staff.report.moderate")}</TextLink>
        </Box>
      </Titled>
      <Comment report={report} />
      <Posts report={report} />
      {report.voice_room !== undefined && <VoiceRoom room={report.voice_room} />}
      {report.event !== undefined && <EventTarget event={report.event} />}
      <Notes report={report} />
      {report.forwardable && (
        <Forward
          report={report}
          onForwarded={(domain) => {
            setSaid((s) => ({ n: (s?.n ?? 0) + 1, words: t("staff.report.done.forwarded", { domain }), alert: false }));
            requestAnimationFrame(() => status.current?.focus());
          }}
        />
      )}
    </ScreenStack>
  );
}

/** The domain of a remote account's server; undefined for a local one. */
const domainOf = (acct: string | undefined): string | undefined =>
  acct?.includes("@") === true ? acct.slice(acct.indexOf("@") + 1) : undefined;

/** The facts of the report. */
function ReportFacts({ report, me }: { report: ReportDetail; me: string | undefined }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const rules = useInstance()?.rules ?? [];
  const assignee = useReportAccount(report.assignee_account_id);
  const domain = domainOf(report.target_account?.acct);
  const state =
    report.state === "open"
      ? t("staff.report.state.open")
      : t("staff.report.resolved", { outcome: report.outcome ?? "dismissed", date: moment(report.state_changed_at) });
  const reporter =
    report.hook_origin !== undefined
      ? t("staff.report.reporter.review", { hook: report.hook_origin.hook })
      : report.uri !== undefined
        ? filedBy(t, report)
        : handleIn(t, report.reporter_account);
  const items: Fact[] = [
    { term: t("staff.report.fact.state"), value: state },
    { term: t("staff.report.fact.category"), value: categoryWords(t, report.category) },
    ...(report.rule_ids.length === 0
      ? []
      : [
          {
            term: t("staff.report.fact.rules"),
            value: (
              <Stack as="ul" gap="entry-line">
                {report.rule_ids.map((id) => (
                  <li key={id}>
                    <bdi>{rules.find((r) => r.id === id)?.text ?? t("staff.report.rule_retired")}</bdi>
                  </li>
                ))}
              </Stack>
            ),
          },
        ]),
    { term: t("staff.report.fact.filed"), value: moment(report.filed_at) },
    { term: t("staff.report.fact.reporter"), value: reporter },
    {
      term: t("staff.report.fact.assignee"),
      value:
        report.assignee_account_id === undefined
          ? t("staff.report.nobody")
          : report.assignee_account_id === me
            ? t("staff.report.you")
            : assignee === undefined
              ? t("staff.report.someone")
              : sentenceHandleOf(assignee),
    },
    ...(domain === undefined
      ? []
      : [
          {
            term: t("staff.report.fact.forwarded"),
            value: t(report.forwarded ? "staff.report.forwarded.yes" : "staff.report.forwarded.no", { domain }),
          },
        ]),
    {
      term: t("staff.report.fact.history"),
      value: t("staff.report.history", {
        reports: report.target_history.reports_count,
        actions: report.target_history.actions_count,
      }),
    },
  ];
  return <FactList label={t("staff.report.facts")} items={items} />;
}

/** The translation of a text of the report on the reader's request: its state, and Translate and Show original. */
type Translated<A> =
  | { kind: "none" }
  | { kind: "translating" }
  | { kind: "failed"; failure: TranslationFailure }
  | { kind: "shown" | "hidden"; answer: A };

/** The reporter's comment, and its translation where the server may send it. */
function Comment({ report }: { report: ReportDetail }) {
  const { t, locale } = useStaffStrings();
  const offer = useInstance()?.translation;
  const translations = useReportTranslations(report.id);
  const [state, setState] = useState<Translated<ReportCommentTranslation>>({ kind: "none" });
  const target = readerLanguage(locale);
  const offered = report.comment !== "" && offer?.reports_to_server === true && serverTranslates(offer, "private");
  const translate = () => {
    if (state.kind === "hidden") {
      setState({ kind: "shown", answer: state.answer });
      return;
    }
    setState({ kind: "translating" });
    translations.comment(target).then(
      (answer) => {
        setState({ kind: "shown", answer });
      },
      (e: unknown) => {
        setState({ kind: "failed", failure: failureOf(e) });
      },
    );
  };
  const shown = state.kind === "shown" ? state.answer : undefined;
  const languageOf = (tag: string) => languageName(tag, locale) ?? tag;
  return (
    <Titled title={t("staff.report.comment")}>
      {report.comment === "" ? (
        <FeedNotice start>{t("staff.report.comment.none")}</FeedNotice>
      ) : (
        <Box as="p" text="body" breaks="phrase" dir="auto" lang={shown?.language} keeps="lines">
          {shown?.comment ?? report.comment}
        </Box>
      )}
      {offered && (
        <CaptionLine
          icon="translate"
          busy={state.kind === "translating"}
          alert={state.kind === "failed"}
          action={
            state.kind === "translating" ? undefined : state.kind === "shown" ? (
              <TextButton
                onPress={() => {
                  setState({ kind: "hidden", answer: state.answer });
                }}
              >
                {t("staff.report.original")}
              </TextButton>
            ) : state.kind === "failed" && !state.failure.retry ? undefined : (
              <TextButton onPress={translate}>
                {state.kind === "failed" ? t("staff.retry") : t("staff.report.translate")}
              </TextButton>
            )
          }
        >
          {state.kind === "translating"
            ? t("staff.report.translating")
            : state.kind === "failed"
              ? t("staff.report.translate_failed", { reason: state.failure.reason })
              : shown === undefined
                ? undefined
                : shown.provider === undefined
                  ? t("staff.report.translated_unnamed", { language: languageOf(shown.source_language) })
                  : t("staff.report.translated", {
                      language: languageOf(shown.source_language),
                      provider: shown.provider.name,
                    })}
        </CaptionLine>
      )}
    </Titled>
  );
}

/** A post the report points at, translated on the reader's request where the server may send it. */
function ReviewedPost({ reportId, reported }: { reportId: string; reported: ReportedStatus }) {
  const { t, locale } = useStaffStrings();
  const moment = useMoment();
  const offer = useInstance()?.translation;
  const translations = useReportTranslations(reportId);
  const { status } = reported;
  const target = readerLanguage(locale);
  const [state, setState] = useState<Translated<PostTranslation>>({ kind: "none" });
  const translatable =
    offer?.reports_to_server === true &&
    inAnotherLanguage(status.language, target) &&
    serverTranslates(offer, status.visibility);
  const onAction = (action: PostAction) => {
    if (action.type === "original" && state.kind === "shown") setState({ kind: "hidden", answer: state.answer });
    if (action.type !== "translate" || state.kind === "translating") return;
    if (state.kind === "hidden") {
      setState({ kind: "shown", answer: state.answer });
      return;
    }
    setState({ kind: "translating" });
    translations.status(status.id, target).then(
      (answer) => {
        setState({ kind: "shown", answer });
      },
      (e: unknown) => {
        setState({ kind: "failed", failure: failureOf(e) });
      },
    );
  };
  return (
    <Post
      status={status}
      review
      translatable={translatable}
      translation={state.kind === "shown" ? state.answer : undefined}
      translating={state.kind === "translating"}
      translationFailure={state.kind === "failed" ? state.failure : undefined}
      onAction={onAction}
      context={
        reported.deleted_at === undefined ? undefined : (
          <CaptionLine icon="delete">{t("staff.report.deleted", { date: moment(reported.deleted_at) })}</CaptionLine>
        )
      }
    />
  );
}

/** The posts the report points at, those the server no longer has said in a line. */
function Posts({ report }: { report: ReportDetail }) {
  const { t } = useStaffStrings();
  const gone = report.status_ids.length - report.statuses.length;
  if (report.status_ids.length === 0) return null;
  return (
    <Titled title={t("staff.report.posts", { count: report.status_ids.length })}>
      <Stack gap="feed-tight">
        {report.statuses.map((s) => (
          <ReviewedPost key={s.status.id} reportId={report.id} reported={s} />
        ))}
      </Stack>
      {gone > 0 && <FeedNotice start>{t("staff.report.posts.gone", { count: gone })}</FeedNotice>}
    </Titled>
  );
}

/** An account the report names, in a sentence; while it is read, or once it is gone, the words for that. */
function useHandle(id: string | undefined): string {
  const { t } = useStaffStrings();
  return handleIn(t, useReportAccount(id));
}

function Participation({ p }: { p: ReportedVoiceRoom["participations"][number] }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const who = useHandle(p.account_id);
  return (
    <LogEntry
      icon="room"
      date={p.joined_at}
      about={
        p.left_at === undefined
          ? t("staff.report.room.still")
          : t("staff.report.room.left", { reason: p.leave_reason ?? "left", date: moment(p.left_at) })
      }
    >
      {t("staff.report.room.took_part", { handle: who })}
    </LogEntry>
  );
}

function Ban({ ban }: { ban: ReportedVoiceRoom["bans"][number] }) {
  const { t } = useStaffStrings();
  const who = useHandle(ban.account_id);
  const by = useHandle(ban.banned_by_account_id);
  return (
    <LogEntry icon="block" date={ban.banned_at}>
      {t("staff.report.room.removed", { by, handle: who })}
    </LogEntry>
  );
}

/** A change of a room's or an event's settings: who made it, the settings it left as its facts. */
function Revision({
  by,
  at,
  reported,
  facts,
  label,
}: {
  by: string;
  at: string;
  reported: boolean;
  facts: readonly Fact[];
  label: string;
}) {
  const { t } = useStaffStrings();
  const who = useHandle(by);
  return (
    <LogEntry
      icon="edit"
      date={at}
      about={reported ? t("staff.report.revision.reported") : undefined}
      detail={<FactList label={label} items={facts} />}
    >
      {t("staff.report.revision.by", { handle: who })}
    </LogEntry>
  );
}

/** The voice chat room the report points at: its settings as reported, how it ended, its changes, who took part. */
function VoiceRoom({ room }: { room: ReportedVoiceRoom }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const seen = room.revisions.find((r) => r.id === room.reported_revision_id) ?? room.revisions.at(-1);
  const settings = (r: ReportedVoiceRoom["revisions"][number]): Fact[] => [
    { term: t("staff.report.room.name"), value: <bdi>{r.name}</bdi> },
    { term: t("staff.report.room.description"), value: <bdi>{r.description}</bdi> },
  ];
  const items: Fact[] = [
    ...(seen === undefined ? [] : settings(seen)),
    { term: t("staff.report.room.opened"), value: moment(room.opened_at) },
    ...(room.ended_at === undefined
      ? [{ term: t("staff.report.room.ended"), value: t("staff.report.room.open_now") }]
      : [
          {
            term: t("staff.report.room.ended"),
            value: t("staff.report.room.end", {
              reason: room.end_reason ?? "host",
              date: moment(room.ended_at),
            }),
          },
        ]),
    ...(room.end_note === undefined
      ? []
      : [{ term: t("staff.report.room.end_note"), value: <bdi>{room.end_note}</bdi> }]),
  ];
  return (
    <Titled title={t("staff.report.room")}>
      <Stack gap="entry-room">
        <FactList label={t("staff.report.room")} items={items} />
        <LogList label={t("staff.report.room.log")}>
          {[...room.bans].reverse().map((b) => (
            <Ban key={`${b.account_id}-${b.banned_at}`} ban={b} />
          ))}
          {[...room.participations].reverse().map((p) => (
            <Participation key={`${p.account_id}-${p.joined_at}`} p={p} />
          ))}
          {[...room.revisions].reverse().map((r) => (
            <Revision
              key={r.id}
              by={r.revised_by_account_id}
              at={r.revised_at}
              reported={r.id === room.reported_revision_id}
              facts={settings(r)}
              label={t("staff.report.room.settings")}
            />
          ))}
        </LogList>
      </Stack>
    </Titled>
  );
}

/** The places of an event's settings, one a line. */
function placesOf(t: T, places: ReportedEvent["revisions"][number]["places"]): ReactNode {
  if (places.length === 0) return t("staff.report.event.no_places");
  return (
    <Stack as="ul" gap="entry-line">
      {places.map((p, i) => (
        <li key={i}>
          <bdi>
            {p.kind === "voice"
              ? t("staff.report.event.voice_place")
              : [p.name, p.address, p.url].filter((x) => x !== undefined && x !== "").join(" — ")}
          </bdi>
        </li>
      ))}
    </Stack>
  );
}

/** The event the report points at: its settings as reported, its date and state, its changes. */
function EventTarget({ event }: { event: ReportedEvent }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const seen = event.revisions.find((r) => r.id === event.reported_revision_id) ?? event.revisions.at(-1);
  const settings = (r: ReportedEvent["revisions"][number]): Fact[] => [
    { term: t("staff.report.event.title"), value: <bdi>{r.title}</bdi> },
    { term: t("staff.report.event.description"), value: <bdi>{r.description}</bdi> },
    { term: t("staff.report.event.places"), value: placesOf(t, r.places) },
  ];
  const items: Fact[] = [
    ...(seen === undefined ? [] : settings(seen)),
    {
      term: t("staff.report.event.when"),
      value: event.starts_at === undefined ? t("staff.report.event.no_date") : moment(event.starts_at),
    },
    {
      term: t("staff.report.event.state"),
      value:
        event.cancellation === undefined
          ? t("staff.report.event.state_value", { state: event.state })
          : t("staff.report.event.cancelled", {
              reason: event.cancellation.reason,
              date: moment(event.cancellation.cancelled_at),
            }),
    },
  ];
  return (
    <Titled title={t("staff.report.event")}>
      <Stack gap="entry-room">
        <FactList label={t("staff.report.event")} items={items} />
        <Box text="body">
          <TextLink href={eventPath(event.id)}>{t("staff.report.event.open")}</TextLink>
        </Box>
        <LogList label={t("staff.report.event.log")}>
          {[...event.revisions].reverse().map((r) => (
            <Revision
              key={r.id}
              by={r.revised_by_account_id}
              at={r.revised_at}
              reported={r.id === event.reported_revision_id}
              facts={settings(r)}
              label={t("staff.report.event.settings")}
            />
          ))}
        </LogList>
      </Stack>
    </Titled>
  );
}

function Note({ note }: { note: ReportDetail["notes"][number] }) {
  const { t } = useStaffStrings();
  const by = useHandle(note.author_account_id);
  return (
    <LogEntry
      icon="note"
      date={note.created_at}
      about={
        note.author_account_id === undefined ? t("staff.report.note.cli") : t("staff.report.note.by", { handle: by })
      }
    >
      <bdi>{note.body}</bdi>
    </LogEntry>
  );
}

/** The moderators' notes, the newest first, and the form of a new one. */
function Notes({ report }: { report: ReportDetail }) {
  const { t } = useStaffStrings();
  const { addNote } = useReportActions(report.id);
  const id = useId();
  const [written, setWritten] = useState("");
  const [round, setRound] = useState(0);
  const [error, setError] = useState<string | undefined>(undefined);
  const [failed, setFailed] = useState(false);
  const [busy, setBusy] = useState(false);
  const send = () => {
    if (busy) return;
    if (written.trim() === "") {
      setError(t("staff.report.note.blank"));
      document.getElementById(id)?.focus();
      return;
    }
    setBusy(true);
    setFailed(false);
    addNote(written).then(
      () => {
        setBusy(false);
        setWritten("");
        setError(undefined);
        // A new field, empty, for the next note.
        setRound((n) => n + 1);
      },
      () => {
        setBusy(false);
        setFailed(true);
      },
    );
  };
  return (
    <Titled title={t("staff.report.notes")}>
      <Stack gap="entry-room">
        {report.notes.length === 0 ? (
          <FeedNotice start>{t("staff.report.notes.none")}</FeedNotice>
        ) : (
          <LogList label={t("staff.report.notes")}>
            {[...report.notes].reverse().map((n) => (
              <Note key={n.id} note={n} />
            ))}
          </LogList>
        )}
        <Stack
          gap="screen-field-gap"
          as={
            <form
              onSubmit={(e) => {
                e.preventDefault();
                send();
              }}
            />
          }
        >
          <TextField
            key={round}
            id={id}
            name="note"
            label={t("staff.report.note.field")}
            hint={error === undefined ? t("staff.report.note.hint") : undefined}
            error={error}
            multiline
            rows={3}
            maxLength={reportNoteLimit}
            onChange={(value) => {
              setWritten(value);
              if (value.trim() !== "") setError(undefined);
            }}
          />
          {failed && <FormAlert regular>{t("staff.report.note.failed")}</FormAlert>}
          <Row>
            <Button type="submit" busy={busy}>
              {t("staff.report.note.add")}
            </Button>
          </Row>
        </Stack>
      </Stack>
    </Titled>
  );
}

/** Forwarding a remote account's report to its server, with a text of the reader's alone, asked first. */
function Forward({ report, onForwarded }: { report: ReportDetail; onForwarded: (domain: string) => void }) {
  const { t } = useStaffStrings();
  const { forward } = useReportActions(report.id);
  const domain = domainOf(report.target_account?.acct) ?? "";
  const id = useId();
  const [text, setText] = useState("");
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  const [refusal, setRefusal] = useState<string | undefined>(undefined);
  const tooLong = charsOf(text) > reportForwardLimit;
  const send = () => {
    setBusy(true);
    forward(text.trim()).then(
      () => {
        setBusy(false);
        setAsking(false);
        onForwarded(domain);
      },
      (e: unknown) => {
        setBusy(false);
        setAsking(false);
        setRefusal(t(reportNotForwardable(e) ? "staff.report.forward.refused" : "staff.report.write_failed"));
      },
    );
  };
  return (
    <Titled title={t("staff.report.forward")}>
      <Box as="p" text="body" breaks="phrase">
        {t("staff.report.forward.about", { domain })}
      </Box>
      <TextField
        id={id}
        name="forward"
        label={t("staff.report.forward.field")}
        hint={tooLong ? undefined : t("staff.report.forward.hint")}
        error={tooLong ? t("staff.report.forward.too_long") : undefined}
        multiline
        rows={3}
        onChange={setText}
      />
      {refusal !== undefined && <FormAlert regular>{refusal}</FormAlert>}
      <Row>
        <Button
          isDisabled={tooLong}
          onPress={() => {
            setRefusal(undefined);
            setAsking(true);
          }}
        >
          {t("staff.report.forward.send")}
        </Button>
      </Row>
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            title={t("staff.report.forward.confirm", { domain })}
            cancel={t("staff.report.cancel")}
            confirm={t("staff.report.forward.send")}
            busy={busy}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={send}
          >
            {t("staff.report.forward.confirm.about")}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </Titled>
  );
}
