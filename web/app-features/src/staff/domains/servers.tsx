// SPDX-License-Identifier: AGPL-3.0-only

// The remote servers (ADR-0006 §3, ADR-0058 §1): their list at /moderation/instances, every server this one knows, or
// those with problems (deliveries failed to reach them, or they are marked unavailable), or the unavailable ones, in
// domain order, each with its software and its state, with Open a server, which opens any server the reader names by
// its domain, a handle on it or an address on it; and a server's page (…/{domain}): its software, when it was first
// seen, whether deliveries go to it, Mark available, Refetch posts, the ways to its domain actions and the audit log, the times it
// was marked unavailable or available, and the deliveries to it, newest first, each with its attempts, of one activity
// or one local account (which the audit log records, as the field says), a masked one's sender, activity and inbox
// shown on demand, also recorded.

import {
  remoteServerStates,
  useAvailabilityEvents,
  useDeliveries,
  useDeliveryAccount,
  useFindAccount,
  usePermissions,
  useRemoteServer,
  useRemoteServers,
  useRemoteServerWrites,
  type AvailabilityEvent,
  type Delivery,
  type DeliveryAttempt,
  type DeliveryReveal,
  type RemoteServer,
  type RemoteServerQuery,
  type RemoteServerSoftware,
  type RemoteServerState,
} from "@my/app-data";
import {
  Pager,
  BackButton,
  Box,
  Button,
  dateFormat,
  FactList,
  FeedNotice,
  FilterBar,
  FilterFacet,
  LogEntry,
  LogList,
  RecordTable,
  Row,
  ScreenStack,
  ScreenTitle,
  Stack,
  StateMark,
  TextButton,
  TextField,
  type Fact,
  type FilterValue,
  type RecordColumn,
  type RecordRow,
  useAddedFocus,
} from "@my/design-system";
import { useId, useState, type ReactNode, type RefCallback } from "react";
import { staffHref, type StaffAddress, type StaffSearch } from "../entries";
import { ListMore, ReadNotice, useMoment, useStaffStrings, type T, useSaid } from "../parts";
import {
  auditLogPath,
  cursorOf,
  domainActionsOf,
  filtered,
  GoButton,
  newDomainActionPath,
  useAddressPages,
  refusalWords,
  remoteServerPath,
  Section,
  tied,
  Way,
  Ways,
} from "./parts";

/**
 * The domain of a server the reader names: a domain, a handle on it (`@name@domain`), or an address on it; undefined
 * for what names none.
 */
export function serverDomainOf(named: string): string | undefined {
  const text = named.trim();
  const handle = /^@?[^@\s]+@([^@\s/]+)$/.exec(text)?.[1];
  const host = (() => {
    if (handle !== undefined) return handle;
    try {
      return new URL(/^[a-z][a-z0-9+.-]*:\/\//i.test(text) ? text : `https://${text}`).hostname;
    } catch {
      return undefined;
    }
  })();
  if (host === undefined || !host.includes(".") || /\s/.test(host)) return undefined;
  return host.toLowerCase().replace(/\.$/, "");
}

/** What the list's filter shows besides every server: those with problems, or the unavailable ones. */
const serverFilters = ["troubled", "unavailable"] as const;
type ServerFilter = (typeof serverFilters)[number];

/** The filter an address's query names (`state`); none, every known server. */
const serverFilterOf = (search: StaffSearch): ServerFilter | undefined =>
  serverFilters.find((f) => f === search["state"]);

/** What the list shows at an address's query: every known server, unless it names a filter. */
export function serverQueryOf(search: StaffSearch): RemoteServerQuery {
  const filter = serverFilterOf(search);
  return {
    ...(filter === "troubled" ? { troubled: true } : {}),
    state: remoteServerStates.find((s) => s === filter),
    cursor: cursorOf(search),
  };
}

/** The software a server reports, its name and its version, as it reports them. */
const softwareWords = (s: RemoteServerSoftware): string => (s.version === "" ? s.name : `${s.name} ${s.version}`);

/** The state of a server, marked: deliveries go to it, or only follows and their answers do. */
function ServerStateMark({ state }: { state: RemoteServerState }) {
  const { t } = useStaffStrings();
  return (
    <StateMark state={state === "available" ? "active" : "disabled"}>
      {t("staff_domains.server.state", { state })}
    </StateMark>
  );
}

type Column = "software" | "failures" | "state";

/** The list of remote servers: Open a server, the filter of their state, the page, and the pages. */
export function RemoteServersScreen(address: StaffAddress): ReactNode {
  const pages = useAddressPages(address);
  const { search, onSearch, go } = address;
  const { t } = useStaffStrings();
  const q = serverQueryOf(search);
  const filter = serverFilterOf(search);
  const page = useRemoteServers(q);
  const filters: FilterValue[] = [
    { id: "troubled", label: t("staff_domains.servers.state.troubled"), icon: "error" },
    { id: "unavailable", label: t("staff_domains.server.state", { state: "unavailable" }), icon: "stateDisabled" },
  ];
  const head = (
    <>
      <ScreenTitle>{t("staff_domains.entry.instances")}</ScreenTitle>
      <OpenServer
        onOpen={(domain) => {
          go(remoteServerPath(domain));
        }}
      />
      <FilterBar
        label={t("staff_domains.servers.filters")}
        applied={filter !== undefined}
        onClear={() => {
          onSearch({});
        }}
      >
        <FilterFacet
          label={t("staff_domains.servers.state")}
          any={t("staff_domains.servers.state.any")}
          values={filters}
          chosen={filter}
          onChoose={(state) => {
            onSearch(filtered(search, { state }));
          }}
        />
      </FilterBar>
    </>
  );
  if (page.forbidden) return <FeedNotice lead>{t("staff_domains.forbidden")}</FeedNotice>;
  if (page.status !== "success")
    return (
      <ScreenStack>
        {head}
        <ReadNotice
          status={page.status}
          loading={t("staff_domains.servers.loading")}
          failed={t("staff_domains.servers.failed")}
          retry={page.refetch}
        />
      </ScreenStack>
    );
  const columns: RecordColumn<Column>[] = [
    // "mastodon 4.3.2" says what it is: on a narrow table, without its column's name.
    { key: "software", label: t("staff_domains.servers.column.software"), bare: true },
    { key: "failures", label: t("staff_domains.servers.column.failures"), kind: "figures" },
    { key: "state", label: t("staff_domains.servers.column.state"), kind: "fit", beside: true },
  ];
  const rows = page.items.map((s): RecordRow<Column> => ({
    id: s.domain,
    head: s.domain,
    href: remoteServerPath(s.domain),
    // A server that reports no software, or none failed, leaves its cell empty: most known servers have no failure.
    cells: {
      software: s.software === undefined ? undefined : softwareWords(s.software),
      failures:
        s.failure_days === 0 ? undefined : tied(t("staff_domains.server.failure_days", { count: s.failure_days })),
      state: <ServerStateMark state={s.state} />,
    },
  }));
  return (
    <ScreenStack busy={page.stale}>
      {head}
      <RecordTable
        label={t("staff_domains.entry.instances")}
        head={{ label: t("staff_domains.servers.column.domain") }}
        columns={columns}
        rows={rows}
        busy={page.stale}
        empty={
          <FeedNotice>
            {t(filter === undefined ? "staff_domains.servers.empty" : "staff_domains.servers.empty.filtered")}
          </FeedNotice>
        }
        even
      />
      <Pager {...pages.pager(page.next)} />
    </ScreenStack>
  );
}

/** Open a server: its field, which takes a domain, a handle or an address, and Open. */
function OpenServer({ onOpen }: { onOpen: (domain: string) => void }) {
  const { t } = useStaffStrings();
  const id = useId();
  const [named, setNamed] = useState("");
  const [error, setError] = useState<string | undefined>(undefined);
  return (
    <Stack
      gap="screen-field-gap"
      as={
        <form
          onSubmit={(e) => {
            e.preventDefault();
            const domain = serverDomainOf(named);
            if (domain === undefined) {
              setError(t("staff_domains.servers.open.invalid"));
              document.getElementById(id)?.focus();
            } else onOpen(domain);
          }}
        />
      }
    >
      <TextField
        id={id}
        name="server"
        label={t("staff_domains.servers.open")}
        hint={t("staff_domains.servers.open.hint")}
        autoCapitalize="none"
        error={error}
        onChange={(value) => {
          setNamed(value);
          setError(undefined);
        }}
      />
      <Row>
        <Button type="submit" size="timeline">
          {t("staff_domains.servers.open.submit")}
        </Button>
      </Row>
    </Stack>
  );
}

export type RemoteServerScreenProps = StaffAddress & { domain: string; onBack: () => void };

/** The server of `domain` on its page; or why it shows none. */
export function RemoteServerScreen({ domain, onBack, ...address }: RemoteServerScreenProps): ReactNode {
  const { t } = useStaffStrings();
  const read = useRemoteServer(domain);
  if (read.forbidden) return <FeedNotice lead>{t("staff_domains.forbidden")}</FeedNotice>;
  const back = <BackButton onPress={onBack}>{t("staff_domains.entry.instances")}</BackButton>;
  if (read.value === undefined)
    return (
      <ScreenStack>
        <ScreenTitle back={back}>{domain}</ScreenTitle>
        <ReadNotice
          status={read.status === "error" ? "error" : "pending"}
          loading={t("staff_domains.server.loading")}
          failed={t("staff_domains.server.failed")}
          retry={read.refetch}
        />
      </ScreenStack>
    );
  return (
    <ScreenStack>
      <ScreenTitle back={back}>{domain}</ScreenTitle>
      <ServerHead server={read.value} />
      <Availability domain={domain} />
      <Deliveries domain={domain} address={address} />
    </ScreenStack>
  );
}

/**
 * Mark available, Refetch posts; the server's software, when it was first seen, and whether deliveries go to it; and
 * the ways to its actions and the log.
 */
function ServerHead({ server }: { server: RemoteServer }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const { locale } = useStaffStrings();
  // A failed day is a day of UTC, as the server counts them.
  const day = (d: string) => dateFormat(locale, "UTC", { dateStyle: "medium" }).format(new Date(`${d}T00:00:00Z`));
  const audits = usePermissions().includes("view_audit");
  const { markAvailable, refetchPosts } = useRemoteServerWrites(server.domain);
  const { say, status } = useSaid();
  const [busy, setBusy] = useState<"mark" | "refetch" | undefined>(undefined);
  const run = (kind: "mark" | "refetch", write: () => Promise<unknown>, done: string) => {
    if (busy !== undefined) return;
    setBusy(kind);
    write().then(
      () => {
        setBusy(undefined);
        say(done);
      },
      (e: unknown) => {
        setBusy(undefined);
        say(refusalWords(t, e), true);
      },
    );
  };
  const items: Fact[] = [
    {
      term: t("staff_domains.server.fact.software"),
      value: server.software === undefined ? t("staff_domains.server.software.none") : softwareWords(server.software),
    },
    ...(server.first_seen_at === undefined
      ? []
      : [{ term: t("staff_domains.server.fact.first_seen"), value: moment(server.first_seen_at) }]),
    { term: t("staff_domains.server.fact.state"), value: <ServerStateMark state={server.state} /> },
    ...(server.state_changed_at === undefined
      ? []
      : [{ term: t("staff_domains.server.fact.since"), value: moment(server.state_changed_at) }]),
    {
      term: t("staff_domains.server.fact.failures"),
      value: tied(t("staff_domains.server.failure_days", { count: server.failure_days })),
    },
    ...(server.last_failure_day === undefined
      ? []
      : [{ term: t("staff_domains.server.fact.last_failure"), value: day(server.last_failure_day) }]),
  ];
  return (
    <Stack gap="entry-room">
      <Row wrap gap="screen-tab-gap">
        {(server.state === "unavailable" || server.failure_days > 0) && (
          <Button
            size="timeline"
            busy={busy === "mark"}
            onPress={() => {
              run("mark", markAvailable, t("staff_domains.server.marked"));
            }}
          >
            {t("staff_domains.server.mark")}
          </Button>
        )}
        <Button
          size="timeline"
          busy={busy === "refetch"}
          onPress={() => {
            run("refetch", refetchPosts, t("staff_domains.server.refetching"));
          }}
        >
          {t("staff_domains.server.refetch")}
        </Button>
      </Row>
      {status}
      <FactList label={t("staff_domains.server.facts")} items={items} />
      <Ways>
        <Way href={domainActionsOf(server.domain)}>{t("staff_domains.server.actions")}</Way>
        <Way href={newDomainActionPath(server.domain)}>{t("staff_domains.actions.new")}</Way>
        {audits && (
          <Way href={staffHref(auditLogPath, { target_kind: "domain", target: server.domain })}>
            {t("staff_domains.to_audit")}
          </Way>
        )}
      </Ways>
    </Stack>
  );
}

/** An availability event, in words. */
const eventWords = (t: T, e: AvailabilityEvent): string =>
  t("staff_domains.availability", { kind: e.kind, reason: e.reason });

/** The times the server was marked unavailable or available again, newest first, grown under Show more. */
function Availability({ domain }: { domain: string }) {
  const { t } = useStaffStrings();
  const list = useAvailabilityEvents(domain);
  const added = useAddedFocus(list.items.length);
  return (
    <Section title={t("staff_domains.availability.title")}>
      {list.items.length === 0 && list.status !== "success" ? (
        <ReadNotice
          status={list.status}
          loading={t("staff_domains.availability.loading")}
          failed={t("staff_domains.availability.failed")}
          retry={list.refetch}
        />
      ) : list.items.length === 0 ? (
        <FeedNotice start>{t("staff_domains.availability.empty")}</FeedNotice>
      ) : (
        <Stack gap="entry-room">
          <LogList label={t("staff_domains.availability.title")}>
            {list.items.map((e, i) => (
              <LogEntry
                key={e.id}
                itemRef={added.item(i)}
                icon={e.kind === "marked_available" ? "play" : "pause"}
                date={e.occurred_at}
              >
                {eventWords(t, e)}
              </LogEntry>
            ))}
          </LogList>
          <ListMore list={list} added={added} />
        </Stack>
      )}
    </Section>
  );
}

/** What the deliveries show at an address's query. */
const deliveryQueryOf = (search: StaffSearch) => ({
  activity: search["activity"] === "" ? undefined : search["activity"],
  account: search["account"] === "" ? undefined : search["account"],
});

/** The deliveries to the server, newest first: their filters, and the deliveries, grown under Show more. */
function Deliveries({ domain, address }: { domain: string; address: StaffAddress }) {
  const { t } = useStaffStrings();
  const q = deliveryQueryOf(address.search);
  const list = useDeliveries(domain, q);
  const added = useAddedFocus(list.items.length);
  return (
    <Section title={t("staff_domains.deliveries")}>
      <Stack gap="entry-room">
        <DeliveryFilters address={address} />
        {list.refused ? (
          <FeedNotice start>{t("staff_domains.deliveries.not_local")}</FeedNotice>
        ) : list.items.length === 0 && list.status !== "success" ? (
          <ReadNotice
            status={list.status}
            loading={t("staff_domains.deliveries.loading")}
            failed={t("staff_domains.deliveries.failed")}
            retry={list.refetch}
          />
        ) : list.items.length === 0 ? (
          <FeedNotice start>
            {t(
              q.activity === undefined && q.account === undefined
                ? "staff_domains.deliveries.empty"
                : "staff_domains.deliveries.empty.filtered",
            )}
          </FeedNotice>
        ) : (
          <>
            <LogList label={t("staff_domains.deliveries")}>
              {list.items.map((d, i) => (
                <DeliveryEntry key={d.id} domain={domain} delivery={d} itemRef={added.item(i)} />
              ))}
            </LogList>
            <ListMore list={list} added={added} />
          </>
        )}
      </Stack>
    </Section>
  );
}

/**
 * The filters of the deliveries: an activity's ID, and a local account by its username, which the audit log records,
 * as the field says before it is sent; Show applies both, Clear takes them off.
 */
function DeliveryFilters({ address }: { address: StaffAddress }) {
  const { t } = useStaffStrings();
  const { search, onSearch } = address;
  const id = useId();
  const ids = { activity: `${id}-activity`, account: `${id}-account` };
  const find = useFindAccount();
  const chosen = useDeliveryAccount(search["account"]);
  const [activity, setActivity] = useState(search["activity"] ?? "");
  const [account, setAccount] = useState("");
  const [error, setError] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const applied = search["activity"] !== undefined || search["account"] !== undefined;
  const apply = (accountId: string | undefined) => {
    onSearch(
      filtered(search, {
        activity: activity.trim() === "" ? undefined : activity.trim(),
        account: accountId,
      }),
    );
  };
  return (
    <Stack
      key={`${search["activity"] ?? ""}/${search["account"] ?? ""}`}
      gap="screen-field-gap"
      as={
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (busy) return;
            setError(undefined);
            if (account.trim() === "") {
              apply(search["account"]);
              return;
            }
            setBusy(true);
            void find(account).then((found) => {
              setBusy(false);
              if (found === undefined) {
                setError(t("staff_domains.deliveries.account.unknown"));
                document.getElementById(ids.account)?.focus();
              } else apply(found.id);
            });
          }}
        />
      }
    >
      <TextField
        id={ids.activity}
        name="activity"
        label={t("staff_domains.deliveries.activity")}
        defaultValue={search["activity"]}
        autoCapitalize="none"
        onChange={setActivity}
      />
      <TextField
        id={ids.account}
        name="account"
        label={t("staff_domains.deliveries.account")}
        hint={
          chosen === undefined
            ? t("staff_domains.deliveries.account.hint")
            : t("staff_domains.deliveries.account.chosen", { handle: `@${chosen.acct}` })
        }
        autoCapitalize="none"
        error={error}
        onChange={setAccount}
      />
      <Row wrap align="center" gap="screen-tab-gap">
        <Button type="submit" size="timeline" busy={busy}>
          {t("staff_domains.deliveries.show")}
        </Button>
        {applied && (
          <TextButton
            onPress={() => {
              onSearch(filtered(search, { activity: undefined, account: undefined }));
            }}
          >
            {t("staff_domains.deliveries.clear")}
          </TextButton>
        )}
      </Row>
    </Stack>
  );
}

/** An attempt of a delivery, in words: what came of it, with the response's status and its scheme, or the error. */
function attemptWords(t: T, a: DeliveryAttempt): string {
  return t("staff_domains.attempt", {
    outcome: a.outcome,
    status: a.status_code ?? 0,
    scheme: a.signature_scheme ?? "none",
    error: a.error ?? "",
  });
}

/**
 * A delivery: what was sent, how its last attempt went, and under Details every attempt, the sender, the activity and
 * the inbox, a masked one's shown on demand (Show, which the audit log records).
 */
function DeliveryEntry({
  domain,
  delivery: d,
  itemRef,
}: {
  domain: string;
  delivery: Delivery;
  itemRef: RefCallback<HTMLElement> | undefined;
}) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const { reveal } = useRemoteServerWrites(domain);
  const [shown, setShown] = useState<DeliveryReveal | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const last = d.attempts.at(-1);
  const sender = shown?.sender ?? d.sender;
  const ask =
    d.mask !== undefined && shown === undefined
      ? {
          busy,
          onReveal: () => {
            setBusy(true);
            setFailed(false);
            reveal(d.id).then(
              (r) => {
                setBusy(false);
                setShown(r);
              },
              () => {
                setBusy(false);
                setFailed(true);
              },
            );
          },
        }
      : undefined;
  const items: Fact[] = [
    {
      term: t("staff_domains.delivery.sender"),
      ...(ask === undefined
        ? {
            value:
              sender === undefined
                ? t("staff_domains.none")
                : sender.username === undefined
                  ? t("staff_domains.account_gone")
                  : `@${sender.username}`,
          }
        : { reveal: ask }),
    },
    {
      term: t("staff_domains.delivery.activity"),
      code: true,
      ...(ask === undefined
        ? { value: shown?.activity_uri ?? d.activity_uri ?? t("staff_domains.none") }
        : { reveal: ask }),
    },
    {
      term: t("staff_domains.delivery.inbox"),
      code: true,
      ...(ask === undefined || d.shared_inbox
        ? { value: shown?.inbox_url ?? d.inbox_url ?? t("staff_domains.none") }
        : { reveal: ask }),
    },
    { term: t("staff_domains.delivery.recorded"), value: moment(d.activity_created_at) },
  ];
  return (
    <LogEntry
      itemRef={itemRef}
      icon={last?.outcome === "succeeded" ? "accepted" : last === undefined ? "time" : "error"}
      date={d.created_at}
      about={
        d.mask === undefined
          ? last === undefined
            ? t("staff_domains.delivery.waiting")
            : attemptWords(t, last)
          : t("staff_domains.delivery.masked", { mask: d.mask })
      }
      detail={
        <Stack gap="entry-room">
          {d.mask !== undefined && shown === undefined && (
            <Box as="p" text="caption" tone="muted">
              {t("staff_domains.delivery.reveal.audited")}
            </Box>
          )}
          <FactList label={t("staff_domains.delivery.facts")} items={items} />
          {failed && (
            <Box as="p" text="caption">
              {t("staff_domains.delivery.reveal.failed")}
            </Box>
          )}
          {d.attempts.length > 0 && (
            <LogList label={t("staff_domains.delivery.attempts")}>
              {[...d.attempts].reverse().map((a) => (
                <LogEntry key={a.id} icon={a.outcome === "succeeded" ? "accepted" : "error"} date={a.attempted_at}>
                  {attemptWords(t, a)}
                </LogEntry>
              ))}
            </LogList>
          )}
        </Stack>
      }
    >
      {t("staff_domains.delivery", { type: d.activity_type, shared: d.shared_inbox ? "yes" : "no" })}
    </LogEntry>
  );
}

/**
 * The way to the server of an account's handle, `acct`, for a reader with `manage_domains`: Open the account's server;
 * none for a local account.
 */
export function AccountServerButton({ acct }: { acct: string | undefined }): ReactNode {
  const { t } = useStaffStrings();
  const may = usePermissions().includes("manage_domains");
  const domain = acct?.includes("@") === true ? acct.slice(acct.indexOf("@") + 1) : undefined;
  if (domain === undefined || !may) return null;
  return (
    <Row>
      <GoButton href={remoteServerPath(domain)} icon="federated">
        {t("staff_domains.to_account_server")}
      </GoButton>
    </Row>
  );
}
