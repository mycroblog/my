// SPDX-License-Identifier: AGPL-3.0-only

// The actions on remote domains (FR-702, FR-716, ADR-0058 §1): their list at /moderation/domain-actions, newest first,
// those in effect or all, of one domain where the address names it, each its domain, its kind, when it ends and its
// state, with the ways to a new action, Mastodon's CSV, the allowlist and the block-list subscriptions; and an action's
// page (…/{id}), all it holds, Lift while it is in effect, and the ways to the server, the domain's other actions and
// what the audit log holds of it.

import {
  useDomainAction,
  useDomainActions,
  useDomainActionWrites,
  usePermissions,
  type DomainAction,
} from "@my/app-data";
import {
  BackButton,
  Button,
  FactList,
  FeedNotice,
  FilterBar,
  FilterToggle,
  LinkButton,
  RecordTable,
  Row,
  ScreenStack,
  ScreenTitle,
  TextLink,
  type Fact,
  type RecordColumn,
  type RecordRow,
} from "@my/design-system";
import { useEffect, useState, type ReactNode } from "react";
import { staffHref, type StaffAddress, type StaffSearch } from "../entries";
import { ReadNotice, useMoment, useStaffStrings, useSaid } from "../parts";
import {
  auditLogPath,
  cursorOf,
  domainActionPath,
  domainActionsOf,
  domainActionsPath,
  DomainActionStateMark,
  domainAllowsPath,
  domainImportPath,
  domainKindWords,
  domainListPath,
  domainListsPath,
  filtered,
  newDomainActionPath,
  Pages,
  refusalWords,
  remoteServerPath,
  useEnd,
  Way,
  Ways,
} from "./parts";

/** What the list shows at an address's query: the actions in effect, of every domain, unless it says otherwise. */
export function actionQueryOf(search: StaffSearch) {
  const domain = search["domain"]?.trim();
  return {
    domain: domain === undefined || domain === "" ? undefined : domain,
    active: search["active"] !== "all",
    cursor: cursorOf(search),
  };
}

type Column = "kind" | "ends" | "state";

/** How long the reader pauses in the domain's search before the list follows it. */
const searchPause = 400;

/** The list of domain actions: the ways on, its filters, the page, and the pages. */
export function DomainActionsScreen(address: StaffAddress): ReactNode {
  const { search, onSearch } = address;
  const { t } = useStaffStrings();
  const end = useEnd();
  const q = actionQueryOf(search);
  const page = useDomainActions(q);
  // The domain written is the field's until the reader pauses: only then does the address, and the list, follow it.
  const [written, setWritten] = useState(q.domain ?? "");
  const shown = q.domain ?? "";
  // An address that changed by another way (back, a link) is what the field holds from then on.
  const [followed, setFollowed] = useState(shown);
  if (followed !== shown) {
    setFollowed(shown);
    setWritten(shown);
  }
  useEffect(() => {
    if (written.trim() === shown) return;
    const later = setTimeout(() => {
      onSearch(filtered(search, { domain: written.trim() === "" ? undefined : written.trim() }));
    }, searchPause);
    return () => {
      clearTimeout(later);
    };
  }, [written, shown, search, onSearch]);
  const head = (
    <>
      <ScreenTitle
        actions={
          <Row wrap gap="screen-tab-gap">
            <LinkButton href={newDomainActionPath(q.domain)} size="timeline">
              {t("staff_domains.actions.new")}
            </LinkButton>
            <LinkButton href={domainImportPath} size="timeline">
              {t("staff_domains.actions.csv")}
            </LinkButton>
          </Row>
        }
      >
        {t("staff_domains.entry.domain_actions")}
      </ScreenTitle>
      <Ways>
        <Way href={domainAllowsPath}>{t("staff_domains.allows")}</Way>
        <Way href={domainListsPath}>{t("staff_domains.lists")}</Way>
      </Ways>
      <FilterBar
        label={t("staff_domains.actions.filters")}
        search={{
          label: t("staff_domains.actions.domain"),
          value: written,
          onChange: setWritten,
          clearLabel: t("staff_domains.actions.domain.clear"),
          error: page.invalid ? t("staff_domains.actions.domain.invalid") : undefined,
        }}
        applied={q.domain !== undefined || !q.active}
        onClear={() => {
          setWritten("");
          onSearch({});
        }}
      >
        <FilterToggle
          label={t("staff_domains.actions.all")}
          on={!q.active}
          onChange={(on) => {
            onSearch(filtered(search, { active: on ? "all" : undefined }));
          }}
        />
      </FilterBar>
    </>
  );
  if (page.forbidden) return <FeedNotice lead>{t("staff_domains.forbidden")}</FeedNotice>;
  if (page.invalid) return <ScreenStack>{head}</ScreenStack>;
  if (page.status !== "success")
    return (
      <ScreenStack>
        {head}
        <ReadNotice
          status={page.status}
          loading={t("staff_domains.actions.loading")}
          failed={t("staff_domains.actions.failed")}
          retry={page.refetch}
        />
      </ScreenStack>
    );
  const columns: RecordColumn<Column>[] = [
    { key: "kind", label: t("staff_domains.actions.column.kind"), bare: true },
    { key: "ends", label: t("staff_domains.actions.column.ends"), kind: "fit" },
    { key: "state", label: t("staff_domains.actions.column.state"), kind: "fit", beside: true },
  ];
  const rows = page.items.map((a): RecordRow<Column> => ({
    id: a.id,
    head: a.domain,
    caption: a.public_comment,
    href: domainActionPath(a.id),
    cells: {
      kind: domainKindWords(t, a.kind),
      ends: end(a.expires_at),
      state: <DomainActionStateMark action={a} />,
    },
  }));
  return (
    <ScreenStack busy={page.stale}>
      {head}
      <RecordTable
        label={t("staff_domains.actions.table")}
        head={{ label: t("staff_domains.actions.column.domain") }}
        columns={columns}
        rows={rows}
        busy={page.stale}
        empty={
          <FeedNotice>
            {t(
              q.domain === undefined && q.active
                ? "staff_domains.actions.empty"
                : "staff_domains.actions.empty.filtered",
            )}
          </FeedNotice>
        }
        even
      />
      <Pages address={address} next={page.next} />
    </ScreenStack>
  );
}

/** The title of a domain action's page, its domain; undefined until it is read. */
export function useDomainActionTitle(id: string): string | undefined {
  return useDomainAction(id).value?.domain;
}

/** The domain action of `id` on its page, with Lift while it is in effect; or why it shows none. */
export function DomainActionScreen({ id, onBack }: { id: string; onBack: () => void }): ReactNode {
  const { t } = useStaffStrings();
  const read = useDomainAction(id);
  if (read.missing)
    return (
      <FeedNotice alert lead action={<TextLink href={domainActionsPath}>{t("staff_domains.action.all")}</TextLink>}>
        {t("staff_domains.action.missing")}
      </FeedNotice>
    );
  if (read.forbidden) return <FeedNotice lead>{t("staff_domains.forbidden")}</FeedNotice>;
  if (read.value === undefined)
    return (
      <ReadNotice
        status={read.status === "error" ? "error" : "pending"}
        loading={t("staff_domains.action.loading")}
        failed={t("staff_domains.action.failed")}
        retry={read.refetch}
      />
    );
  return <ActionView action={read.value} onBack={onBack} />;
}

function ActionView({ action, onBack }: { action: DomainAction; onBack: () => void }) {
  const { t } = useStaffStrings();
  const { lift } = useDomainActionWrites();
  const audits = usePermissions().includes("view_audit");
  const { say, status } = useSaid();
  const [busy, setBusy] = useState(false);
  return (
    <ScreenStack>
      <ScreenTitle back={<BackButton onPress={onBack}>{t("staff_domains.actions.back")}</BackButton>}>
        {action.domain}
      </ScreenTitle>
      {action.state === "active" && (
        <Row>
          <Button
            size="timeline"
            busy={busy}
            onPress={() => {
              setBusy(true);
              lift(action.id).then(
                () => {
                  setBusy(false);
                  say(t("staff_domains.action.lifted"));
                },
                (e: unknown) => {
                  setBusy(false);
                  say(refusalWords(t, e), true);
                },
              );
            }}
          >
            {t("staff_domains.action.lift")}
          </Button>
        </Row>
      )}
      {status}
      <ActionFacts action={action} />
      <Ways>
        <Way href={remoteServerPath(action.domain)}>{t("staff_domains.to_server")}</Way>
        <Way href={domainActionsOf(action.domain)}>{t("staff_domains.action.others")}</Way>
        {audits && (
          <Way href={staffHref(auditLogPath, { target_kind: "domain", target: action.domain })}>
            {t("staff_domains.to_audit")}
          </Way>
        )}
      </Ways>
    </ScreenStack>
  );
}

/** All a domain action holds, as facts, with the way to the subscription that issued it. */
function ActionFacts({ action }: { action: DomainAction }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const end = useEnd();
  const items: Fact[] = [
    { term: t("staff_domains.action.fact.state"), value: <DomainActionStateMark action={action} /> },
    { term: t("staff_domains.action.fact.kind"), value: domainKindWords(t, action.kind) },
    { term: t("staff_domains.action.fact.issued"), value: moment(action.issued_at) },
    { term: t("staff_domains.action.fact.ends"), value: end(action.expires_at) },
    ...(action.lifted_at === undefined
      ? []
      : [{ term: t("staff_domains.action.fact.lifted"), value: moment(action.lifted_at) }]),
    {
      term: t("staff_domains.action.fact.public_comment"),
      value: action.public_comment === undefined ? t("staff_domains.none") : <bdi>{action.public_comment}</bdi>,
    },
    {
      term: t("staff_domains.action.fact.private_note"),
      value: action.private_note === undefined ? t("staff_domains.none") : <bdi>{action.private_note}</bdi>,
    },
    {
      term: t("staff_domains.action.fact.obfuscate"),
      value: t(action.obfuscate ? "staff_domains.action.obfuscated" : "staff_domains.action.not_obfuscated"),
    },
    ...(action.subscription_id === undefined
      ? []
      : [
          {
            term: t("staff_domains.action.fact.issued_by"),
            value: (
              <TextLink href={domainListPath(action.subscription_id)}>
                {t("staff_domains.action.subscription")}
              </TextLink>
            ),
          },
        ]),
  ];
  return <FactList label={t("staff_domains.action.facts")} items={items} />;
}
