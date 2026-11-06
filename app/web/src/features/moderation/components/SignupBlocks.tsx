// SPDX-License-Identifier: AGPL-3.0-only
// The list page of the sign-up blocks screen below its heading and local navigation (FR-719, web-client §14): the
// link to the form of a new block (/moderation/signup-blocks/new), then the blocks, in effect or all, newest first,
// in the admin mode's table: a toolbar with the filter (kept in the URL by the route, web-client/README.md S3), one
// page of the table at a time, previous / next. Each row names what the block matches, which links to the block's
// page (/moderation/signup-blocks/$id) with its comment, dates, and the lift; the list itself changes nothing.
import { ButtonLink, EmptyState, Link, ListToolbar, LocalNav, Select, SelectItem, Stack, Text } from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  type SignupBlock,
  type SignupBlockFilter,
  signupBlockFilters,
  type SignupBlockState,
} from "../api/signupBlocks";
import { signupBlockKindLabels, useSignupBlocks } from "../hooks/useSignupBlocks";
import { DateTime, DomainName, type RecordColumns, RecordTable, RequirePermission } from "./parts";

const stateLabels: Readonly<Record<SignupBlockState, ClientMessageKey>> = {
  active: "client:admin.signup_blocks.state.active",
  expired: "client:admin.signup_blocks.state.expired",
  lifted: "client:admin.signup_blocks.state.lifted",
};

const filterLabels: Readonly<Record<SignupBlockFilter, ClientMessageKey>> = {
  active: "client:admin.signup_blocks.filter.active",
  all: "client:admin.signup_blocks.filter.all",
};

function isFilter(key: unknown): key is SignupBlockFilter {
  return signupBlockFilters.some((filter) => filter === key);
}

/** The address of a block's page. */
function signupBlockHref(id: string): string {
  return `/moderation/signup-blocks/${encodeURIComponent(id)}`;
}

/** The pages of the screen under its heading: the list, and the import of a list of values. */
export function SignupBlocksNav({ current }: { current: "list" | "import" }) {
  const t = useT();
  return (
    <LocalNav
      label={t("client:admin.signup_blocks.pages")}
      items={[
        {
          href: "/moderation/signup-blocks",
          label: t("client:admin.signup_blocks.list.title"),
          current: current === "list",
        },
        {
          href: "/moderation/signup-blocks/import",
          label: t("client:admin.signup_blocks.import.title"),
          current: current === "import",
        },
      ]}
    />
  );
}

export type SignupBlocksProps = {
  /** The selected filter, from the URL. */
  filter: SignupBlockFilter;
  /** Selects another filter: the route puts it into the URL. */
  onFilterChange: (filter: SignupBlockFilter) => void;
};

export function SignupBlocks({ filter, onFilterChange }: SignupBlocksProps) {
  const t = useT();
  return (
    <RequirePermission permission="manage_accounts">
      <Stack gap="4">
        <div>
          <ButtonLink href="/moderation/signup-blocks/new" variant="primary">
            {t("client:admin.signup_blocks.new.title")}
          </ButtonLink>
        </div>
        <ListToolbar label={t("client:admin.list.filters")}>
          <Select
            variant="toolbar"
            aria-label={t("client:admin.signup_blocks.filter.label")}
            value={filter}
            onChange={(key) => {
              if (isFilter(key)) onFilterChange(key);
            }}
          >
            {signupBlockFilters.map((item) => (
              <SelectItem key={item} id={item}>
                {t(filterLabels[item])}
              </SelectItem>
            ))}
          </Select>
        </ListToolbar>
        {/* Another filter is another list: it starts at its first page. */}
        <SignupBlockList key={filter} filter={filter} />
      </Stack>
    </RequirePermission>
  );
}

function SignupBlockList({ filter }: { filter: SignupBlockFilter }) {
  const t = useT();
  const list = useSignupBlocks(filter);
  const empty =
    filter === "active" ? (
      <EmptyState
        headingLevel={2}
        heading={t("client:admin.signup_blocks.empty.active")}
        description={t("client:admin.signup_blocks.empty.active.description")}
      />
    ) : (
      <EmptyState headingLevel={2} heading={t("client:admin.signup_blocks.empty.all")} />
    );
  const columns: RecordColumns<SignupBlock> = [
    {
      id: "value",
      header: t("client:admin.signup_blocks.column.value"),
      wrap: "anywhere",
      cell: (block) => (
        <Link href={signupBlockHref(block.id)} variant="standalone" wrap="anywhere">
          <SignupBlockValue block={block} />
        </Link>
      ),
    },
    {
      id: "kind",
      header: t("client:admin.signup_blocks.kind"),
      cell: (block) => t(signupBlockKindLabels[block.kind]),
    },
    {
      id: "state",
      header: t("client:admin.signup_blocks.state"),
      cell: (block) => <SignupBlockStateText block={block} />,
    },
    {
      id: "expires_at",
      header: t("client:admin.signup_blocks.expires_at"),
      cell: (block) => <SignupBlockEnd block={block} />,
    },
  ];
  return (
    <RecordTable
      list={list}
      caption={t(filterLabels[filter])}
      columns={columns}
      getId={(block) => block.id}
      headingLevel={2}
      empty={empty}
    />
  );
}

/** What a block matches: a domain in both of its forms, else the value as the server keeps it. */
export function SignupBlockValue({ block }: { block: Pick<SignupBlock, "kind" | "value"> }) {
  if (block.kind === "email_domain") return <DomainName domain={block.value} />;
  return <span dir="ltr">{block.value}</span>;
}

/** A block's state, in bold while it is in effect. */
export function SignupBlockStateText({ block }: { block: Pick<SignupBlock, "state"> }) {
  const t = useT();
  return (
    <Text {...(block.state === "active" ? { weight: "semibold" as const } : {})}>{t(stateLabels[block.state])}</Text>
  );
}

/** When a block ends by itself, or that it lasts until it is lifted. */
export function SignupBlockEnd({ block }: { block: Pick<SignupBlock, "expires_at"> }) {
  const t = useT();
  return block.expires_at === undefined ? (
    t("client:admin.signup_blocks.duration.indefinite")
  ) : (
    <DateTime value={block.expires_at} />
  );
}
