// SPDX-License-Identifier: AGPL-3.0-only

// The appeals of actions (FR-707, ADR-0058): their queue at /moderation/appeals, those pending by default, newest
// first, each its account, the action it appeals, when, and its state; and an appeal at /moderation/appeals/{id}, with
// what the user wrote, the action and the way to its page, and, while it is pending, the decision: the reason the user
// is told, then Uphold (the action stays) or Overturn (the action is lifted; overturning a ban also cancels the
// account's deletion), each asked first, since an appeal is decided once.

import { sentenceHandleOf } from "@my/app-components";
import {
  appealReasonLimit,
  appealStates,
  refusedMember,
  useDecideAppeal,
  useModeratedAppeal,
  useModeratedAppeals,
  useModeratedAccounts,
  type ModeratedAppeal,
  type ModeratedAppealState,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  ConfirmDialog,
  FactList,
  FeedNotice,
  FilterBar,
  FilterFacet,
  FormAlert,
  ModalPresence,
  RecordTable,
  RelativeTime,
  Row,
  ScreenStack,
  ScreenTitle,
  Stack,
  StateMark,
  TextField,
  TextLink,
  type Fact,
  type FilterValue,
  type RecordColumn,
  type RecordRow,
  type RecordState,
} from "@my/design-system";
import { useId, useState, type ReactNode } from "react";
import { staffEntryPath, staffHref, type StaffAddress, type StaffSearch } from "../entries";
import { handleIn, ReadNotice, Titled, useMoment, useStaffStrings } from "../parts";
import { accountActionPath } from "./account";
import { actionCategoryWords, cursorOf, filtered, kindWords, Pages, refusalWords, useSaid, Section } from "./parts";

/** The state of the appeals the queue shows at an address's query: pending unless it says otherwise. */
export function appealStateOf(search: StaffSearch): ModeratedAppealState | undefined {
  if (search["state"] === "any") return undefined;
  return appealStates.find((s) => s === search["state"]) ?? "pending";
}

/** The address of the appeal of `id`, which keeps the queue's query for its way back. */
export const appealHref = (id: string, search: StaffSearch): string =>
  staffHref(`${staffEntryPath("moderation", "appeals")}/${encodeURIComponent(id)}`, search);

/** How an appeal's state is marked: pending waits, a decided one is over. */
const appealMark = (state: ModeratedAppealState): RecordState => (state === "pending" ? "pending" : "resolved");

/** An appeal's state, marked. */
export function AppealStateMark({ state }: { state: ModeratedAppealState }) {
  const { t } = useStaffStrings();
  return <StateMark state={appealMark(state)}>{t("staff_accounts.appeal_state", { state })}</StateMark>;
}

type Column = "action" | "submitted" | "state";

/** The queue of appeals: the filter of their state, the page, and the pages. */
export function AppealQueueScreen(address: StaffAddress): ReactNode {
  const { search, onSearch } = address;
  const { t } = useStaffStrings();
  const state = appealStateOf(search);
  const page = useModeratedAppeals(state, cursorOf(search));
  const accounts = useModeratedAccounts(page.items.map((a) => a.account_id));
  const states: FilterValue[] = appealStates.map((s) => ({
    id: s,
    label: t("staff_accounts.appeal_state", { state: s }),
    icon: s === "pending" ? "statePending" : "stateResolved",
  }));
  const head = (
    <>
      <ScreenTitle>{t("staff_accounts.entry.appeals")}</ScreenTitle>
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
          values={states}
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
          loading={t("staff_accounts.appeals.loading")}
          failed={t("staff_accounts.appeals.failed")}
          retry={page.refetch}
        />
      </ScreenStack>
    );
  const columns: RecordColumn<Column>[] = [
    { key: "action", label: t("staff_accounts.appeals.column.action"), bare: true },
    { key: "submitted", label: t("staff_accounts.appeals.column.submitted"), kind: "fit", bare: true },
    { key: "state", label: t("staff_accounts.appeals.column.state"), kind: "fit", beside: true },
  ];
  const rows = page.items.map((a): RecordRow<Column> => ({
    id: a.id,
    head: handleIn(t, accounts.get(a.account_id)?.account),
    href: appealHref(a.id, search),
    cells: {
      action: kindWords(t, a.action.kind),
      submitted: <RelativeTime date={a.submitted_at} />,
      state: <AppealStateMark state={a.state} />,
    },
  }));
  return (
    <ScreenStack busy={page.stale}>
      {head}
      <RecordTable
        label={t("staff_accounts.appeals.table")}
        head={{ label: t("staff_accounts.appeals.column.account") }}
        columns={columns}
        rows={rows}
        busy={page.stale}
        empty={
          <FeedNotice>
            {t(state === "pending" ? "staff_accounts.appeals.empty.pending" : "staff_accounts.appeals.empty.filtered")}
          </FeedNotice>
        }
        even
      />
      <Pages address={address} next={page.next} />
    </ScreenStack>
  );
}

export type AppealScreenProps = {
  id: string;
  /** Goes back to the queue the reader came from. */
  onBack: () => void;
};

/** The title of an appeal's screen, naming who appealed; undefined until it is read. */
export function useAppealTitle(id: string): string | undefined {
  const { t } = useStaffStrings();
  const appeal = useModeratedAppeal(id).value;
  const account = useModeratedAccounts([appeal?.account_id]).get(appeal?.account_id ?? "")?.account;
  return appeal === undefined ? undefined : t("staff_accounts.appeal.title", { handle: handleIn(t, account) });
}

/** The appeal of `id`, or why it shows none. */
export function AppealScreen({ id, onBack }: AppealScreenProps): ReactNode {
  const { t } = useStaffStrings();
  const read = useModeratedAppeal(id);
  const title = useAppealTitle(id);
  const { say, status } = useSaid();
  if (read.missing)
    return (
      <FeedNotice
        alert
        lead
        action={<TextLink href={staffEntryPath("moderation", "appeals")}>{t("staff_accounts.appeal.all")}</TextLink>}
      >
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
  const appeal = read.value;
  return (
    <ScreenStack>
      <ScreenTitle back={<BackButton onPress={onBack}>{t("staff_accounts.appeal.back")}</BackButton>}>
        {title ?? t("staff_accounts.entry.appeals")}
      </ScreenTitle>
      <AppealFacts appeal={appeal} />
      <Titled title={t("staff_accounts.appeal.text")}>
        <Box as="p" text="body" breaks="phrase" dir="auto" keeps="lines">
          {appeal.text}
        </Box>
      </Titled>
      {status}
      {appeal.state === "pending" && <Decide appeal={appeal} say={say} />}
    </ScreenStack>
  );
}

/** The appeal's facts and those of the action it appeals. */
function AppealFacts({ appeal }: { appeal: ModeratedAppeal }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const people = useModeratedAccounts([appeal.account_id, appeal.decided_by_account_id]);
  const account = people.get(appeal.account_id)?.account;
  const decider = people.get(appeal.decided_by_account_id ?? "")?.account;
  const items: Fact[] = [
    { term: t("staff_accounts.appeal.fact.state"), value: <AppealStateMark state={appeal.state} /> },
    { term: t("staff_accounts.appeal.fact.submitted"), value: moment(appeal.submitted_at) },
    {
      term: t("staff_accounts.appeal.fact.action"),
      value: (
        <TextLink href={accountActionPath(appeal.account_id, appeal.action.id)}>
          {t("staff_accounts.appeal.action", {
            kind: kindWords(t, appeal.action.kind),
            handle: account === undefined ? t("staff.account_gone") : sentenceHandleOf(account),
          })}
        </TextLink>
      ),
    },
    { term: t("staff_accounts.appeal.fact.category"), value: actionCategoryWords(t, appeal.action.reason_category) },
    {
      term: t("staff_accounts.appeal.fact.public_reason"),
      value:
        appeal.action.public_reason === undefined ? t("staff_accounts.none") : <bdi>{appeal.action.public_reason}</bdi>,
    },
    { term: t("staff_accounts.appeal.fact.issued"), value: moment(appeal.action.issued_at) },
    ...(appeal.decided_at === undefined
      ? []
      : [
          { term: t("staff_accounts.appeal.fact.decided"), value: moment(appeal.decided_at) },
          {
            term: t("staff_accounts.appeal.fact.decider"),
            value:
              appeal.decided_by_account_id === undefined
                ? t("staff_accounts.action.issuer.cli")
                : decider === undefined
                  ? t("staff_accounts.action.issuer.someone")
                  : sentenceHandleOf(decider),
          },
        ]),
    ...(appeal.decision_reason === undefined
      ? []
      : [{ term: t("staff_accounts.appeal.fact.decision_reason"), value: <bdi>{appeal.decision_reason}</bdi> }]),
  ];
  return <FactList label={t("staff_accounts.appeal.facts")} items={items} />;
}

/** The decision of a pending appeal: the reason the user is told, then Uphold or Overturn, each asked first. */
function Decide({ appeal, say }: { appeal: ModeratedAppeal; say: (words: string, alert?: boolean) => void }) {
  const { t } = useStaffStrings();
  const decide = useDecideAppeal();
  const id = useId();
  const [reason, setReason] = useState("");
  const [asking, setAsking] = useState<"uphold" | "overturn" | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const [alert, setAlert] = useState<string | undefined>(undefined);
  const moment = useMoment();
  const ban = appeal.action.kind === "ban";
  // A ban deletes its account 30 days after it was taken, which upholding the original decision makes final.
  const deleted = moment(new Date(Date.parse(appeal.action.issued_at) + 30 * 86_400_000).toISOString());
  const handle = handleIn(t, useModeratedAccounts([appeal.account_id]).get(appeal.account_id)?.account);
  const send = () => {
    if (asking === undefined) return;
    const decision = asking;
    setBusy(true);
    decide(appeal.id, decision, reason.trim()).then(
      () => {
        setBusy(false);
        setAsking(undefined);
        say(t(decision === "uphold" ? "staff_accounts.appeal.upheld" : "staff_accounts.appeal.overturned"));
      },
      (e: unknown) => {
        setBusy(false);
        setAsking(undefined);
        const member = refusedMember(e);
        if (member?.member === "public_reason") {
          setError(member.said);
          document.getElementById(id)?.focus();
        } else setAlert(refusalWords(t, e));
      },
    );
  };
  return (
    <Section title={t("staff_accounts.appeal.decide")}>
      <Stack gap="screen-field-gap">
        {ban && (
          <Box as="p" text="body" weight="label">
            {t("staff_accounts.appeal.ban_final", { date: deleted })}
          </Box>
        )}
        <TextField
          id={id}
          name="public_reason"
          label={t("staff_accounts.appeal.reason")}
          multiline
          rows={3}
          maxLength={appealReasonLimit}
          error={error}
          onChange={(v) => {
            setReason(v);
            setError(undefined);
          }}
        />
        {alert !== undefined && <FormAlert regular>{alert}</FormAlert>}
        <Row wrap align="center" gap="screen-tab-gap">
          <Button
            onPress={() => {
              setAlert(undefined);
              setAsking("uphold");
            }}
          >
            {t("staff_accounts.appeal.uphold")}
          </Button>
          <Button
            onPress={() => {
              setAlert(undefined);
              setAsking("overturn");
            }}
          >
            {t("staff_accounts.appeal.overturn")}
          </Button>
        </Row>
      </Stack>
      <ModalPresence>
        {asking !== undefined && (
          <ConfirmDialog
            title={t(
              asking === "uphold" ? "staff_accounts.appeal.uphold.confirm" : "staff_accounts.appeal.overturn.confirm",
            )}
            cancel={t("staff_accounts.cancel")}
            confirm={t(asking === "uphold" ? "staff_accounts.appeal.uphold" : "staff_accounts.appeal.overturn")}
            busy={busy}
            phrase={asking === "uphold" && ban ? handle : undefined}
            onCancel={() => {
              setAsking(undefined);
            }}
            onConfirm={send}
          >
            {asking === "uphold" && ban
              ? t("staff_accounts.appeal.uphold.ban_about", { date: deleted })
              : t(
                  asking === "uphold"
                    ? "staff_accounts.appeal.uphold.about"
                    : ban
                      ? "staff_accounts.appeal.overturn.ban_about"
                      : "staff_accounts.appeal.overturn.about",
                )}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </Section>
  );
}
