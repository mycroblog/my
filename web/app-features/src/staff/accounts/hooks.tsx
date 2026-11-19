// SPDX-License-Identifier: AGPL-3.0-only

// The automated reviews' records (ADR-0147, ADR-0058), for a holder of `manage_reports`: at /moderation/hook-decisions,
// how many decisions each hook with a plugin made in the last 7 days, then the decisions, newest first, filtered by
// hook, mode and outcome (without an outcome, those a hook stopped, would have stopped, or could not check), each its
// hook, its outcome, its mode, when, and whether it was cancelled; a decision at …/{id}, with all it holds and Cancel
// where it can be cancelled, asked first; and the authors' appeals of a post `local.status` refused, at …/appeals,
// pending first, and one at …/appeals/{id}, with the text the author tried to post and their comment when there is
// one, upheld or overturned, asked first.

import { moderatedAccountPath, sentenceHandleOf } from "@my/app-components";
import {
  hookAppealStates,
  hookDecisionModes,
  hookNames,
  hookOutcomes,
  useHookDecision,
  useHookDecisions,
  useHookSummary,
  useHookWrites,
  useModeratedHookAppeal,
  useModeratedHookAppeals,
  useStaffAccounts,
  type HookDecision,
  type HookDecisionQuery,
  type HookAppealState,
  type ModeratedHookAppeal,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  ConfirmDialog,
  FactList,
  FeedNotice,
  FigureTiles,
  FilterBar,
  FilterFacet,
  LinkButton,
  ModalPresence,
  RecordTable,
  RelativeTime,
  Row,
  ScreenStack,
  ScreenTitle,
  StateMark,
  TextLink,
  type Fact,
  type FilterValue,
  type RecordColumn,
  type RecordRow,
} from "@my/design-system";
import { useState, type ReactNode } from "react";
import { staffHref, type StaffAddress, type StaffSearch } from "../entries";
import { handleIn, ReadNotice, Titled, useMoment, useStaffStrings, type T } from "../parts";
import { AppealStateMark } from "./appeals";
import { cursorOf, filtered, Pages, refusalWords, useSaid } from "./parts";

/** The address of the decisions, and of their appeals. */
const decisionsPath = "/moderation/hook-decisions";
export const hookAppealsPath = `${decisionsPath}/appeals`;

/** The address of the decision of `id`. */
const decisionPath = (id: string): string => `${decisionsPath}/${encodeURIComponent(id)}`;

/** The address of the appeal of `id`, keeping the appeals' query for its way back. */
const hookAppealHref = (id: string, search: StaffSearch): string =>
  staffHref(`${hookAppealsPath}/${encodeURIComponent(id)}`, search);

/** What the list of decisions shows at an address's query. */
export function decisionQueryOf(search: StaffSearch): HookDecisionQuery {
  return {
    hook: hookNames.find((h) => h === search["hook"]),
    mode: hookDecisionModes.find((m) => m === search["mode"]),
    outcome: hookOutcomes.find((o) => o === search["outcome"]),
    cursor: cursorOf(search),
  };
}

/** A hook's name, in words. */
const hookWords = (t: T, hook: HookDecision["hook"]): string =>
  t("staff_accounts.hook", { hook: hook.replace(".", "_") });
/** An outcome, in words. */
const outcomeWords = (t: T, outcome: HookDecision["outcome"]): string => t("staff_accounts.hook_outcome", { outcome });
/** A mode, in words. */
const modeWords = (t: T, mode: HookDecision["mode"]): string => t("staff_accounts.hook_mode", { mode });

/** Whether a decision stands, or a moderator cancelled it. */
function DecisionStateMark({ decision }: { decision: HookDecision }) {
  const { t } = useStaffStrings();
  return decision.overridden_at === undefined ? (
    <StateMark state="active">{t("staff_accounts.decision.stands")}</StateMark>
  ) : (
    <StateMark state="lifted">{t("staff_accounts.decision.cancelled")}</StateMark>
  );
}

/** How many decisions each hook with a plugin made in the last 7 days; nothing while it loads or failed. */
function Summary() {
  const { t, locale } = useStaffStrings();
  const { value } = useHookSummary();
  if (value === undefined) return null;
  const configured = value.hooks.filter((h) => h.configured);
  if (configured.length === 0) return <FeedNotice start>{t("staff_accounts.decisions.no_hooks")}</FeedNotice>;
  const number = new Intl.NumberFormat(locale);
  return (
    <FigureTiles
      label={t("staff_accounts.decisions.summary")}
      items={configured.map((h) => ({
        figure: number.format(h.outcomes.reduce((n, o) => n + o.count, 0)),
        label: t("staff_accounts.decisions.summary.hook", { hook: hookWords(t, h.hook), mode: h.mode }),
        href: staffHref(decisionsPath, { hook: h.hook }),
      }))}
    />
  );
}

type Column = "outcome" | "mode" | "decided" | "state";

/** The decisions: the summary, the filters, the way to the appeals, the page, and the pages. */
export function HookDecisionsScreen(address: StaffAddress): ReactNode {
  const { search, onSearch } = address;
  const { t } = useStaffStrings();
  const q = decisionQueryOf(search);
  const page = useHookDecisions(q);
  const accounts = useStaffAccounts(page.items.map((d) => d.account_id));
  const facet = <K extends string>(values: readonly K[], words: (v: K) => string): FilterValue[] =>
    values.map((v) => ({ id: v, label: words(v), icon: "reviewFailing" }));
  const head = (
    <>
      <ScreenTitle
        actions={
          <LinkButton href={hookAppealsPath} size="timeline">
            {t("staff_accounts.hook_appeals.title")}
          </LinkButton>
        }
      >
        {t("staff_accounts.entry.hook_decisions")}
      </ScreenTitle>
      <Summary />
      <FilterBar
        label={t("staff_accounts.decisions.filters")}
        applied={q.hook !== undefined || q.mode !== undefined || q.outcome !== undefined}
        onClear={() => {
          onSearch({});
        }}
      >
        <FilterFacet
          label={t("staff_accounts.decisions.hook")}
          any={t("staff_accounts.decisions.hook.any")}
          values={facet(hookNames, (h) => hookWords(t, h))}
          chosen={q.hook}
          onChoose={(hook) => {
            onSearch(filtered(search, { hook }));
          }}
        />
        <FilterFacet
          label={t("staff_accounts.decisions.mode")}
          any={t("staff_accounts.decisions.mode.any")}
          values={facet(hookDecisionModes, (m) => modeWords(t, m))}
          chosen={q.mode}
          onChoose={(mode) => {
            onSearch(filtered(search, { mode }));
          }}
        />
        <FilterFacet
          label={t("staff_accounts.decisions.outcome")}
          any={t("staff_accounts.decisions.outcome.any")}
          values={facet(hookOutcomes, (o) => outcomeWords(t, o))}
          chosen={q.outcome}
          onChoose={(outcome) => {
            onSearch(filtered(search, { outcome }));
          }}
        />
      </FilterBar>
    </>
  );
  if (page.forbidden) return <FeedNotice lead>{t("staff_accounts.forbidden")}</FeedNotice>;
  if (page.status !== "success")
    return (
      <ScreenStack>
        {head}
        <ReadNotice
          status={page.status}
          loading={t("staff_accounts.decisions.loading")}
          failed={t("staff_accounts.decisions.failed")}
          retry={page.refetch}
        />
      </ScreenStack>
    );
  const columns: RecordColumn<Column>[] = [
    { key: "outcome", label: t("staff_accounts.decisions.column.outcome") },
    { key: "mode", label: t("staff_accounts.decisions.column.mode"), bare: true },
    { key: "decided", label: t("staff_accounts.decisions.column.decided"), kind: "fit", bare: true },
    { key: "state", label: t("staff_accounts.decisions.column.state"), kind: "fit", beside: true },
  ];
  const rows = page.items.map((d): RecordRow<Column> => ({
    id: d.id,
    head: hookWords(t, d.hook),
    caption: d.account_id === undefined ? undefined : handleIn(t, accounts.get(d.account_id)),
    href: decisionPath(d.id),
    cells: {
      outcome: outcomeWords(t, d.outcome),
      mode: modeWords(t, d.mode),
      decided: <RelativeTime date={d.decided_at} />,
      state: <DecisionStateMark decision={d} />,
    },
  }));
  return (
    <ScreenStack busy={page.stale}>
      {head}
      <RecordTable
        label={t("staff_accounts.decisions.table")}
        head={{ label: t("staff_accounts.decisions.column.hook") }}
        columns={columns}
        rows={rows}
        busy={page.stale}
        empty={
          <FeedNotice>
            {t(
              q.hook === undefined && q.mode === undefined && q.outcome === undefined
                ? "staff_accounts.decisions.empty"
                : "staff_accounts.decisions.empty.filtered",
            )}
          </FeedNotice>
        }
        even
      />
      <Pages address={address} next={page.next} />
    </ScreenStack>
  );
}

/** The title of a decision's page; undefined until it is read. */
export function useHookDecisionTitle(id: string): string | undefined {
  const { t } = useStaffStrings();
  const decision = useHookDecision(id).value;
  return decision === undefined
    ? undefined
    : t("staff_accounts.decision.title", {
        hook: hookWords(t, decision.hook),
        outcome: outcomeWords(t, decision.outcome),
      });
}

/** The decision of `id`, with Cancel where it can be; or why it shows none. */
export function HookDecisionScreen({ id, onBack }: { id: string; onBack: () => void }): ReactNode {
  const { t } = useStaffStrings();
  const read = useHookDecision(id);
  const title = useHookDecisionTitle(id);
  const { override } = useHookWrites();
  const { say, status } = useSaid();
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  if (read.missing)
    return (
      <FeedNotice alert lead action={<TextLink href={decisionsPath}>{t("staff_accounts.decision.all")}</TextLink>}>
        {t("staff_accounts.decision.missing")}
      </FeedNotice>
    );
  if (read.forbidden) return <FeedNotice lead>{t("staff_accounts.forbidden")}</FeedNotice>;
  if (read.value === undefined)
    return (
      <ReadNotice
        status={read.status === "error" ? "error" : "pending"}
        loading={t("staff_accounts.decision.loading")}
        failed={t("staff_accounts.decision.failed")}
        retry={read.refetch}
      />
    );
  const decision = read.value;
  const cancellable = decision.overridable && decision.overridden_at === undefined;
  return (
    <ScreenStack>
      <ScreenTitle back={<BackButton onPress={onBack}>{t("staff_accounts.decision.back")}</BackButton>}>
        {title ?? hookWords(t, decision.hook)}
      </ScreenTitle>
      <DecisionFacts decision={decision} />
      {status}
      {cancellable && (
        <Row>
          <Button
            onPress={() => {
              setAsking(true);
            }}
          >
            {t("staff_accounts.decision.cancel")}
          </Button>
        </Row>
      )}
      {decision.hook === "local.status" && decision.outcome === "reject" && (
        <Row>
          <TextLink href={hookAppealsPath}>{t("staff_accounts.decision.by_appeal")}</TextLink>
        </Row>
      )}
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            title={t("staff_accounts.decision.cancel.confirm")}
            cancel={t("staff_accounts.decision.keep")}
            confirm={t("staff_accounts.decision.cancel")}
            busy={busy}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={() => {
              setBusy(true);
              override(decision.id).then(
                () => {
                  setBusy(false);
                  setAsking(false);
                  say(t("staff_accounts.decision.cancelled_now"));
                },
                (e: unknown) => {
                  setBusy(false);
                  setAsking(false);
                  say(refusalWords(t, e), true);
                },
              );
            }}
          >
            {t("staff_accounts.decision.cancel.about", { outcome: decision.outcome })}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </ScreenStack>
  );
}

/** All a decision holds, as facts. */
function DecisionFacts({ decision }: { decision: HookDecision }) {
  const { t, locale } = useStaffStrings();
  const moment = useMoment();
  const account = useStaffAccounts([decision.account_id]).get(decision.account_id ?? "");
  const items: Fact[] = [
    { term: t("staff_accounts.decision.fact.state"), value: <DecisionStateMark decision={decision} /> },
    { term: t("staff_accounts.decision.fact.hook"), value: hookWords(t, decision.hook) },
    { term: t("staff_accounts.decision.fact.outcome"), value: outcomeWords(t, decision.outcome) },
    { term: t("staff_accounts.decision.fact.mode"), value: modeWords(t, decision.mode) },
    ...(decision.reason === undefined
      ? []
      : [
          {
            term: t("staff_accounts.decision.fact.reason"),
            value: t("staff_accounts.hook_reason", { reason: decision.reason }),
          },
        ]),
    ...(decision.note === undefined
      ? []
      : [{ term: t("staff_accounts.decision.fact.note"), value: <bdi>{decision.note}</bdi> }]),
    ...(decision.account_id === undefined
      ? []
      : [
          {
            term: t("staff_accounts.decision.fact.account"),
            value: (
              <TextLink href={moderatedAccountPath(decision.account_id)}>
                {account === undefined ? t("staff.account_gone") : sentenceHandleOf(account)}
              </TextLink>
            ),
          },
        ]),
    { term: t("staff_accounts.decision.fact.subject"), value: decision.subject_key, code: true },
    { term: t("staff_accounts.decision.fact.decided"), value: moment(decision.decided_at) },
    {
      term: t("staff_accounts.decision.fact.latency"),
      value: t("staff_accounts.decision.latency", { ms: new Intl.NumberFormat(locale).format(decision.latency_ms) }),
    },
    ...(decision.restorable_until === undefined
      ? []
      : [{ term: t("staff_accounts.decision.fact.restorable"), value: moment(decision.restorable_until) }]),
    ...(decision.overridden_at === undefined
      ? []
      : [{ term: t("staff_accounts.decision.fact.cancelled"), value: moment(decision.overridden_at) }]),
  ];
  return <FactList label={t("staff_accounts.decision.facts")} items={items} />;
}

/** The state of the appeals of refused posts the list shows at an address's query: pending unless it says otherwise. */
const hookAppealStateOf = (search: StaffSearch): HookAppealState | undefined =>
  search["state"] === "any" ? undefined : (hookAppealStates.find((s) => s === search["state"]) ?? "pending");

type AppealColumn = "submitted" | "state";

/** The appeals of refused posts: the filter of their state, the page, and the pages. */
export function HookAppealsScreen({ onBack, ...address }: StaffAddress & { onBack: () => void }): ReactNode {
  const { search, onSearch } = address;
  const { t } = useStaffStrings();
  const state = hookAppealStateOf(search);
  const page = useModeratedHookAppeals(state, cursorOf(search));
  const accounts = useStaffAccounts(page.items.map((a) => a.account_id));
  const head = (
    <>
      <ScreenTitle back={<BackButton onPress={onBack}>{t("staff_accounts.decision.back")}</BackButton>}>
        {t("staff_accounts.hook_appeals.title")}
      </ScreenTitle>
      <FilterBar
        label={t("staff_accounts.appeals.filters")}
        applied={state !== "pending"}
        onClear={() => {
          onSearch({});
        }}
      >
        <FilterFacet
          label={t("staff_accounts.appeals.state")}
          any={t("staff_accounts.appeals.state.any")}
          values={hookAppealStates.map((s) => ({
            id: s,
            label: t("staff_accounts.appeal_state", { state: s }),
            icon: s === "pending" ? "statePending" : "stateResolved",
          }))}
          chosen={state}
          onChoose={(s) => {
            onSearch(filtered(search, { state: s === undefined ? "any" : s === "pending" ? undefined : s }));
          }}
        />
      </FilterBar>
    </>
  );
  if (page.forbidden) return <FeedNotice lead>{t("staff_accounts.forbidden")}</FeedNotice>;
  if (page.status !== "success")
    return (
      <ScreenStack>
        {head}
        <ReadNotice
          status={page.status}
          loading={t("staff_accounts.hook_appeals.loading")}
          failed={t("staff_accounts.hook_appeals.failed")}
          retry={page.refetch}
        />
      </ScreenStack>
    );
  const columns: RecordColumn<AppealColumn>[] = [
    { key: "submitted", label: t("staff_accounts.appeals.column.submitted"), kind: "fit", bare: true },
    { key: "state", label: t("staff_accounts.appeals.column.state"), kind: "fit", beside: true },
  ];
  const rows = page.items.map((a): RecordRow<AppealColumn> => ({
    id: a.id,
    head: handleIn(t, accounts.get(a.account_id)),
    caption: a.status_text.slice(0, 80),
    href: hookAppealHref(a.id, search),
    cells: { submitted: <RelativeTime date={a.submitted_at} />, state: <AppealStateMark state={a.state} /> },
  }));
  return (
    <ScreenStack busy={page.stale}>
      {head}
      <RecordTable
        label={t("staff_accounts.hook_appeals.table")}
        head={{ label: t("staff_accounts.appeals.column.account") }}
        columns={columns}
        rows={rows}
        busy={page.stale}
        empty={
          <FeedNotice>
            {t(
              state === "pending"
                ? "staff_accounts.hook_appeals.empty.pending"
                : "staff_accounts.appeals.empty.filtered",
            )}
          </FeedNotice>
        }
      />
      <Pages address={address} next={page.next} />
    </ScreenStack>
  );
}

/** The title of an appeal of a refused post, naming its author; undefined until it is read. */
export function useHookAppealTitle(id: string): string | undefined {
  const { t } = useStaffStrings();
  const appeal = useModeratedHookAppeal(id).value;
  const account = useStaffAccounts([appeal?.account_id]).get(appeal?.account_id ?? "");
  return appeal === undefined ? undefined : t("staff_accounts.hook_appeal.title", { handle: handleIn(t, account) });
}

/** The appeal of a refused post of `id`, upheld or overturned while it is pending; or why it shows none. */
export function HookAppealScreen({ id, onBack }: { id: string; onBack: () => void }): ReactNode {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const read = useModeratedHookAppeal(id);
  const title = useHookAppealTitle(id);
  const { decide } = useHookWrites();
  const { say, status } = useSaid();
  const [asking, setAsking] = useState<"uphold" | "overturn" | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  if (read.missing)
    return (
      <FeedNotice alert lead action={<TextLink href={hookAppealsPath}>{t("staff_accounts.hook_appeal.all")}</TextLink>}>
        {t("staff_accounts.appeal.missing")}
      </FeedNotice>
    );
  if (read.forbidden) return <FeedNotice lead>{t("staff_accounts.forbidden")}</FeedNotice>;
  if (read.value === undefined)
    return (
      <ReadNotice
        status={read.status === "error" ? "error" : "pending"}
        loading={t("staff_accounts.appeal.loading")}
        failed={t("staff_accounts.appeal.failed")}
        retry={read.refetch}
      />
    );
  const appeal: ModeratedHookAppeal = read.value;
  const items: Fact[] = [
    { term: t("staff_accounts.appeal.fact.state"), value: <AppealStateMark state={appeal.state} /> },
    { term: t("staff_accounts.appeal.fact.submitted"), value: moment(appeal.submitted_at) },
    {
      term: t("staff_accounts.hook_appeal.fact.decision"),
      value: <TextLink href={decisionPath(appeal.decision.id)}>{outcomeWords(t, appeal.decision.outcome)}</TextLink>,
    },
    ...(appeal.decision.reason === undefined
      ? []
      : [
          {
            term: t("staff_accounts.decision.fact.reason"),
            value: t("staff_accounts.hook_reason", { reason: appeal.decision.reason }),
          },
        ]),
    ...(appeal.decided_at === undefined
      ? []
      : [{ term: t("staff_accounts.appeal.fact.decided"), value: moment(appeal.decided_at) }]),
  ];
  const send = () => {
    if (asking === undefined) return;
    const decision = asking;
    setBusy(true);
    decide(appeal.id, decision).then(
      () => {
        setBusy(false);
        setAsking(undefined);
        say(t(decision === "uphold" ? "staff_accounts.hook_appeal.upheld" : "staff_accounts.hook_appeal.overturned"));
      },
      (e: unknown) => {
        setBusy(false);
        setAsking(undefined);
        say(refusalWords(t, e), true);
      },
    );
  };
  return (
    <ScreenStack>
      <ScreenTitle back={<BackButton onPress={onBack}>{t("staff_accounts.hook_appeal.back")}</BackButton>}>
        {title ?? t("staff_accounts.hook_appeals.title")}
      </ScreenTitle>
      <FactList label={t("staff_accounts.appeal.facts")} items={items} />
      <Titled title={t("staff_accounts.hook_appeal.post")}>
        <Box as="p" text="body" breaks="phrase" dir="auto" keeps="lines">
          {appeal.status_text}
        </Box>
      </Titled>
      {appeal.comment !== undefined && (
        <Titled title={t("staff_accounts.hook_appeal.comment")}>
          <Box as="p" text="body" breaks="phrase" dir="auto" keeps="lines">
            {appeal.comment}
          </Box>
        </Titled>
      )}
      {status}
      {appeal.state === "pending" && (
        <Row wrap align="center" gap="screen-tab-gap">
          <Button
            onPress={() => {
              setAsking("uphold");
            }}
          >
            {t("staff_accounts.appeal.uphold")}
          </Button>
          <Button
            onPress={() => {
              setAsking("overturn");
            }}
          >
            {t("staff_accounts.appeal.overturn")}
          </Button>
        </Row>
      )}
      <ModalPresence>
        {asking !== undefined && (
          <ConfirmDialog
            title={t(
              asking === "uphold" ? "staff_accounts.appeal.uphold.confirm" : "staff_accounts.appeal.overturn.confirm",
            )}
            cancel={t("staff_accounts.cancel")}
            confirm={t(asking === "uphold" ? "staff_accounts.appeal.uphold" : "staff_accounts.appeal.overturn")}
            busy={busy}
            onCancel={() => {
              setAsking(undefined);
            }}
            onConfirm={send}
          >
            {t(
              asking === "uphold"
                ? "staff_accounts.hook_appeal.uphold.about"
                : "staff_accounts.hook_appeal.overturn.about",
            )}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </ScreenStack>
  );
}
