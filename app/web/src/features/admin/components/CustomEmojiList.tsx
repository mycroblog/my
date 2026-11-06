// SPDX-License-Identifier: AGPL-3.0-only
// The emoji page of the custom emoji screen (/admin/custom-emojis; ADR-0051, manage_custom_emojis;
// web-client/screens/admin-custom-emojis.md): the emoji of this server or of other servers (chosen in the list's
// toolbar and kept in the URL by the route, web-client/README.md S3), newest first, with a search, one page at a time
// (RecordTable). A row names the emoji (a link to its page, which holds every other change), its state, when its image
// was set, and its one inline action: enabling or disabling it at once. The page's one primary action is "Register a
// custom emoji", a link to the form's page; other servers' emoji come with their posts, so their list says how to get
// one here instead.
import {
  ButtonLink,
  Button,
  Cluster,
  EmojiTile,
  EmptyState,
  Hint,
  InlineError,
  Link,
  ListToolbar,
  Select,
  SelectItem,
  Stack,
  StatusMessage,
  Text,
  TextField,
} from "@my/ui";
import { type SubmitEvent, useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { DateTime, type RecordColumns, RecordTable, RequirePermission, Section } from "../../moderation";
import {
  type AdminCustomEmoji,
  type CustomEmojiFilter,
  type CustomEmojiOrigin,
  customEmojiOrigins,
} from "../api/customEmojis";
import {
  type CustomEmojiEnabling,
  emojiInText,
  useAdminCustomEmojis,
  useCustomEmojiEnabling,
} from "../hooks/useCustomEmojis";
import { CustomEmojiNav } from "./CustomEmojiNav";

const originLabels: Readonly<Record<CustomEmojiOrigin, ClientMessageKey>> = {
  local: "client:admin.custom_emojis.filter.local",
  remote: "client:admin.custom_emojis.filter.remote",
};

function isOrigin(key: unknown): key is CustomEmojiOrigin {
  return customEmojiOrigins.some((origin) => origin === key);
}

export type CustomEmojiListProps = {
  /** The list's filter, from the URL. */
  filter: CustomEmojiFilter;
  /** Shows another filter: the route puts it into the URL. */
  onFilterChange: (filter: CustomEmojiFilter) => void;
};

export function CustomEmojiList(props: CustomEmojiListProps) {
  return (
    <RequirePermission permission="manage_custom_emojis">
      <Stack gap="6">
        <CustomEmojiNav current="emoji" />
        <EmojiSection {...props} />
      </Stack>
    </RequirePermission>
  );
}

function EmojiSection({ filter, onFilterChange }: CustomEmojiListProps) {
  const t = useT();
  const enabling = useCustomEmojiEnabling();
  return (
    <Section surface="card" title={t("client:admin.custom_emojis.list.title")}>
      <Stack gap="4">
        {filter.origin === "local" ? (
          <div>
            <ButtonLink href="/admin/custom-emojis/new" variant="primary">
              {t("client:admin.custom_emojis.new.title")}
            </ButtonLink>
          </div>
        ) : (
          <Hint size="inherit">{t("client:admin.custom_emojis.remote_add")}</Hint>
        )}
        <EmojiToolbar filter={filter} onFilterChange={onFilterChange} />
        <StatusMessage>{enabling.status}</StatusMessage>
        {enabling.error !== undefined && (
          <InlineError size="inherit" alert>
            {t("client:admin.custom_emojis.change_failed", { reason: enabling.error })}
          </InlineError>
        )}
        {/* Keyed by the filter, so that another list starts at its first page. */}
        <EmojiTable
          key={`${filter.origin}:${filter.shortcode ?? ""}:${filter.domain ?? ""}`}
          filter={filter}
          enabling={enabling}
        />
      </Stack>
    </Section>
  );
}

const originChoices = customEmojiOrigins.map((id) => ({ id, label: originLabels[id] }));

/** What narrows the list: this server or other servers, and the search. */
function EmojiToolbar({ filter, onFilterChange }: CustomEmojiListProps) {
  const t = useT();
  return (
    <Stack gap="3">
      <ListToolbar label={t("client:admin.list.filters")}>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.custom_emojis.filter.label")}
          value={filter.origin}
          onChange={(key) => {
            if (isOrigin(key)) onFilterChange({ origin: key });
          }}
          items={originChoices.map((choice) => ({ id: choice.id, name: t(choice.label) }))}
        >
          {(choice) => (
            <SelectItem id={choice.id} textValue={choice.name}>
              {choice.name}
            </SelectItem>
          )}
        </Select>
      </ListToolbar>
      <SearchForm
        key={`${filter.origin}:${filter.shortcode ?? ""}:${filter.domain ?? ""}`}
        filter={filter}
        onFilterChange={onFilterChange}
      />
    </Stack>
  );
}

/** The search of the list: the shortcode, and for other servers the domain. Submitting puts it into the URL. */
function SearchForm({ filter, onFilterChange }: CustomEmojiListProps) {
  const t = useT();
  const [shortcode, setShortcode] = useState(filter.shortcode ?? "");
  const [domain, setDomain] = useState(filter.domain ?? "");
  const searching = filter.shortcode !== undefined || filter.domain !== undefined;
  const onSubmit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    const code = shortcode.trim().replace(/^:+|:+$/gu, "");
    const host = domain.trim();
    onFilterChange({
      origin: filter.origin,
      ...(code === "" ? {} : { shortcode: code }),
      ...(filter.origin === "remote" && host !== "" ? { domain: host } : {}),
    });
  };
  return (
    <Cluster
      as="form"
      gap="2"
      align="end"
      role="search"
      aria-label={t("client:admin.custom_emojis.search.label")}
      onSubmit={onSubmit}
    >
      <TextField
        type="search"
        label={t("client:admin.custom_emojis.search.shortcode")}
        value={shortcode}
        onChange={setShortcode}
        maxLength={128}
        autoComplete="off"
        spellCheck="false"
      />
      {filter.origin === "remote" && (
        <TextField
          type="search"
          label={t("client:admin.custom_emojis.search.domain")}
          value={domain}
          onChange={setDomain}
          maxLength={253}
          autoComplete="off"
          spellCheck="false"
        />
      )}
      <Button type="submit">{t("client:admin.custom_emojis.search.submit")}</Button>
      {searching && (
        <Button
          variant="ghost"
          onPress={() => {
            onFilterChange({ origin: filter.origin });
          }}
        >
          {t("client:admin.custom_emojis.search.clear")}
        </Button>
      )}
    </Cluster>
  );
}

/** The emoji of the filter, one page at a time. */
function EmojiTable({ filter, enabling }: { filter: CustomEmojiFilter; enabling: CustomEmojiEnabling }) {
  const t = useT();
  const list = useAdminCustomEmojis(filter);
  const searching = filter.shortcode !== undefined || filter.domain !== undefined;
  const columns: RecordColumns<AdminCustomEmoji> = [
    {
      id: "emoji",
      header: t("client:admin.custom_emojis.column.emoji"),
      wrap: "anywhere",
      cell: (emoji) => <EmojiName emoji={emoji} />,
    },
    {
      id: "state",
      header: t("client:admin.custom_emojis.state"),
      cell: (emoji) => <EmojiState emoji={emoji} />,
    },
    {
      id: "updated",
      header: t("client:admin.custom_emojis.updated_at"),
      cell: (emoji) => <DateTime value={emoji.updated_at} />,
    },
    {
      id: "actions",
      kind: "actions",
      header: t("client:admin.list.column.actions"),
      cell: (emoji) => <EnableButton emoji={emoji} enabling={enabling} />,
    },
  ];
  let empty = <EmptyState headingLevel={3} heading={t("client:admin.custom_emojis.empty.remote")} />;
  if (searching) empty = <EmptyState headingLevel={3} heading={t("client:admin.custom_emojis.empty.search")} />;
  else if (filter.origin === "local")
    empty = (
      <EmptyState
        headingLevel={3}
        heading={t("client:admin.custom_emojis.empty.local")}
        description={t("client:admin.custom_emojis.empty.local.description")}
      />
    );
  return (
    <RecordTable
      list={list}
      caption={t("client:admin.custom_emojis.list.title")}
      columns={columns}
      getId={(emoji) => emoji.id}
      headingLevel={3}
      empty={empty}
    />
  );
}

/** An emoji's image, and its `:shortcode:` (with the server of a remote one) as the link to its page. */
function EmojiName({ emoji }: { emoji: AdminCustomEmoji }) {
  const t = useT();
  const heading = `:${emoji.shortcode}:`;
  return (
    <Cluster gap="3" align="center" wrap={false}>
      {/* The name of the emoji is the link; the image beside it repeats it. */}
      <EmojiTile
        emoji={
          emoji.url !== undefined && emoji.static_url !== undefined
            ? { shortcode: emoji.shortcode, url: emoji.url, static_url: emoji.static_url }
            : undefined
        }
      />
      <Stack gap="1" shrink>
        <Link href={`/admin/custom-emojis/${encodeURIComponent(emoji.id)}`} wrap="anywhere">
          <Text as="span" weight="semibold" mono wrap="anywhere" dir="ltr">
            {heading}
          </Text>
        </Link>
        {emoji.domain !== undefined && (
          <Text size="sm" tone="muted" wrap="anywhere" dir="ltr">
            {emoji.domain}
          </Text>
        )}
        {emoji.url === undefined && (
          <Text size="sm" tone="muted">
            {t("client:admin.custom_emojis.no_image")}
          </Text>
        )}
      </Stack>
    </Cluster>
  );
}

/** Whether an emoji is shown, in words. */
export function EmojiState({ emoji }: { emoji: AdminCustomEmoji }) {
  const t = useT();
  return (
    <Text tone={emoji.state === "enabled" ? "fg" : "danger"} weight={emoji.state === "enabled" ? "normal" : "semibold"}>
      {t(
        emoji.state === "enabled"
          ? "client:admin.custom_emojis.state.enabled"
          : "client:admin.custom_emojis.state.disabled",
      )}
    </Text>
  );
}

/** Enabling or disabling an emoji at once; the button stays and says the other. */
export function EnableButton({
  emoji,
  enabling,
  size = "sm",
}: {
  emoji: AdminCustomEmoji;
  enabling: CustomEmojiEnabling;
  size?: "sm" | "md";
}) {
  const t = useT();
  const enabled = emoji.state === "enabled";
  return (
    <Button
      size={size}
      isPending={enabling.pendingId === emoji.id}
      aria-label={t(enabled ? "client:admin.custom_emojis.disable_emoji" : "client:admin.custom_emojis.enable_emoji", {
        emoji: emojiInText(emoji),
      })}
      onPress={() => {
        enabling.setEnabled(emoji, !enabled);
      }}
    >
      {t(enabled ? "client:admin.custom_emojis.disable" : "client:admin.custom_emojis.enable")}
    </Button>
  );
}
