// SPDX-License-Identifier: AGPL-3.0-only
// The two searches of a server with a search engine (FR-305, FR-309): accounts, the search every server has, and posts.
// The route keeps the tab in the address (web-client/README.md S3) and renders this only for a viewer who has the
// search of posts, so that no other viewer sees a tab with a single choice.
import { Tab, TabList, TabPanel, Tabs } from "@my/ui";
import type { ReactNode } from "react";
import { useT } from "../../../lib/i18n";
import { StatusSearch } from "./StatusSearch";

export type SearchType = "accounts" | "posts";

export type SearchTabsProps = {
  /** The id of the page heading, which names each search's landmark. */
  headingId: string;
  selected: SearchType;
  onSelect: (type: SearchType) => void;
  /** The search of accounts. */
  accounts: ReactNode;
  /** The search of posts in the page's address; empty for none. */
  postQuery: string;
  onPostSearch: (query: string) => void;
};

export function SearchTabs({ headingId, selected, onSelect, accounts, postQuery, onPostSearch }: SearchTabsProps) {
  const t = useT();
  return (
    <Tabs
      selectedKey={selected}
      onSelectionChange={(key) => {
        onSelect(key === "posts" ? "posts" : "accounts");
      }}
    >
      <TabList aria-label={t("client:search.tabs")}>
        <Tab id="accounts">{t("client:search.tabs.accounts")}</Tab>
        <Tab id="posts">{t("client:search.tabs.posts")}</Tab>
      </TabList>
      <TabPanel id="accounts">{accounts}</TabPanel>
      <TabPanel id="posts">
        <StatusSearch headingId={headingId} query={postQuery} onSearch={onPostSearch} />
      </TabPanel>
    </Tabs>
  );
}
