// SPDX-License-Identifier: AGPL-3.0-only
// The search of posts (FR-309, web-client/screens/search.md): a search landmark with one labelled field and its submit
// button, and under it the posts found, drawn as every timeline draws them. The backend's text is never drawn: the
// posts are the server's `Status` objects (ADR-0149 §8).
import { Button, Cluster, EmptyState, RegionError, Section, Stack, TextField, VisuallyHidden } from "@my/ui";
import { useId } from "react";
import { useSearchScope } from "../../../lib/fullTextSearch";
import { useT } from "../../../lib/i18n";
import { StatusFeed, TimelineSkeleton } from "../../timeline";
import { maxPostQueryLength, postQueryOf } from "../api/statusSearch";
import { type StatusSearch as StatusSearchState, useStatusSearch, useStatusSearchForm } from "../hooks/useStatusSearch";

export type StatusSearchProps = {
  /** The id of the page heading, which names the search landmark. */
  headingId: string;
  /** The search in the page's address; empty for none. */
  query: string;
  onSearch: (query: string) => void;
};

export function StatusSearch({ headingId, query, onSearch }: StatusSearchProps) {
  const t = useT();
  const { results, searchAgain } = useStatusSearch(postQueryOf(query));
  const form = useStatusSearchForm({ query, onSearch, onSearchAgain: searchAgain });
  return (
    <Stack gap="3">
      <Stack as="form" gap="3" role="search" aria-labelledby={headingId} onSubmit={form.submit}>
        <TextField
          {...form.field}
          type="search"
          label={t("client:search.posts.label")}
          description={t("client:search.posts.description")}
          isRequired
          maxLength={maxPostQueryLength}
          // Text of nothing but spaces is not a search: the browser refuses it, as it does an empty field.
          validate={(value) => (value.trim() === "" ? t("client:search.posts.required") : null)}
          errorMessage={t("client:search.posts.required")}
          enterKeyHint="search"
          shrink
        />
        <Cluster justify="end">
          {/* A pending button keeps its focus and tells screen readers it is busy (React Aria Button, isPending). */}
          <Button type="submit" variant="primary" isPending={results.status === "loading"}>
            {t("client:search.posts.submit")}
          </Button>
        </Cluster>
      </Stack>
      <SearchResults results={results} />
    </Stack>
  );
}

/** How many posts the search has found so far, which a screen reader hears when the search answers. */
function foundCount(results: StatusSearchState): number | undefined {
  if (results.status !== "found" || results.isLoadingMore || results.isRefreshing) return undefined;
  // A page can be empty while more follow: nothing can be said until the search has found something or has ended.
  return results.statuses.length === 0 && results.hasMore ? undefined : results.statuses.length;
}

function SearchResults({ results }: { results: StatusSearchState }) {
  const t = useT();
  const headingId = useId();
  const count = foundCount(results);
  return (
    <>
      <VisuallyHidden role="status">
        {count === undefined ? "" : t("client:search.posts.results.count", { count })}
      </VisuallyHidden>
      {results.status !== "idle" && (
        <Section title={t("client:search.posts.results")} headingId={headingId} surface="rows">
          <ResultsBody results={results} headingId={headingId} />
        </Section>
      )}
    </>
  );
}

function ResultsBody({
  results,
  headingId,
}: {
  results: Exclude<StatusSearchState, { status: "idle" }>;
  headingId: string;
}) {
  const t = useT();
  const scope = useSearchScope();
  switch (results.status) {
    case "loading":
      return <TimelineSkeleton />;
    case "error":
      return <SearchFailure failure={results.failure} />;
    case "found":
      if (results.statuses.length === 0 && !results.hasMore) {
        return (
          <EmptyState
            headingLevel={3}
            heading={t("client:search.posts.empty.heading")}
            description={t("client:search.posts.empty.description", { scope })}
          />
        );
      }
      return (
        <StatusFeed
          entries={results.statuses.map((status) => ({ key: status.id, status }))}
          aria-labelledby={headingId}
          hasMore={results.hasMore}
          isLoadingMore={results.isLoadingMore}
          loadMore={results.loadMore}
          loadMoreError={results.loadMoreError}
        />
      );
  }
}

function SearchFailure({ failure }: { failure: Extract<StatusSearchState, { status: "error" }>["failure"] }) {
  const t = useT();
  switch (failure.kind) {
    case "rate-limited":
      return (
        <RegionError
          alert
          headingLevel={3}
          heading={t("client:search.posts.rate_limited.heading")}
          reason={t("client:search.posts.rate_limited.reason")}
        />
      );
    case "unavailable":
      return (
        <RegionError alert headingLevel={3} heading={t("client:search.posts.failed.heading")} reason={failure.reason} />
      );
    case "failed":
      return (
        <RegionError
          alert
          headingLevel={3}
          heading={t("client:search.posts.failed.heading")}
          reason={failure.reason}
          retryLabel={t("client:timeline.retry")}
          onRetry={failure.retry}
          isRetrying={failure.isRetrying}
        />
      );
  }
}
