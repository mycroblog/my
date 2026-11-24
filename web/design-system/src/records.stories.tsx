// SPDX-License-Identifier: AGPL-3.0-only

// The lists of records of the staff's screens (records.tsx), each in every state, and the screens they make together:
// the moderators' queue of reports, filtered, sorted and paged, its rows picked for one action; the administrators'
// accounts and domain actions; the operators' jobs, which grow in place; a list with nothing yet and one with nothing
// that matches; the states of a record; and a report open with its queue beside it from `wide`, alone with the way
// back under it. The accounts, servers and times are samples, formatted as the app would.

import { color, rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { Button, TextButton } from "./button";
import { CodeText } from "./code-text";
import { FeedNotice } from "./feed";
import { Box, Stack } from "./layout";
import { BackButton, ScreenTitle } from "./list";
import {
  FilterBar,
  FilterFacet,
  FilterToggle,
  RecordTable,
  StateMark,
  type FilterValue,
  type RecordColumn,
  type RecordRow,
  type RecordSort,
  type RecordState,
} from "./records";
import { Pager, ShowMore, useAddedFocus } from "./pager";
import { Frame, FrameAside, FrameDock, FrameHeading, FrameMain, FrameMark, FrameSide, NavItem, NavList } from "./shell";

const rem = (px: number) => `${px / rootPx}rem`;

/** The column of a screen: as wide as the app's main column at most, the frame's edge around it. */
function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
}

// ---- The moderators' queue of reports -------------------------------------------------------------------------------

type ReportColumn = "category" | "reports" | "received" | "state";

const reportColumns: readonly RecordColumn<ReportColumn>[] = [
  { key: "category", label: "Category" },
  { key: "reports", label: "Reports", kind: "figures", sorts: "descending" },
  { key: "received", label: "Received", kind: "fit", sorts: "descending" },
  { key: "state", label: "State", kind: "fit", beside: true },
];

type Report = {
  id: string;
  account: string;
  by: string;
  category: string;
  categoryId: string;
  reports: number;
  /** Minutes since it came, for sorting; `received`, as the app writes it. */
  age: number;
  received: string;
  state: RecordState;
  mine?: boolean;
};

const stateWords: Record<RecordState, string> = {
  open: "Open",
  assigned: "Assigned",
  resolved: "Resolved",
  active: "Active",
  limited: "Limited",
  suspended: "Suspended",
  permanent: "Permanent",
  expired: "Expired",
  lifted: "Lifted",
  pending: "Pending",
  ending: "Ends Apr 10",
  failed: "Failed",
  disabled: "Disabled",
  queued: "Queued",
  running: "Running",
  retrying: "Waiting to retry",
  muted: "Muted",
};

const reports: readonly Report[] = [
  {
    id: "1042",
    account: "@deals4u@promo.example",
    by: "Reported by @mika and 11 others · Report 1042",
    category: "Spam",
    categoryId: "spam",
    reports: 12,
    age: 3,
    received: "3m ago",
    state: "open",
  },
  {
    id: "1041",
    account: "@kuro@social.example",
    by: "Reported by @hana · Report 1041",
    category: "Harassment",
    categoryId: "harassment",
    reports: 2,
    age: 41,
    received: "41m ago",
    state: "assigned",
    mine: true,
  },
  {
    id: "1040",
    account: "@newsbot@feeds.example",
    by: "Forwarded by feeds.example · Report 1040",
    category: "Other",
    categoryId: "other",
    reports: 1,
    age: 130,
    received: "2h ago",
    state: "open",
  },
  {
    id: "1038",
    account: "@ren",
    by: "Reported by @sora · Report 1038",
    category: "Rule 3: Mark sensitive media",
    categoryId: "rules",
    reports: 3,
    age: 310,
    received: "5h ago",
    state: "assigned",
  },
  {
    id: "1035",
    account: "@aoi@pixel.example",
    by: "Reported by @yuki · Report 1035",
    category: "Illegal content",
    categoryId: "legal",
    reports: 1,
    age: 1500,
    received: "1d ago",
    state: "resolved",
  },
];

const reportRow = (r: Report): RecordRow<ReportColumn> => ({
  id: r.id,
  head: r.account,
  caption: r.by,
  cells: {
    category: r.category,
    reports: String(r.reports),
    received: r.received,
    state: <StateMark state={r.state}>{r.mine === true ? "Assigned to you" : stateWords[r.state]}</StateMark>,
  },
});

const sortReports = (rows: readonly Report[], sort: RecordSort<ReportColumn>) => {
  const sign = sort.direction === "ascending" ? 1 : -1;
  const key = (r: Report): number | string =>
    sort.by === "reports" ? r.reports : sort.by === "received" ? -r.age : sort.by === "head" ? r.account : r.id;
  return [...rows].sort((a, b) => (key(a) < key(b) ? -sign : key(a) > key(b) ? sign : 0));
};

const stateValues: readonly FilterValue[] = [
  { id: "open", label: "Open", icon: "stateOpen" },
  { id: "assigned", label: "Assigned", icon: "stateAssigned" },
  { id: "resolved", label: "Resolved", icon: "stateResolved" },
];

const categoryValues: readonly FilterValue[] = [
  { id: "spam", label: "Spam", icon: "report" },
  { id: "harassment", label: "Harassment", icon: "report" },
  { id: "legal", label: "Illegal content", icon: "report" },
  { id: "rules", label: "Server rules", icon: "report" },
  { id: "other", label: "Other", icon: "report" },
];

const domainValues: readonly FilterValue[] = [
  { id: "local", label: "This server", icon: "local" },
  { id: "remote", label: "Other servers", icon: "federated" },
];

type Filters = { search: string; state: string | undefined; category: string | undefined; mine: boolean };
const none: Filters = { search: "", state: undefined, category: undefined, mine: false };

const filtered = (all: readonly Report[], f: Filters) =>
  all.filter(
    (r) =>
      (f.state === undefined || r.state === f.state) &&
      (f.category === undefined || r.categoryId === f.category) &&
      (!f.mine || r.mine === true) &&
      (f.search === "" || r.account.includes(f.search) || r.by.includes(f.search)),
  );

const applied = (f: Filters) => f.search !== "" || f.state !== undefined || f.category !== undefined || f.mine;

/** The filters of the queue, as its screen holds them. */
function ReportFilters({ filters, set, count }: { filters: Filters; set: (f: Filters) => void; count: number }) {
  return (
    <FilterBar
      label="Filter reports"
      search={{
        label: "Search reports",
        value: filters.search,
        onChange: (search) => {
          set({ ...filters, search });
        },
        clearLabel: "Clear search",
      }}
      applied={applied(filters)}
      onClear={() => {
        set(none);
      }}
      summary={count === 1 ? "1 report" : `${String(count)} reports`}
    >
      <FilterFacet
        label="State"
        any="Any state"
        values={stateValues}
        chosen={filters.state}
        onChoose={(state) => {
          set({ ...filters, state });
        }}
      />
      <FilterFacet
        label="Category"
        any="Any category"
        values={categoryValues}
        chosen={filters.category}
        onChoose={(category) => {
          set({ ...filters, category });
        }}
      />
      <FilterFacet
        label="Origin"
        any="Any server"
        values={domainValues}
        chosen={undefined}
        onChoose={() => undefined}
      />
      <FilterToggle
        label="Assigned to me"
        on={filters.mine}
        onChange={(mine) => {
          set({ ...filters, mine });
        }}
      />
    </FilterBar>
  );
}

/**
 * The moderators' queue, as its screen composes the parts: the filters over it, the reports sorted by when they came,
 * latest first, a row picked for an action on all picked, a press on a report opening it, and the pages under it.
 */
function Queue({
  start = none,
  picked = [],
  pages = "middle",
}: {
  start?: Filters;
  picked?: readonly string[];
  /** Where the page shown stands among the queue's: between others, the first, or the only one. */
  pages?: "middle" | "first" | "only";
}) {
  const [filters, setFilters] = useState<Filters>(start);
  const [sort, setSort] = useState<RecordSort<ReportColumn>>({ by: "received", direction: "descending" });
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set(picked));
  const [open, setOpen] = useState<string | undefined>(undefined);
  const shown = sortReports(filtered(reports, filters), sort);
  return (
    <Column>
      <Stack gap="entry-room">
        <ReportFilters filters={filters} set={setFilters} count={shown.length} />
        <RecordTable
          label="Reports"
          head={{ label: "Account", sorts: "ascending" }}
          columns={reportColumns}
          rows={shown.map(reportRow)}
          sort={sort}
          onSort={setSort}
          current={open}
          onOpen={setOpen}
          selection={{
            selected,
            onChange: setSelected,
            actions: [
              { id: "assign", label: "Assign to me", onPress: () => undefined },
              { id: "resolve", label: "Resolve…", onPress: () => undefined },
            ],
          }}
          empty={
            <FeedNotice
              action={
                <TextButton
                  onPress={() => {
                    setFilters(none);
                  }}
                >
                  Clear filters
                </TextButton>
              }
            >
              No reports match these filters.
            </FeedNotice>
          }
        />
        {pages !== "only" && (
          <Pager
            {...(pages === "first" ? {} : { previous: { onPress: () => undefined } })}
            next={{ onPress: () => undefined }}
            position={pages === "first" ? 1 : 2}
          />
        )}
      </Stack>
    </Column>
  );
}

/** The queue's table alone, its rows picked as given, for the actions on all picked. */
function Picking({ picked }: { picked: readonly string[] }) {
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set(picked));
  return (
    <RecordTable
      label="Reports"
      head={{ label: "Account" }}
      columns={reportColumns}
      rows={reports.map(reportRow)}
      selection={{
        selected,
        onChange: setSelected,
        actions: [
          { id: "assign", label: "Assign to me", onPress: () => undefined },
          { id: "resolve", label: "Resolve…", onPress: () => undefined },
        ],
      }}
    />
  );
}

// ---- The administrators' lists --------------------------------------------------------------------------------------

type AccountColumn = "role" | "posts" | "joined" | "state";

const accountColumns: readonly RecordColumn<AccountColumn>[] = [
  { key: "role", label: "Role" },
  { key: "posts", label: "Posts", kind: "figures", sorts: "descending" },
  { key: "joined", label: "Joined", kind: "fit", sorts: "descending" },
  { key: "state", label: "State", kind: "fit", beside: true },
];

const accounts: readonly RecordRow<AccountColumn>[] = [
  {
    id: "a1",
    head: "@yuki",
    caption: "Yuki Tanaka",
    href: "#/admin/accounts/a1",
    cells: {
      role: "Admin",
      posts: "4,812",
      joined: "Mar 2, 2024",
      state: <StateMark state="active">Active</StateMark>,
    },
  },
  {
    id: "a2",
    head: "@sora",
    caption: "そら",
    href: "#/admin/accounts/a2",
    cells: {
      role: "Moderator",
      posts: "1,207",
      joined: "Jun 18, 2024",
      state: <StateMark state="active">Active</StateMark>,
    },
  },
  {
    id: "a3",
    head: "@kenji",
    caption: "Asked to join: “I run the city’s cycling club and want a place for our rides.”",
    href: "#/admin/accounts/a3",
    cells: { posts: "0", joined: "Today", state: <StateMark state="pending">Awaiting approval</StateMark> },
  },
  {
    id: "a4",
    head: "@oldtimer",
    caption: "Suspended for spam",
    href: "#/admin/accounts/a4",
    cells: { posts: "36", joined: "Jan 9, 2025", state: <StateMark state="disabled">Suspended</StateMark> },
  },
];

type DomainColumn = "action" | "accounts" | "since" | "state";

const domainColumns: readonly RecordColumn<DomainColumn>[] = [
  { key: "action", label: "Action" },
  { key: "accounts", label: "Accounts", kind: "figures" },
  { key: "since", label: "Since", kind: "fit", sorts: "descending" },
  { key: "state", label: "State", kind: "fit", beside: true },
];

const domains: readonly RecordRow<DomainColumn>[] = [
  {
    id: "d1",
    head: "promo.example",
    caption: "By @sora · Spam from new accounts",
    href: "#/moderation/domains/d1",
    cells: {
      action: "Suspend",
      accounts: "214",
      since: "Nov 12, 2026",
      state: <StateMark state="active">Active</StateMark>,
    },
  },
  {
    id: "d2",
    head: "loud.example",
    caption: "By @yuki · Until Dec 1, 2026",
    href: "#/moderation/domains/d2",
    cells: {
      action: "Limit",
      accounts: "38",
      since: "Nov 1, 2026",
      state: <StateMark state="active">Active</StateMark>,
    },
  },
  {
    id: "d3",
    head: "quiet-now.example",
    caption: "By @yuki · Their moderators answered",
    href: "#/moderation/domains/d3",
    cells: {
      action: "Limit",
      accounts: "12",
      since: "Aug 4, 2026",
      state: <StateMark state="lifted">Lifted</StateMark>,
    },
  },
  {
    id: "d4",
    head: "trial.example",
    caption: "By @sora · For 30 days",
    href: "#/moderation/domains/d4",
    cells: {
      action: "Reject media",
      accounts: "5",
      since: "Jun 30, 2026",
      state: <StateMark state="expired">Expired</StateMark>,
    },
  },
];

// ---- The operators' jobs --------------------------------------------------------------------------------------------

type JobColumn = "attempts" | "next" | "state";

const jobColumns: readonly RecordColumn<JobColumn>[] = [
  { key: "attempts", label: "Attempts", kind: "figures" },
  { key: "next", label: "Next try", kind: "fit" },
  { key: "state", label: "State", kind: "fit", beside: true },
];

const job = (id: number, head: string, caption: string, state: RecordState, attempts: string, next?: string) => ({
  id: `j${String(id)}`,
  head,
  caption,
  code: true,
  href: `#/admin/jobs/j${String(id)}`,
  cells: {
    attempts,
    next,
    state: <StateMark state={state}>{state === "active" ? "Running" : stateWords[state]}</StateMark>,
  },
});

const jobs: readonly RecordRow<JobColumn>[] = [
  job(1, "Deliver activity", "To mastodon.example · Create of a post", "active", "1"),
  job(2, "Deliver activity", "To down.example · Follow", "failed", "8", "in 2h"),
  job(3, "Fetch remote account", "@river@slow.example", "pending", "0", "in 1m"),
  job(4, "Refresh link preview", "news.example/2026/11/rain", "failed", "3", "in 20m"),
  job(5, "Import block list", "From a subscribed list", "pending", "0", "in 5m"),
];

// ---- The shell around a record open beside its list -----------------------------------------------------------------

// A stand-in for the brand's wordmark, in the text's color of a scheme.
const wordmark = (fill: string) =>
  `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 44 24" width="44" height="24"><text x="2" y="19" font-family="system-ui, sans-serif" font-size="21" font-style="italic" font-weight="800" fill="${fill}">my</text></svg>`)}`;

const navigation = [
  { href: "/", icon: "home", label: "Home" },
  { href: "/notifications", icon: "notifications", label: "Notifications" },
  { href: "/search", icon: "search", label: "Search" },
  { href: "/moderation", icon: "moderation", label: "Moderation", current: true, count: 3 },
  { href: "/admin", icon: "admin", label: "Administration" },
] as const;

const places = (
  <NavList>
    {navigation.map((p) => (
      <NavItem
        key={p.href}
        href={p.href}
        icon={p.icon}
        label={p.label}
        current={"current" in p}
        count={"count" in p ? p.count : undefined}
        countLabel={"count" in p ? `${String(p.count)} open reports` : undefined}
      />
    ))}
  </NavList>
);

/** A report open: its head under the way back to the queue, and what the report says. */
function ReportDetail() {
  return (
    <Stack gap="entry-room">
      <ScreenTitle
        back={<BackButton onPress={() => undefined}>Reports</BackButton>}
        actions={
          <>
            <Button kind="secondary" size="timeline" onPress={() => undefined}>
              Assign to me
            </Button>
            <Button kind="primary" size="timeline" onPress={() => undefined}>
              Resolve…
            </Button>
          </>
        }
      >
        @deals4u@promo.example
      </ScreenTitle>
      <Stack gap="entry-line">
        <StateMark state="open">Open</StateMark>
        <Box as="p" text="body" breaks="phrase">
          Twelve people reported this account for spam in the last hour: it replies to every post that mentions a sale
          with the same link.
        </Box>
        <Box as="p" text="caption" tone="muted">
          Reported by @mika and 11 others · Report 1042 · 3m ago
        </Box>
      </Stack>
    </Stack>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Records", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/**
 * The queue of reports, as its screen composes the parts: the search and the facets over it, how many it holds; the
 * reports, latest first, each its account over who reported it, its category, how many reports, when, and its state;
 * one picked, the actions on the picked enabled; and the pages under it. A phone stacks each report, its state at the
 * end of its account's line, a fact a line, the columns that sort in a row over them.
 */
export const ReportsScreen: Story = {
  render: () => <Queue picked={["1040"]} />,
};

/**
 * The queue with its columns of words and its heads sharing the room evenly (`even`), so that they stand in one place
 * whatever the rows hold, and its category and time bare on a phone: under each account, its category and when, muted,
 * with no column's name.
 */
export const EvenAndBare: Story = {
  render: () => (
    <Column>
      <RecordTable
        label="Reports"
        head={{ label: "Account" }}
        columns={reportColumns.map((c) => (c.key === "category" || c.key === "received" ? { ...c, bare: true } : c))}
        rows={reports.map(reportRow)}
        even
      />
    </Column>
  ),
};

/**
 * The queue filtered: reports of harassment assigned to the reader, the search's words, every facet applied and the
 * toggle on, and Clear filters at the facets' end.
 */
export const FiltersApplied: Story = {
  render: () => (
    <Queue start={{ search: "kuro", state: "assigned", category: "harassment", mine: true }} pages="only" />
  ),
};

/** The filters at rest: nothing applied, so no Clear filters; the facets each say what they filter by. */
export const FiltersAtRest: Story = {
  render: () => {
    function Screen() {
      const [filters, setFilters] = useState<Filters>(none);
      return (
        <Column>
          <ReportFilters filters={filters} set={setFilters} count={filtered(reports, filters).length} />
        </Column>
      );
    }
    return <Screen />;
  },
};

/**
 * Every report picked but one: select all shows a dash, how many are picked says so, the actions act on them. With
 * none picked, the actions wait, disabled, and the count says none.
 */
export const Selecting: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-room">
        <Picking picked={["1042", "1041", "1040", "1038"]} />
        <Picking picked={[]} />
      </Stack>
    </Column>
  ),
};

/**
 * The accounts of the server, for the administrators: sorted by when they joined, latest first, which the column's
 * name says by its glyph and to assistive technology; a row opens the account. One asks to join, one is suspended.
 */
export const Accounts: Story = {
  render: () => (
    <Column>
      <RecordTable
        label="Accounts"
        head={{ label: "Account" }}
        columns={accountColumns}
        rows={accounts}
        sort={{ by: "joined", direction: "descending" }}
        onSort={() => undefined}
      />
    </Column>
  ),
};

/** The actions on other servers, each in force, lifted or expired, the number of their accounts it reaches. */
export const DomainActions: Story = {
  render: () => (
    <Column>
      <RecordTable label="Domain actions" head={{ label: "Domain" }} columns={domainColumns} rows={domains} />
    </Column>
  ),
};

/** Subscriptions to block lists: each one's address under its title, in the code's face, breaking after a slash. */
export const CodeCaptions: Story = {
  render: () => (
    <RecordTable
      label="Block list subscriptions"
      head={{ label: "Subscription" }}
      columns={[{ key: "mode", label: "Mode", kind: "fit", beside: true }]}
      rows={[
        {
          id: "s1",
          head: "Neighbours' published blocks",
          caption: "https://moderation.example/api/v1/instance/domain_blocks",
          captionCode: true,
          href: "#/moderation/domain-actions/subscriptions/s1",
          cells: { mode: "Automatic" },
        },
        {
          id: "s2",
          head: "Shared block list",
          caption: "https://lists.example/blocklist.csv",
          captionCode: true,
          href: "#/moderation/domain-actions/subscriptions/s2",
          cells: { mode: "Drafts" },
        },
      ]}
    />
  ),
};

/**
 * The jobs of the server, for the operators: running, failed and waiting to run again, how many times each was
 * tried; the list grows in place under Show more, busy while the next jobs come, the table faint meanwhile.
 */
export const Jobs: Story = {
  render: () => {
    function Screen() {
      const [busy, setBusy] = useState(false);
      return (
        <Column>
          <Stack gap="entry-room">
            <RecordTable label="Jobs" head={{ label: "Job" }} columns={jobColumns} rows={jobs} busy={busy} />
            <ShowMore
              more
              busy={busy}
              onPress={() => {
                setBusy(true);
              }}
            />
          </Stack>
        </Column>
      );
    }
    return <Screen />;
  },
};

const longError =
  "content: robots.txt cannot be read now: https://share.deutschlandradio.de/dlf-audiothek-audio-teilen.html?audio_id=dira_B0241C8AD2EB11F1768EB883034C2FA0: content: no link card: robots.txt cannot be read, which disallows the URL: https://share.deutschlandradio.de/dlf-audiothek-audio-teilen.html?audio_id=dira_B0241C8AD2EB11F1768EB883034C2FA0 (gave up after 10 attempts)";

/**
 * Jobs that failed on a long error, its first line under each row of the list, muted, and the runs of one of them,
 * each run's error under it: the error wraps inside its row, whatever its length, and the table keeps to its column,
 * every column's name on one line and the job's type in full.
 */
export const LongErrors: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-room">
        <RecordTable
          label="Jobs"
          head={{ label: "Type" }}
          columns={[
            { key: "attempts", label: "Attempts", kind: "figures" },
            { key: "when", label: "Failed at", kind: "fit", bare: true },
            { key: "state", label: "State", kind: "fit", beside: true },
          ]}
          rows={["l1", "l2"].map((id, i) => ({
            id,
            head: i === 0 ? "content.fetch_link_card" : "federation.deliver",
            code: true,
            href: `#/admin/jobs/${id}`,
            detail: (
              <Box as="p" text="caption" tone="muted">
                <CodeText>{i === 0 ? longError : 'Get "https://discover.example/nodeinfo/2.1": timeout'}</CodeText>
              </Box>
            ),
            cells: {
              attempts: "10",
              when: "Nov 23, 8:29 AM",
              state: <StateMark state="failed">Failed</StateMark>,
            },
          }))}
        />
        <RecordTable
          label="Failed attempts"
          head={{ label: "Try" }}
          columns={[
            { key: "outcome", label: "Outcome", kind: "fit" },
            { key: "finished", label: "Finished at", kind: "fit" },
            { key: "worker", label: "Worker", kind: "fit" },
          ]}
          rows={[10, 9].map((n) => ({
            id: `a${String(n)}`,
            head: `#${String(n)}`,
            cells: {
              outcome: (
                <StateMark state={n === 10 ? "failed" : "retrying"}>{n === 10 ? "No retry" : "Retry"}</StateMark>
              ),
              finished: "Nov 23, 8:29 AM",
              worker: <CodeText>worker-7f9c</CodeText>,
            },
            detail: (
              <Box as="p" text="caption">
                <CodeText>{longError}</CodeText>
              </Box>
            ),
          }))}
        />
      </Stack>
    </Column>
  ),
};

/**
 * The jobs' table growing under Show more, four rows a press: the head of the first row each press added takes the
 * focus (`itemRef` with `useAddedFocus`), and Show more goes once the list is read to its end.
 */
export const GrowingRows: Story = {
  render: () => {
    function Screen() {
      const [shown, setShown] = useState(4);
      const added = useAddedFocus(shown);
      const rows = Array.from({ length: shown }, (_, i) => {
        const sample = jobs[i % jobs.length];
        return sample === undefined ? undefined : { ...sample, id: `${sample.id}-${String(i)}` };
      }).filter((row) => row !== undefined);
      return (
        <Column>
          <Stack gap="entry-room">
            <RecordTable label="Jobs" head={{ label: "Job" }} columns={jobColumns} rows={rows} itemRef={added.item} />
            <ShowMore
              more={shown < 12}
              onPress={added.press(() => {
                setShown((n) => n + 4);
              })}
            />
          </Stack>
        </Column>
      );
    }
    return <Screen />;
  },
};

/**
 * A list with nothing to show, in the table's place: nothing yet, which says what will come; nothing that matches
 * the filters, with Clear filters after it.
 */
export const Empty: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-room">
        <RecordTable
          label="Reports"
          head={{ label: "Account" }}
          columns={reportColumns}
          rows={[]}
          empty={<FeedNotice>No open reports.</FeedNotice>}
        />
        <RecordTable
          label="Accounts"
          head={{ label: "Account" }}
          columns={accountColumns}
          rows={[]}
          empty={
            <FeedNotice action={<TextButton onPress={() => undefined}>Clear filters</TextButton>}>
              No accounts match these filters.
            </FeedNotice>
          }
        />
      </Stack>
    </Column>
  ),
};

/** Every state of a record: by its glyph's shape and its word; what waits for someone weighs most, what is over least. */
export const States: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-line">
        {(Object.keys(stateWords) as RecordState[]).map((state) => (
          <div key={state}>
            <StateMark state={state}>{stateWords[state]}</StateMark>
          </div>
        ))}
      </Stack>
    </Column>
  ),
};

/**
 * A report open from its queue, in the app's frame: the report in the main column under the way back; from `wide`,
 * the queue beside it, compact (its accounts and their states), the open report marked; under `wide`, the report
 * alone.
 */
export const ReportOpen: Story = {
  parameters: { layout: "fullscreen" },
  render: () => (
    <Frame>
      <FrameSide label="Main">
        <FrameMark href="/" label="my.example" light={wordmark(color.text.light)} dark={wordmark(color.text.dark)} />
        {places}
      </FrameSide>
      <FrameMain>
        <FrameHeading>Report 1042</FrameHeading>
        <ReportDetail />
      </FrameMain>
      <FrameAside label="Reports">
        <RecordTable
          label="Reports"
          head={{ label: "Account" }}
          columns={reportColumns.filter((c) => c.key === "state")}
          rows={reports.map(reportRow)}
          current="1042"
          onOpen={() => undefined}
          contained
        />
      </FrameAside>
      <FrameDock label="Main" tabs={places} />
    </Frame>
  ),
};
