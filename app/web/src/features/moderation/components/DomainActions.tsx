// SPDX-License-Identifier: AGPL-3.0-only
// The list page of the domain actions screen below its heading and local navigation (FR-702, web-client §14): the
// link to the form of a new action (/moderation/domain-actions/new), then the actions, in effect or all, newest
// first, in the admin mode's table: a toolbar with the filter (kept in the URL by the route, web-client/README.md S3),
// one page of the table at a time, previous / next. Each row names its domain, which links to the action's page
// (/moderation/domain-actions/$id) with its comments, dates, and the lift; the list itself changes nothing.
import { ButtonLink, Cluster, EmptyState, Link, ListToolbar, LocalNav, Select, SelectItem, Stack, Text } from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { type DomainAction, type DomainActionFilter, domainActionFilters, type DomainActionState } from "../api/staff";
import { domainActionKindLabels, domainInText, useDomainActions } from "../hooks/useDomainActions";
import { DateTime, DomainName, type RecordColumns, RecordTable, RequirePermission } from "./parts";

const stateLabels: Readonly<Record<DomainActionState, ClientMessageKey>> = {
  active: "client:admin.domain_actions.state.active",
  expired: "client:admin.domain_actions.state.expired",
  lifted: "client:admin.domain_actions.state.lifted",
};

const filterLabels: Readonly<Record<DomainActionFilter, ClientMessageKey>> = {
  active: "client:admin.domain_actions.filter.active",
  all: "client:admin.domain_actions.filter.all",
};

function isFilter(key: unknown): key is DomainActionFilter {
  return domainActionFilters.some((filter) => filter === key);
}

/** The address of an action's page. */
function domainActionHref(id: string): string {
  return `/moderation/domain-actions/${encodeURIComponent(id)}`;
}

/** The address of the form of a new action, filled in with `domain` when there is one. */
function newDomainActionHref(domain: string | undefined): string {
  return domain === undefined
    ? "/moderation/domain-actions/new"
    : `/moderation/domain-actions/new?domain=${encodeURIComponent(domain)}`;
}

/** The pages of the screen under its heading: the list, and Mastodon's CSV. */
export function DomainActionsNav({ current }: { current: "list" | "import" }) {
  const t = useT();
  return (
    <LocalNav
      label={t("client:admin.domain_actions.pages")}
      items={[
        {
          href: "/moderation/domain-actions",
          label: t("client:admin.domain_actions.list.title"),
          current: current === "list",
        },
        {
          href: "/moderation/domain-actions/import",
          label: t("client:admin.domain_actions.csv.title"),
          current: current === "import",
        },
      ]}
    />
  );
}

export type DomainActionsProps = {
  /** The selected filter, from the URL. */
  filter: DomainActionFilter;
  /** The domain whose actions to list, from the URL; undefined lists every domain. */
  domain: string | undefined;
  /** Selects another filter: the route puts it into the URL. */
  onFilterChange: (filter: DomainActionFilter) => void;
  /** The address of the list of every domain under the same filter. */
  everyDomainHref: string;
};

export function DomainActions({ filter, domain, onFilterChange, everyDomainHref }: DomainActionsProps) {
  const t = useT();
  return (
    <RequirePermission permission="manage_domains">
      <Stack gap="4">
        <div>
          <ButtonLink href={newDomainActionHref(domain)} variant="primary">
            {t("client:admin.domain_actions.new.title")}
          </ButtonLink>
        </div>
        {domain !== undefined && (
          <Cluster as="p" align="baseline" gap="2" rowGap="0">
            <Text wrap="anywhere">
              {t("client:admin.domain_actions.list.of_domain", { domain: domainInText(t, domain) })}
            </Text>
            <Link href={everyDomainHref} variant="standalone">
              {t("client:admin.domain_actions.list.every_domain")}
            </Link>
          </Cluster>
        )}
        <ListToolbar label={t("client:admin.list.filters")}>
          <Select
            variant="toolbar"
            aria-label={t("client:admin.domain_actions.filter.label")}
            value={filter}
            onChange={(key) => {
              if (isFilter(key)) onFilterChange(key);
            }}
          >
            {domainActionFilters.map((item) => (
              <SelectItem key={item} id={item}>
                {t(filterLabels[item])}
              </SelectItem>
            ))}
          </Select>
        </ListToolbar>
        {/* Another filter is another list: it starts at its first page. */}
        <DomainActionList key={filter} filter={filter} domain={domain} />
      </Stack>
    </RequirePermission>
  );
}

function DomainActionList({ filter, domain }: { filter: DomainActionFilter; domain: string | undefined }) {
  const t = useT();
  const list = useDomainActions({ filter, domain });
  const empty =
    filter === "active" ? (
      <EmptyState
        headingLevel={2}
        heading={t("client:admin.domain_actions.empty.active")}
        description={t("client:admin.domain_actions.empty.active.description")}
      />
    ) : (
      <EmptyState headingLevel={2} heading={t("client:admin.domain_actions.empty.all")} />
    );
  const columns: RecordColumns<DomainAction> = [
    {
      id: "domain",
      header: t("client:admin.domain_actions.column.domain"),
      wrap: "anywhere",
      cell: (action) => (
        <Link href={domainActionHref(action.id)} variant="standalone" wrap="anywhere">
          <DomainName domain={action.domain} />
        </Link>
      ),
    },
    {
      id: "kind",
      header: t("client:admin.domain_actions.kind"),
      cell: (action) => t(domainActionKindLabels[action.kind]),
    },
    {
      id: "state",
      header: t("client:admin.domain_actions.state"),
      cell: (action) => <DomainActionStateText action={action} />,
    },
    {
      id: "expires_at",
      header: t("client:admin.domain_actions.expires_at"),
      cell: (action) => <DomainActionEnd action={action} />,
    },
  ];
  return (
    <RecordTable
      list={list}
      caption={t(filterLabels[filter])}
      columns={columns}
      getId={(action) => action.id}
      headingLevel={2}
      empty={empty}
    />
  );
}

/** An action's state, in bold while it is in effect. */
export function DomainActionStateText({ action }: { action: Pick<DomainAction, "state"> }) {
  const t = useT();
  return (
    <Text {...(action.state === "active" ? { weight: "semibold" as const } : {})}>{t(stateLabels[action.state])}</Text>
  );
}

/** When an action ends by itself, or that it lasts until it is lifted. */
export function DomainActionEnd({ action }: { action: Pick<DomainAction, "expires_at"> }) {
  const t = useT();
  return action.expires_at === undefined ? (
    t("client:admin.domain_actions.duration.indefinite")
  ) : (
    <DateTime value={action.expires_at} />
  );
}
