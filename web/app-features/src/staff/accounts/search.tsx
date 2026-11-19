// SPDX-License-Identifier: AGPL-3.0-only

// The search of accounts, at /moderation/accounts (FR-714): the local and remote accounts, newest first, found by the
// start of a username, a domain, an email address, an email domain or an IP address the account signed up or signed in
// from (one at a time, which the search's facet chooses), and filtered by local or remote and by state, all of which
// the address keeps; each account its handle, where it is, when it was created and its state, opening its screen. A
// page at a time by the server's cursor: a search reads a bounded number of accounts per page, so a page may be short,
// or empty, and still lead on.

import { moderatedAccountPath, sentenceHandleOf } from "@my/app-components";
import {
  moderatedAccountStates,
  useModeratedAccountSearch,
  type AccountSearch,
  type ModeratedAccount,
  type ModeratedAccountState,
} from "@my/app-data";
import {
  FeedNotice,
  FilterBar,
  FilterFacet,
  RecordTable,
  RelativeTime,
  ScreenStack,
  ScreenTitle,
  type FilterValue,
  type RecordColumn,
  type RecordRow,
} from "@my/design-system";
import { useEffect, useState, type ReactNode } from "react";
import type { StaffAddress, StaffSearch } from "../entries";
import { ReadNotice, useStaffStrings } from "../parts";
import { AccountStateMark, cursorOf, filtered, Pages } from "./parts";

/** What the search's words are matched against. */
const searchKeys = ["username", "domain", "email", "email_domain", "ip"] as const;
type SearchKey = (typeof searchKeys)[number];

/** What the search finds by at an address's query: a username's start unless it says otherwise. */
const keyOf = (search: StaffSearch): SearchKey => searchKeys.find((k) => k === search["by"]) ?? "username";

/** What the search asks the server at an address's query. */
export function searchQueryOf(search: StaffSearch): AccountSearch {
  const words = (search["q"] ?? "").trim();
  const key = keyOf(search);
  const origin = search["origin"];
  return {
    ...(words === "" ? {} : { [key]: key === "username" ? words.replace(/^@/, "") : words }),
    origin: origin === "local" || origin === "remote" ? origin : undefined,
    state: moderatedAccountStates.find((s) => s === search["state"]),
    cursor: cursorOf(search),
  };
}

/** The address of the search of the accounts of an email domain, the way from an account to those that share it. */
export const emailDomainSearch = (domain: string): StaffSearch => ({ by: "email_domain", q: domain });

/** Where an account is: this server, or the domain of its own. */
function originOf(t: ReturnType<typeof useStaffStrings>["t"], a: ModeratedAccount): string {
  return a.local ? t("staff_accounts.origin.local") : a.account.acct.slice(a.account.acct.indexOf("@") + 1);
}

type Column = "origin" | "created" | "state";

/** The table of the accounts found. */
function AccountTable({
  accounts,
  busy,
  empty,
}: {
  accounts: readonly ModeratedAccount[];
  busy?: boolean;
  empty?: ReactNode;
}) {
  const { t } = useStaffStrings();
  const columns: RecordColumn<Column>[] = [
    { key: "origin", label: t("staff_accounts.search.column.origin"), bare: true },
    { key: "created", label: t("staff_accounts.search.column.created"), kind: "fit", bare: true },
    { key: "state", label: t("staff_accounts.search.column.state"), kind: "fit", beside: true },
  ];
  const rows = accounts.map((a): RecordRow<Column> => ({
    id: a.id,
    head: sentenceHandleOf(a.account),
    caption: a.account.display_name === "" ? undefined : a.account.display_name,
    href: moderatedAccountPath(a.id),
    cells: {
      origin: originOf(t, a),
      created: <RelativeTime date={a.account.created_at} />,
      state: <AccountStateMark state={a.state} />,
    },
  }));
  return (
    <RecordTable
      label={t("staff_accounts.search.table")}
      head={{ label: t("staff_accounts.search.column.account") }}
      columns={columns}
      rows={rows}
      busy={busy}
      empty={empty}
      even
    />
  );
}

/** The search of accounts: its field and filters, the page of accounts found, and the pages. */
export function AccountSearchScreen(address: StaffAddress): ReactNode {
  const { search, onSearch } = address;
  const { t } = useStaffStrings();
  const q = searchQueryOf(search);
  const key = keyOf(search);
  const page = useModeratedAccountSearch(q);
  // The words as written, sent to the address a moment after the reader stops writing.
  const [words, setWords] = useState(search["q"] ?? "");
  useEffect(() => {
    if (words === (search["q"] ?? "")) return;
    const timer = setTimeout(() => {
      onSearch(filtered(search, { q: words === "" ? undefined : words }));
    }, 400);
    return () => {
      clearTimeout(timer);
    };
  }, [words, search, onSearch]);
  const keys: FilterValue[] = searchKeys.map((k) => ({
    id: k,
    label: t("staff_accounts.search.by", { key: k }),
    icon: k === "ip" || k === "domain" ? "federated" : k === "username" ? "profile" : "direct",
  }));
  const origins: FilterValue[] = [
    { id: "local", label: t("staff_accounts.origin.local"), icon: "local" },
    { id: "remote", label: t("staff_accounts.origin.remote"), icon: "federated" },
  ];
  const states: FilterValue[] = moderatedAccountStates.map((s: ModeratedAccountState) => ({
    id: s,
    label: t("staff_accounts.account_state", { state: s }),
    icon: s === "active" ? "stateActive" : s === "pending" ? "statePending" : "stateDisabled",
  }));
  const applied = words !== "" || q.origin !== undefined || q.state !== undefined || key !== "username";
  const head = (
    <>
      <ScreenTitle>{t("staff_accounts.entry.accounts")}</ScreenTitle>
      <FilterBar
        label={t("staff_accounts.search.filters")}
        search={{
          label: t("staff_accounts.search.field", { key }),
          value: words,
          onChange: setWords,
          clearLabel: t("staff_accounts.search.clear"),
        }}
        applied={applied}
        onClear={() => {
          setWords("");
          onSearch({});
        }}
      >
        <FilterFacet
          label={t("staff_accounts.search.by_facet")}
          any={t("staff_accounts.search.by", { key: "username" })}
          values={keys.filter((k) => k.id !== "username")}
          chosen={key === "username" ? undefined : key}
          onChoose={(by) => {
            onSearch(filtered(search, { by }));
          }}
        />
        <FilterFacet
          label={t("staff_accounts.search.origin")}
          any={t("staff_accounts.search.origin.any")}
          values={origins}
          chosen={q.origin}
          onChoose={(origin) => {
            onSearch(filtered(search, { origin }));
          }}
        />
        <FilterFacet
          label={t("staff_accounts.search.state")}
          any={t("staff_accounts.search.state.any")}
          values={states}
          chosen={q.state}
          onChoose={(state) => {
            onSearch(filtered(search, { state }));
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
          loading={t("staff_accounts.search.loading")}
          failed={t("staff_accounts.search.failed")}
          retry={page.refetch}
        />
      </ScreenStack>
    );
  return (
    <ScreenStack busy={page.stale}>
      {head}
      <AccountTable
        accounts={page.items}
        busy={page.stale}
        empty={
          <FeedNotice>
            {page.next === undefined ? t("staff_accounts.search.empty") : t("staff_accounts.search.empty_more")}
          </FeedNotice>
        }
      />
      <Pages address={address} next={page.next} />
    </ScreenStack>
  );
}
