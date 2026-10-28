// SPDX-License-Identifier: AGPL-3.0-only
// Pieces that the admin screens share, so that every list and every detail page of the admin area has the
// same shape: the permission gate and its message, the paged list of records (rows with dividers, the empty
// state and "show more"), the titled section of a detail page, a server's state in words, a domain in both
// of its forms, and dates in the viewer's locale and time zone. The facts of a record are @my/ui's DescriptionList.
import {
  Box,
  Button,
  Card,
  DetailSection,
  EmptyState,
  Pagination,
  Stack,
  Table,
  TableBody,
  TableHead,
  Text,
} from "@my/ui";
import { Fragment, type ReactNode, type Ref, type RefObject, useEffect, useRef } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { domainToUnicode } from "../../../lib/idn";
import type { InstanceState, Permission } from "../api/staff";
import { type PagedList, type PagedPages, useListPages } from "../hooks/pagedList";
import { useHasPermission } from "../hooks/usePermissions";

export { DetailSection };

const stateLabels: Readonly<Record<InstanceState, ClientMessageKey>> = {
  available: "client:admin.instance.state.available",
  unavailable: "client:admin.instance.state.unavailable",
};

/** A server's state in words, not by color alone (WCAG 1.4.1): the color only repeats it. */
export function InstanceStateLabel({ state }: { state: InstanceState }) {
  const t = useT();
  return (
    <Text weight="semibold" tone={state === "unavailable" ? "danger" : "success"}>
      {t(stateLabels[state])}
    </Text>
  );
}

/**
 * A domain as people read it: its U-labels (`例え.jp`), followed by the A-labels the server keeps
 * (`xn--r8jz45g.jp`) when they differ, so that a look-alike in another script stands out. A domain reads left
 * to right in every locale.
 */
export function DomainName({ domain }: { domain: string }) {
  const t = useT();
  const unicode = domainToUnicode(domain);
  if (unicode === domain) return <span dir="ltr">{domain}</span>;
  return (
    <>
      <span dir="ltr">{unicode}</span>{" "}
      <Text size="sm" weight="normal" tone="muted">
        {t("client:admin.domain.ascii", { domain })}
      </Text>
    </>
  );
}

/** A moment in the viewer's locale and time zone, as a `<time>`. */
export function DateTime({ value }: { value: string }) {
  const intl = useIntl();
  return <time dateTime={value}>{intl.formatDate(value, { dateStyle: "medium", timeStyle: "short" })}</time>;
}

/** A UTC calendar day (`YYYY-MM-DD`) as a date in the viewer's locale; it is the same day in every time zone. */
export function Day({ value }: { value: string }) {
  const intl = useIntl();
  return <time dateTime={value}>{intl.formatDate(value, { dateStyle: "medium", timeZone: "UTC" })}</time>;
}

/** The button under a list that loads its next page, while there is one. */
function MoreButton<T>({ list }: { list: PagedList<T> }) {
  const t = useT();
  if (!list.hasMore) return null;
  return (
    <Box>
      <Button onPress={list.loadMore} isPending={list.isLoadingMore}>
        {t("client:admin.list.more")}
      </Button>
    </Box>
  );
}

/** What an admin screen shows instead of its content to a viewer whose roles lack `permission`. */
export function MissingPermission({ permission }: { permission: Permission }) {
  const t = useT();
  return (
    <EmptyState
      heading={t("client:admin.missing_permission.heading")}
      description={t("client:admin.missing_permission.description", { permission })}
    />
  );
}

/**
 * Shows `children` only to a viewer whose roles have `permission`, and the reason to the others. The route
 * reads nothing for them either (routes/-admin/adminRoute.tsx); the server checks every request.
 */
export function RequirePermission({ permission, children }: { permission: Permission; children: ReactNode }) {
  const allowed = useHasPermission(permission);
  if (!allowed) return <MissingPermission permission={permission} />;
  return children;
}

export type RecordListProps<T> = {
  /** The records loaded so far and loading more. */
  list: PagedList<T>;
  /** A stable id of a record: the row's key and its `data-row-id` (lib/useFocusAfterRemoval.ts). */
  getId: (item: T) => string;
  /** One row: a heading (`h2`, or one below the region's heading) and its facts. */
  children: (item: T) => ReactNode;
  /** What an empty list shows, an `EmptyState`. */
  empty: ReactNode;
  /** The `<ul>`, for lists whose rows go away (useFocusAfterRemoval). */
  listRef?: Ref<HTMLUListElement>;
  /** Why the next page failed to load (a `RegionError` without a retry of its own), above "show more", which tries again. */
  moreError?: ReactNode;
};

/**
 * A list of records of an admin screen (web-client/16-look-states-and-wording.md G3): one row per record, divided by lines, each row a
 * size container for its facts, then "show more" while there are more. An empty list shows `empty` instead.
 */
export function RecordList<T>({ list, getId, children, empty, listRef, moreError }: RecordListProps<T>) {
  if (list.items.length === 0) return empty;
  return (
    <Stack gap="4">
      <Stack as="ul" ref={listRef} divided>
        {list.items.map((item) => {
          const id = getId(item);
          return (
            <Box as="li" key={id} data-row-id={id} container paddingY="3">
              {children(item)}
            </Box>
          );
        })}
      </Stack>
      {moreError}
      <MoreButton list={list} />
    </Stack>
  );
}

export type RecordTableProps<T> = {
  /** The records loaded so far and loading more. */
  list: PagedPages<T>;
  /** What the table holds; visually hidden, as the heading and the toolbar around it already say it. */
  caption: string;
  /** The `TableColumnHeader`s of the one header row. */
  head: ReactNode;
  /** A stable id of a record: the row's key. */
  getId: (item: T) => string;
  /** One row: a `TableRow` (with `data-row-id`, lib/useFocusAfterRemoval.ts) whose `TableRowHeader` names the record. */
  children: (item: T) => ReactNode;
  /** What an empty list shows, an `EmptyState`. */
  empty: ReactNode;
  /** Why the next page failed to load (a `RegionError` without a retry of its own), above the pagination, whose next tries again. */
  moreError?: ReactNode;
  /** The `<table>`, for lists whose rows go away (useFocusAfterRemoval). */
  tableRef?: Ref<HTMLTableElement>;
};

/**
 * The list of an admin screen in the admin mode's form: a `Table` in a
 * `Card`, one page of it at a time, with `Pagination` below. The API pages by cursor, so the pagination has
 * previous and next only; next past the loaded pages loads one more (useListPages). Key the component by whatever
 * the list is of (the filter), so that another list starts at its first page.
 */
export function RecordTable<T>({
  list,
  caption,
  head,
  getId,
  children,
  empty,
  moreError,
  tableRef,
}: RecordTableProps<T>) {
  const t = useT();
  const pages = useListPages(list);
  if (list.items.length === 0) return empty;
  return (
    <Stack gap="4">
      <Card padding="0">
        <Table ref={tableRef} caption={caption}>
          <TableHead>{head}</TableHead>
          <TableBody>
            {pages.items.map((item) => (
              <Fragment key={getId(item)}>{children(item)}</Fragment>
            ))}
          </TableBody>
        </Table>
      </Card>
      {moreError}
      {(pages.previous !== undefined || pages.next !== undefined) && (
        <Pagination
          label={t("client:admin.list.pages")}
          previousLabel={t("client:admin.list.previous")}
          nextLabel={t("client:admin.list.next")}
          previous={pages.previous === undefined ? null : { onPress: pages.previous }}
          next={pages.next === undefined ? null : { onPress: pages.next }}
        />
      )}
    </Stack>
  );
}

/**
 * Moves focus to `target` once `value` has changed after `expect()` was called: a change that takes the pressed
 * button away (resolving a report, deciding an appeal) leaves focus on the heading of the button's region.
 */
export function useFocusAfterChange(target: RefObject<HTMLElement | null>, value: unknown): () => void {
  const pendingRef = useRef(false);
  // Focus is moved in the DOM, outside React, once the page shows the change.
  useEffect(() => {
    if (!pendingRef.current) return;
    pendingRef.current = false;
    target.current?.focus();
  }, [target, value]);
  return () => {
    pendingRef.current = true;
  };
}
