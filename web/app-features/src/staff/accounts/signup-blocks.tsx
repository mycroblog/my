// SPDX-License-Identifier: AGPL-3.0-only

// The blocks on sign-ups (FR-719): their list at /moderation/signup-blocks, newest first, of one kind or every kind, in
// effect or all, each its value, what it matches, what it does, when it ends and its state; Add a block
// (/moderation/signup-blocks/new: an email domain, an email address or an IP range, refused or made to wait for
// approval, until lifted or for a time, with a comment); Add from a list (…/import), every value of a list with the
// same settings, which says what it added and each line it skipped; and a block's page (…/{id}), with its comment, the
// ban it was made from, and Lift. A ban of a local account blocks its email address too where its form says so.

import {
  signupBlockCommentLimit,
  signupBlockKinds,
  signupBlockSeverities,
  signupBlockValueLimit,
  refusedMember,
  useAccountAction,
  useSignupBlock,
  useSignupBlocks,
  useSignupBlockWrites,
  type SignupBlock,
  type SignupBlockImportResult,
  type SignupBlockKind,
  type SignupBlockQuery,
  type SignupBlockSeverity,
} from "@my/app-data";
import {
  Pager,
  BackButton,
  Button,
  FactList,
  FeedNotice,
  FilterBar,
  FilterFacet,
  FilterToggle,
  FormAlert,
  LinkButton,
  RecordTable,
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
} from "@my/design-system";
import { useId, useRef, useState, type ReactNode } from "react";
import { staffHref, type StaffAddress, type StaffSearch } from "../entries";
import { ReadNotice, Titled, useMoment, useStaffStrings, type T } from "../parts";
import { accountActionPath } from "./account";
import {
  cursorOf,
  filtered,
  LabelledSelect,
  lengthOptions,
  useAddressPages,
  refusalWords,
  secondsOf,
  useEnd,
  useSaid,
} from "./parts";

/** The address of the list of blocks. */
export const signupBlocksPath = "/moderation/signup-blocks";

/** The address of the block of `id`. */
export const signupBlockPath = (id: string): string => `${signupBlocksPath}/${encodeURIComponent(id)}`;

/** What the list shows at an address's query: every kind, those in effect alone, unless it says otherwise. */
export function blockQueryOf(search: StaffSearch): SignupBlockQuery {
  return {
    kind: signupBlockKinds.find((k) => k === search["kind"]),
    active: search["active"] !== "all",
    cursor: cursorOf(search),
  };
}

/** What a block matches, in words. */
const kindWordsOf = (t: T, kind: SignupBlockKind): string => t("staff_accounts.block_kind", { kind });
/** What a block does, in words. */
const severityWordsOf = (t: T, severity: SignupBlockSeverity): string =>
  t("staff_accounts.block_severity", { severity });

/** The value of a block as the list shows it: an email address's hash cut short, since only its hash is kept. */
const valueOf = (t: T, block: SignupBlock): string =>
  block.kind === "email_hash" ? t("staff_accounts.blocks.hash", { hash: block.value.slice(0, 12) }) : block.value;

type Column = "kind" | "severity" | "ends" | "state";

/** The list of blocks: its filters, the way to add some, the page, and the pages. */
export function SignupBlocksScreen(address: StaffAddress): ReactNode {
  const pages = useAddressPages(address);
  const { search, onSearch } = address;
  const { t } = useStaffStrings();
  const end = useEnd();
  const q = blockQueryOf(search);
  const page = useSignupBlocks(q);
  const kinds: FilterValue[] = signupBlockKinds.map((k) => ({
    id: k,
    label: kindWordsOf(t, k),
    icon: k === "ip_range" ? "federated" : "direct",
  }));
  const head = (
    <>
      <ScreenTitle
        actions={
          <Row wrap gap="screen-tab-gap">
            <LinkButton href={`${signupBlocksPath}/new`} size="timeline">
              {t("staff_accounts.blocks.add")}
            </LinkButton>
            <LinkButton href={`${signupBlocksPath}/import`} size="timeline">
              {t("staff_accounts.blocks.import")}
            </LinkButton>
          </Row>
        }
      >
        {t("staff_accounts.entry.signup_blocks")}
      </ScreenTitle>
      <FilterBar
        label={t("staff_accounts.blocks.filters")}
        applied={q.kind !== undefined || q.active === false}
        onClear={() => {
          onSearch({});
        }}
      >
        <FilterFacet
          label={t("staff_accounts.blocks.kind")}
          any={t("staff_accounts.blocks.kind.any")}
          values={kinds}
          chosen={q.kind}
          onChoose={(kind) => {
            onSearch(filtered(search, { kind }));
          }}
        />
        <FilterToggle
          label={t("staff_accounts.blocks.all")}
          on={q.active === false}
          onChange={(on) => {
            onSearch(filtered(search, { active: on ? "all" : undefined }));
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
          loading={t("staff_accounts.blocks.loading")}
          failed={t("staff_accounts.blocks.failed")}
          retry={page.refetch}
        />
      </ScreenStack>
    );
  const columns: RecordColumn<Column>[] = [
    { key: "kind", label: t("staff_accounts.blocks.column.kind"), bare: true },
    { key: "severity", label: t("staff_accounts.blocks.column.severity") },
    { key: "ends", label: t("staff_accounts.blocks.column.ends"), kind: "fit" },
    // A state wraps before the value is cut short: a value is what the reader compares.
    { key: "state", label: t("staff_accounts.blocks.column.state"), beside: true },
  ];
  const rows = page.items.map((b): RecordRow<Column> => ({
    id: b.id,
    head: valueOf(t, b),
    caption: b.comment,
    href: staffHref(signupBlockPath(b.id), {}),
    cells: {
      kind: kindWordsOf(t, b.kind),
      severity: severityWordsOf(t, b.severity),
      ends: end(b.expires_at),
      state: <StateMark state={b.state}>{t("staff_accounts.action_state", { state: b.state })}</StateMark>,
    },
  }));
  return (
    <ScreenStack busy={page.stale}>
      {head}
      <RecordTable
        label={t("staff_accounts.blocks.table")}
        head={{ label: t("staff_accounts.blocks.column.value") }}
        columns={columns}
        rows={rows}
        busy={page.stale}
        empty={
          <FeedNotice>
            {t(
              q.kind === undefined && q.active === true
                ? "staff_accounts.blocks.empty"
                : "staff_accounts.blocks.empty.filtered",
            )}
          </FeedNotice>
        }
      />
      <Pager {...pages.pager(page.next)} />
    </ScreenStack>
  );
}

/** What the kind, severity, length and comment of a new block or list hold. */
type Settings = { kind: SignupBlockKind; severity: SignupBlockSeverity; length: string; comment: string };

/** The choices a new block and a list share: what they match, what they do, how long, and the comment. */
function SettingsFields({
  settings,
  onChange,
  faults,
  ids,
  children,
}: {
  settings: Settings;
  onChange: (settings: Settings) => void;
  faults: Partial<Record<string, string>>;
  ids: { comment: string };
  /** The value, or the list, after what it matches. */
  children: ReactNode;
}) {
  const { t } = useStaffStrings();
  return (
    <>
      <LabelledSelect
        label={t("staff_accounts.blocks.kind")}
        options={signupBlockKinds.map((k) => ({ value: k, label: kindWordsOf(t, k) }))}
        value={settings.kind}
        onChange={(kind) => {
          onChange({ ...settings, kind: kind as SignupBlockKind });
        }}
      />
      {children}
      <LabelledSelect
        label={t("staff_accounts.blocks.severity")}
        options={signupBlockSeverities.map((s) => ({ value: s, label: severityWordsOf(t, s) }))}
        value={settings.severity}
        onChange={(severity) => {
          onChange({ ...settings, severity: severity as SignupBlockSeverity });
        }}
      />
      <LabelledSelect
        label={t("staff_accounts.blocks.length")}
        options={lengthOptions(t)}
        value={settings.length}
        onChange={(length) => {
          onChange({ ...settings, length });
        }}
        error={faults["expires_in"]}
      />
      <TextField
        id={ids.comment}
        name="comment"
        label={t("staff_accounts.blocks.comment")}
        multiline
        rows={2}
        maxLength={signupBlockCommentLimit}
        error={faults["comment"]}
        onChange={(comment) => {
          onChange({ ...settings, comment });
        }}
      />
    </>
  );
}

/**
 * The members of a refused request that a field of the forms says under it; one of the choices (`kind`,
 * `severity`) offers only what the server takes, so its refusal is said beside the button.
 */
const fielded: ReadonlySet<string> = new Set(["comment", "expires_in"]);

const start: Settings = { kind: "email_domain", severity: "reject", length: "0", comment: "" };

/** The request's settings of `settings`. */
const requestOf = (s: Settings) => {
  const seconds = secondsOf(s.length);
  return {
    kind: s.kind,
    severity: s.severity,
    ...(s.comment.trim() === "" ? {} : { comment: s.comment.trim() }),
    ...(seconds === undefined ? {} : { expires_in: seconds }),
  };
};

/** Add a block: its form, which opens the block's page once added. */
export function NewSignupBlockScreen({ onBack, onAdded }: { onBack: () => void; onAdded: (id: string) => void }) {
  const { t } = useStaffStrings();
  const { add } = useSignupBlockWrites();
  const valueId = useId();
  const commentId = useId();
  const [settings, setSettings] = useState<Settings>(start);
  const [value, setValue] = useState("");
  const [faults, setFaults] = useState<Partial<Record<string, string>>>({});
  const [alert, setAlert] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const submit = () => {
    if (busy) return;
    setFaults({});
    setAlert(undefined);
    if (value.trim() === "") {
      setFaults({ value: t("staff_accounts.blocks.value.blank", { kind: settings.kind }) });
      document.getElementById(valueId)?.focus();
      return;
    }
    setBusy(true);
    add({ ...requestOf(settings), value: value.trim() }).then(
      (block) => {
        setBusy(false);
        onAdded(block.id);
      },
      (e: unknown) => {
        setBusy(false);
        const member = refusedMember(e);
        if (member?.member === "value") {
          setFaults({ value: t("staff_accounts.blocks.value.invalid", { kind: settings.kind }) });
          document.getElementById(valueId)?.focus();
        } else if (member !== undefined && fielded.has(member.member)) {
          setFaults({ [member.member]: member.said });
          if (member.member === "comment") document.getElementById(commentId)?.focus();
        } else setAlert(refusalWords(t, e));
      },
    );
  };
  return (
    <ScreenStack>
      <ScreenTitle back={<BackButton onPress={onBack}>{t("staff_accounts.blocks.back")}</BackButton>}>
        {t("staff_accounts.blocks.add")}
      </ScreenTitle>
      <Stack
        gap="screen-field-gap"
        as={
          <form
            onSubmit={(e) => {
              e.preventDefault();
              submit();
            }}
          />
        }
      >
        <SettingsFields settings={settings} onChange={setSettings} faults={faults} ids={{ comment: commentId }}>
          <TextField
            id={valueId}
            name="value"
            label={t("staff_accounts.blocks.value", { kind: settings.kind })}
            hint={t("staff_accounts.blocks.value.hint", { kind: settings.kind })}
            maxLength={signupBlockValueLimit}
            error={faults["value"]}
            onChange={setValue}
          />
        </SettingsFields>
        {alert !== undefined && <FormAlert regular>{alert}</FormAlert>}
        <Row>
          <Button type="submit" kind="primary" busy={busy}>
            {t("staff_accounts.blocks.add.submit")}
          </Button>
        </Row>
      </Stack>
    </ScreenStack>
  );
}

/** Add from a list: the list and its settings, then what was added and each line skipped, in the form's place. */
export function ImportSignupBlocksScreen({ onBack }: { onBack: () => void }) {
  const { t } = useStaffStrings();
  const { importList } = useSignupBlockWrites();
  const textId = useId();
  const commentId = useId();
  const resultRef = useRef<HTMLHeadingElement>(null);
  const [settings, setSettings] = useState<Settings>(start);
  const [text, setText] = useState("");
  const [faults, setFaults] = useState<Partial<Record<string, string>>>({});
  const [alert, setAlert] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<SignupBlockImportResult | undefined>(undefined);
  const submit = () => {
    if (busy) return;
    setFaults({});
    setAlert(undefined);
    if (text.trim() === "") {
      setFaults({ text: t("staff_accounts.blocks.list.blank") });
      document.getElementById(textId)?.focus();
      return;
    }
    setBusy(true);
    importList({ ...requestOf(settings), text }).then(
      (answer) => {
        setBusy(false);
        setResult(answer);
        requestAnimationFrame(() => resultRef.current?.focus());
      },
      (e: unknown) => {
        setBusy(false);
        const member = refusedMember(e);
        if (member !== undefined && (fielded.has(member.member) || member.member === "text")) {
          setFaults({ [member.member]: member.said });
          document.getElementById(member.member === "comment" ? commentId : textId)?.focus();
        } else setAlert(refusalWords(t, e));
      },
    );
  };
  const back = <BackButton onPress={onBack}>{t("staff_accounts.blocks.back")}</BackButton>;
  if (result !== undefined)
    return (
      <ScreenStack>
        <ScreenTitle back={back} ref={resultRef}>
          {t("staff_accounts.blocks.imported", { count: result.added })}
        </ScreenTitle>
        <Titled title={t("staff_accounts.blocks.skipped", { count: result.skipped.length })}>
          <RecordTable
            label={t("staff_accounts.blocks.skipped", { count: result.skipped.length })}
            head={{ label: t("staff_accounts.blocks.column.line") }}
            columns={[
              { key: "value", label: t("staff_accounts.blocks.column.value") },
              { key: "reason", label: t("staff_accounts.blocks.column.reason"), kind: "fit", beside: true },
            ]}
            rows={result.skipped.map((s) => ({
              id: String(s.line),
              head: t("staff_accounts.blocks.line", { line: s.line }),
              cells: {
                value: <bdi>{s.value}</bdi>,
                reason: t("staff_accounts.blocks.skip_reason", { reason: s.reason }),
              },
            }))}
            empty={<FeedNotice start>{t("staff_accounts.blocks.skipped.none")}</FeedNotice>}
          />
        </Titled>
        <Row>
          <LinkButton href={signupBlocksPath}>{t("staff_accounts.blocks.to_list")}</LinkButton>
        </Row>
      </ScreenStack>
    );
  return (
    <ScreenStack>
      <ScreenTitle back={back}>{t("staff_accounts.blocks.import")}</ScreenTitle>
      <Stack
        gap="screen-field-gap"
        as={
          <form
            onSubmit={(e) => {
              e.preventDefault();
              submit();
            }}
          />
        }
      >
        <SettingsFields settings={settings} onChange={setSettings} faults={faults} ids={{ comment: commentId }}>
          <TextField
            id={textId}
            name="text"
            label={t("staff_accounts.blocks.list")}
            hint={t("staff_accounts.blocks.list.hint")}
            multiline
            rows={8}
            error={faults["text"]}
            onChange={setText}
          />
        </SettingsFields>
        {alert !== undefined && <FormAlert regular>{alert}</FormAlert>}
        <Row>
          <Button type="submit" kind="primary" busy={busy}>
            {t("staff_accounts.blocks.import.submit")}
          </Button>
        </Row>
      </Stack>
    </ScreenStack>
  );
}

/** The title of a block's page, its value; undefined until it is read. */
export function useSignupBlockTitle(id: string): string | undefined {
  const { t } = useStaffStrings();
  const block = useSignupBlock(id).value;
  return block === undefined ? undefined : valueOf(t, block);
}

/** The block of `id` on its page, with Lift while it is in effect; or why it shows none. */
export function SignupBlockScreen({ id, onBack }: { id: string; onBack: () => void }): ReactNode {
  const { t } = useStaffStrings();
  const read = useSignupBlock(id);
  const { lift } = useSignupBlockWrites();
  const { say, status } = useSaid();
  const [busy, setBusy] = useState(false);
  if (read.missing)
    return (
      <FeedNotice alert lead action={<TextLink href={signupBlocksPath}>{t("staff_accounts.block.all")}</TextLink>}>
        {t("staff_accounts.block.missing")}
      </FeedNotice>
    );
  if (read.forbidden) return <FeedNotice lead>{t("staff_accounts.forbidden")}</FeedNotice>;
  if (read.value === undefined)
    return (
      <ReadNotice
        status={read.status === "error" ? "error" : "pending"}
        loading={t("staff_accounts.block.loading")}
        failed={t("staff_accounts.block.failed")}
        retry={read.refetch}
      />
    );
  const block = read.value;
  return (
    <ScreenStack>
      <ScreenTitle back={<BackButton onPress={onBack}>{t("staff_accounts.blocks.back")}</BackButton>}>
        {valueOf(t, block)}
      </ScreenTitle>
      <BlockFacts block={block} />
      {status}
      {block.state === "active" && (
        <Row>
          <Button
            busy={busy}
            onPress={() => {
              setBusy(true);
              lift(block.id).then(
                () => {
                  setBusy(false);
                  say(t("staff_accounts.block.lifted"));
                },
                (e: unknown) => {
                  setBusy(false);
                  say(refusalWords(t, e), true);
                },
              );
            }}
          >
            {t("staff_accounts.block.lift")}
          </Button>
        </Row>
      )}
    </ScreenStack>
  );
}

/** All a block holds, as facts, with the way to the ban it was made from. */
function BlockFacts({ block }: { block: SignupBlock }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const end = useEnd();
  // The ban's account, which its page is under.
  const ban = useAccountAction(block.moderation_action_id).value;
  const items: Fact[] = [
    {
      term: t("staff_accounts.block.fact.state"),
      value: <StateMark state={block.state}>{t("staff_accounts.action_state", { state: block.state })}</StateMark>,
    },
    { term: t("staff_accounts.block.fact.kind"), value: kindWordsOf(t, block.kind) },
    { term: t("staff_accounts.block.fact.value"), value: block.value, code: true },
    { term: t("staff_accounts.block.fact.severity"), value: severityWordsOf(t, block.severity) },
    { term: t("staff_accounts.block.fact.created"), value: moment(block.created_at) },
    { term: t("staff_accounts.block.fact.ends"), value: end(block.expires_at) },
    ...(block.lifted_at === undefined
      ? []
      : [{ term: t("staff_accounts.block.fact.lifted"), value: moment(block.lifted_at) }]),
    ...(block.comment === undefined
      ? []
      : [{ term: t("staff_accounts.block.fact.comment"), value: <bdi>{block.comment}</bdi> }]),
    ...(block.moderation_action_id === undefined
      ? []
      : [
          {
            term: t("staff_accounts.block.fact.ban"),
            value:
              ban === undefined ? (
                t("staff_accounts.block.from_ban")
              ) : (
                <TextLink href={accountActionPath(ban.account_id, ban.id)}>
                  {t("staff_accounts.block.from_ban")}
                </TextLink>
              ),
          },
        ]),
  ];
  return <FactList label={t("staff_accounts.block.facts")} items={items} />;
}
