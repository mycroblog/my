// SPDX-License-Identifier: AGPL-3.0-only
// The custom emoji screen below its heading (ADR-0051, manage_custom_emojis; web-client/screens/admin-custom-emojis.md):
// the emoji of this server or of other servers (chosen in the list's toolbar and kept in the URL by the route,
// web-client/README.md S3), newest first, with a search, built from the list-and-add part (`ListSection`): its one
// primary action opens the form that registers a local emoji with an image file in place. A local emoji's settings
// change in a dialog, its image is replaced by choosing another file, and deleting it asks first in an alert dialog (it
// cannot be undone); a remote emoji is enabled or disabled, or copied into one of this server under a shortcode chosen
// in a dialog. What a change or a registration did, and a failed change, show above the rows. A second tab (kept in the
// URL too) holds the requests of the holders of create_custom_emojis (ADR-0070 §2), with the number of the pending ones
// in its name (ADR-0070 §9).
import {
  Badge,
  Button,
  Checkbox,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  Dialog,
  DialogTrigger,
  EmojiTile,
  FileTrigger,
  FormActionBar,
  Heading,
  Hint,
  InlineError,
  ListSection,
  ListToolbar,
  Select,
  SelectItem,
  Stack,
  Tab,
  TabList,
  TabPanel,
  Tabs,
  Text,
  TextField,
} from "@my/ui";
import { type SubmitEvent, useId, useRef, useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { DateTime, Section, DomainName, RequirePermission } from "../../moderation";
import {
  acceptedEmojiTypes,
  type AdminCustomEmoji,
  categoryLimit,
  type CustomEmojiFilter,
  type CustomEmojiOrigin,
  customEmojiOrigins,
  federatedEmojiBytes,
  maxAliases,
  shortcodePattern,
} from "../api/customEmojis";
import type { CustomEmojiRequestState } from "../api/customEmojiRequests";
import { usePendingCustomEmojiRequestCount } from "../hooks/useCustomEmojiRequests";
import {
  type CustomEmojiActions,
  type CustomEmojiForm,
  emojiInText,
  parseAliases,
  useAdminCustomEmojis,
  useCustomEmojiActions,
  useCustomEmojiForm,
} from "../hooks/useCustomEmojis";
import { AccountLink, CustomEmojiRequests } from "./CustomEmojiRequests";

/** The tabs of the screen: the emoji, and the requests for new ones. */
export type CustomEmojiTab = "emoji" | "requests";
const screenTabs = ["emoji", "requests"] as const satisfies readonly CustomEmojiTab[];

function isScreenTab(key: unknown): key is CustomEmojiTab {
  return screenTabs.some((tab) => tab === key);
}

const originLabels: Readonly<Record<CustomEmojiOrigin, ClientMessageKey>> = {
  local: "client:admin.custom_emojis.filter.local",
  remote: "client:admin.custom_emojis.filter.remote",
};

function isOrigin(key: unknown): key is CustomEmojiOrigin {
  return customEmojiOrigins.some((origin) => origin === key);
}

type EmojiListProps = {
  /** The list's filter, from the URL. */
  filter: CustomEmojiFilter;
  /** Shows another filter: the route puts it into the URL. */
  onFilterChange: (filter: CustomEmojiFilter) => void;
};

export type CustomEmojiSettingsProps = EmojiListProps & {
  /** The tab of the screen, from the URL. */
  tab: CustomEmojiTab;
  /** Shows another tab: the route puts it into the URL. */
  onTabChange: (tab: CustomEmojiTab) => void;
  /** The state of the listed requests, from the URL. */
  requestState: CustomEmojiRequestState;
  /** Shows the requests in another state: the route puts it into the URL. */
  onRequestStateChange: (state: CustomEmojiRequestState) => void;
};

export function CustomEmojiSettings(props: CustomEmojiSettingsProps) {
  return (
    <RequirePermission permission="manage_custom_emojis">
      <ScreenTabs {...props} />
    </RequirePermission>
  );
}

function ScreenTabs({ tab, onTabChange, requestState, onRequestStateChange, ...list }: CustomEmojiSettingsProps) {
  const t = useT();
  const pending = usePendingCustomEmojiRequestCount();
  return (
    <Tabs
      selectedKey={tab}
      onSelectionChange={(key) => {
        if (isScreenTab(key)) onTabChange(key);
      }}
    >
      <TabList aria-label={t("client:admin.custom_emojis.tabs.label")}>
        <Tab id="emoji">{t("client:admin.custom_emojis.tabs.emoji")}</Tab>
        <Tab id="requests">
          <Cluster as="span" inline align="center" gap="2" wrap={false}>
            {t("client:admin.custom_emojis.tabs.requests")}
            {pending !== undefined && pending.count > 0 && (
              <Badge
                count={pending.more ? pending.count + 1 : pending.count}
                max={pending.more ? pending.count : 99}
                label={t(
                  pending.more
                    ? "client:admin.custom_emojis.tabs.pending_more"
                    : "client:admin.custom_emojis.tabs.pending",
                  { count: pending.count },
                )}
              />
            )}
          </Cluster>
        </Tab>
      </TabList>
      <TabPanel id="emoji" gap="8" offset="4">
        <CustomEmojiList {...list} />
      </TabPanel>
      <TabPanel id="requests" gap="8" offset="4">
        <Section surface="card" title={t("client:admin.custom_emoji_requests.title")}>
          <CustomEmojiRequests state={requestState} onStateChange={onRequestStateChange} />
        </Section>
      </TabPanel>
    </Tabs>
  );
}

/** The form that registers a local emoji, in the add panel of the list: it closes once the emoji is registered. */
function NewCustomEmojiForm({ form, close }: { form: CustomEmojiForm; close: () => void }) {
  const t = useT();
  const imageId = useId();
  const imageErrorId = useId();
  const { draft } = form;
  const aliasCount = parseAliases(draft.aliases).length;
  return (
    <Stack
      as="form"
      gap="6"
      onSubmit={(event) => {
        form.submit(event, close);
      }}
    >
      <Stack gap="2" role="group" aria-labelledby={imageId}>
        <Text size="sm" tone="fg" weight="medium" id={imageId}>
          {t("client:admin.custom_emojis.image")}
        </Text>
        <Hint>{t("client:admin.custom_emojis.image.description")}</Hint>
        <div>
          <FileTrigger
            acceptedFileTypes={acceptedEmojiTypes}
            onSelect={(files) => {
              const file = files?.[0];
              if (file !== undefined) form.change({ file });
            }}
          >
            <Button size="sm" aria-describedby={form.missingImage ? `${imageId} ${imageErrorId}` : imageId}>
              {t("client:admin.custom_emojis.image.choose")}
            </Button>
          </FileTrigger>
        </div>
        {draft.file !== null && (
          <Text as="p" size="sm" wrap="anywhere">
            {t("client:admin.custom_emojis.image.chosen", { name: draft.file.name })}
          </Text>
        )}
        {draft.file !== null && draft.file.size > federatedEmojiBytes && (
          <Text as="p" size="sm" tone="warning">
            {t("client:admin.custom_emojis.image.large_hint")}
          </Text>
        )}
        {form.missingImage && (
          <InlineError alert id={imageErrorId}>
            {t("client:admin.custom_emojis.image.required")}
          </InlineError>
        )}
      </Stack>
      <TextField
        label={t("client:admin.custom_emojis.shortcode")}
        description={t("client:admin.custom_emojis.shortcode.description")}
        value={draft.shortcode}
        onChange={(shortcode) => {
          form.change({ shortcode });
        }}
        isRequired
        pattern={shortcodePattern}
        maxLength={30}
        autoComplete="off"
        spellCheck="false"
        errorMessage={t("client:admin.custom_emojis.shortcode.invalid")}
      />
      <TextField
        label={t("client:admin.custom_emojis.category")}
        description={t("client:admin.custom_emojis.category.description", { max: String(categoryLimit) })}
        value={draft.category}
        onChange={(category) => {
          form.change({ category });
        }}
        maxLength={categoryLimit}
        autoComplete="off"
      />
      <TextField
        label={t("client:admin.custom_emojis.aliases")}
        description={t("client:admin.custom_emojis.aliases.description", { max: String(maxAliases) })}
        value={draft.aliases}
        onChange={(aliases) => {
          form.change({ aliases });
        }}
        {...(aliasCount > maxAliases && {
          isInvalid: true,
          errorMessage: t("client:admin.custom_emojis.aliases.too_many", { max: String(maxAliases) }),
        })}
        autoComplete="off"
        spellCheck="false"
      />
      <Checkbox
        isSelected={draft.visibleInPicker}
        onChange={(visibleInPicker) => {
          form.change({ visibleInPicker });
        }}
        description={t("client:admin.custom_emojis.visible_in_picker.description")}
      >
        {t("client:admin.custom_emojis.visible_in_picker")}
      </Checkbox>
      <FormActionBar
        result={
          form.error === undefined
            ? undefined
            : { tone: "danger", message: t("client:admin.custom_emojis.create_failed", { reason: form.error }) }
        }
      >
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.custom_emojis.submit")}
        </Button>
        <Button onPress={close}>{t("client:admin.custom_emojis.cancel")}</Button>
      </FormActionBar>
    </Stack>
  );
}

const originChoices = customEmojiOrigins.map((id) => ({ id, label: originLabels[id] }));

/** What narrows the list: this server or other servers, and the search. */
function EmojiToolbar({ filter, onFilterChange }: EmojiListProps) {
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
function SearchForm({ filter, onFilterChange }: EmojiListProps) {
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

/**
 * The emoji of the filter in the list-and-add part. Registering is the list's one primary action; other servers' emoji
 * come with their posts, so their list says how to get one here instead.
 */
function CustomEmojiList({ filter, onFilterChange }: EmojiListProps) {
  const t = useT();
  const listRef = useRef<HTMLUListElement>(null);
  const list = useAdminCustomEmojis(filter);
  const removed = useFocusAfterRemoval(
    listRef,
    list.items.map((emoji) => emoji.id),
  );
  const actions = useCustomEmojiActions(removed);
  const form = useCustomEmojiForm(actions.report);
  const searching = filter.shortcode !== undefined || filter.domain !== undefined;
  const count = list.items.length;
  let result: { tone: "success" | "danger"; message: string } | undefined;
  if (actions.error !== undefined)
    result = { tone: "danger", message: t("client:admin.custom_emojis.change_failed", { reason: actions.error }) };
  else if (actions.status !== undefined) result = { tone: "success", message: actions.status };
  return (
    <ListSection
      title={t("client:admin.custom_emojis.list.title")}
      toolbar={<EmojiToolbar filter={filter} onFilterChange={onFilterChange} />}
      items={list.items}
      itemKey={(emoji) => emoji.id}
      renderItem={(emoji) => <CustomEmojiRow emoji={emoji} actions={actions} />}
      listRef={listRef}
      countLabel={
        list.hasMore
          ? t("client:admin.custom_emojis.count_at_least", { count })
          : t("client:admin.custom_emojis.count", { count })
      }
      hasMore={list.hasMore}
      more={{ label: t("client:admin.list.more"), onPress: list.loadMore, isPending: list.isLoadingMore }}
      empty={
        searching
          ? { heading: t("client:admin.custom_emojis.empty.search") }
          : filter.origin === "local"
            ? {
                heading: t("client:admin.custom_emojis.empty.local"),
                description: t("client:admin.custom_emojis.empty.local.description"),
              }
            : { heading: t("client:admin.custom_emojis.empty.remote") }
      }
      add={
        filter.origin === "local"
          ? {
              label: t("client:admin.custom_emojis.new.title"),
              panel: (close) => <NewCustomEmojiForm form={form} close={close} />,
            }
          : { unavailable: t("client:admin.custom_emojis.remote_add") }
      }
      result={result}
    />
  );
}

/** A value of an emoji that is not set. */
function None() {
  const t = useT();
  return <Text tone="muted">{t("client:admin.custom_emojis.none")}</Text>;
}

/** An emoji: its image and shortcode, its settings, and the buttons that change it. */
function CustomEmojiRow({ emoji, actions }: { emoji: AdminCustomEmoji; actions: CustomEmojiActions }) {
  const t = useT();
  const name = emojiInText(emoji);
  const heading = `:${emoji.shortcode}:`;
  const isPending = actions.pendingId === emoji.id;
  return (
    <Stack gap="3">
      <Cluster gap="3" align="center" wrap={false}>
        {/* The name of the emoji; the image beside it repeats it. */}
        <EmojiTile
          emoji={
            emoji.url !== undefined && emoji.static_url !== undefined
              ? { shortcode: emoji.shortcode, url: emoji.url, static_url: emoji.static_url }
              : undefined
          }
        />
        <Stack gap="1" shrink>
          <Heading level={3} wrap="anywhere" shrink>
            <Text as="span" weight="semibold" mono wrap="anywhere" dir="ltr">
              {heading}
            </Text>
          </Heading>
          {emoji.url === undefined && (
            <Text size="sm" tone="muted">
              {t("client:admin.custom_emojis.no_image")}
            </Text>
          )}
        </Stack>
      </Cluster>
      <DescriptionList columns={3}>
        {!emoji.local && (
          <DescriptionItem term={t("client:admin.custom_emojis.domain")}>
            {emoji.domain !== undefined && <DomainName domain={emoji.domain} />}
          </DescriptionItem>
        )}
        <DescriptionItem term={t("client:admin.custom_emojis.state")}>
          {t(emoji.enabled ? "client:admin.custom_emojis.state.enabled" : "client:admin.custom_emojis.state.disabled")}
        </DescriptionItem>
        {emoji.local && (
          <>
            <DescriptionItem term={t("client:admin.custom_emojis.picker")}>
              {t(
                emoji.visible_in_picker
                  ? "client:admin.custom_emojis.picker.shown"
                  : "client:admin.custom_emojis.picker.hidden",
              )}
            </DescriptionItem>
            <DescriptionItem term={t("client:admin.custom_emojis.category")}>
              {emoji.category === undefined ? <None /> : <span dir="auto">{emoji.category}</span>}
            </DescriptionItem>
            <DescriptionItem term={t("client:admin.custom_emojis.aliases")}>
              {emoji.aliases.length === 0 ? <None /> : <span dir="ltr">{emoji.aliases.join(", ")}</span>}
            </DescriptionItem>
            <DescriptionItem term={t("client:admin.custom_emojis.created_by")}>
              {emoji.created_by !== undefined ? (
                <AccountLink acct={emoji.created_by.acct} />
              ) : emoji.created_by_deleted ? (
                t("client:admin.custom_emojis.created_by.deleted")
              ) : (
                t("client:admin.custom_emojis.created_by.none")
              )}
            </DescriptionItem>
          </>
        )}
        <DescriptionItem term={t("client:admin.custom_emojis.updated_at")}>
          <DateTime value={emoji.updated_at} />
        </DescriptionItem>
      </DescriptionList>
      <Cluster gap="2">
        {emoji.local ? (
          <>
            <EditCustomEmoji emoji={emoji} actions={actions} isPending={isPending} />
            <FileTrigger
              acceptedFileTypes={acceptedEmojiTypes}
              onSelect={(files) => {
                const file = files?.[0];
                if (file !== undefined) actions.replaceImage(emoji, file);
              }}
            >
              <Button
                size="sm"
                isPending={isPending}
                aria-label={t("client:admin.custom_emojis.replace_image_of", { emoji: name })}
              >
                {t("client:admin.custom_emojis.replace_image")}
              </Button>
            </FileTrigger>
            <DeleteCustomEmoji emoji={emoji} actions={actions} isPending={isPending} />
          </>
        ) : (
          <>
            <Button
              size="sm"
              isPending={isPending}
              aria-label={t(
                emoji.enabled ? "client:admin.custom_emojis.disable_emoji" : "client:admin.custom_emojis.enable_emoji",
                { emoji: name },
              )}
              onPress={() => {
                actions.setEnabled(emoji, !emoji.enabled);
              }}
            >
              {t(emoji.enabled ? "client:admin.custom_emojis.disable" : "client:admin.custom_emojis.enable")}
            </Button>
            <CopyCustomEmoji emoji={emoji} actions={actions} isPending={isPending} />
          </>
        )}
      </Cluster>
    </Stack>
  );
}

type RowActionProps = { emoji: AdminCustomEmoji; actions: CustomEmojiActions; isPending: boolean };

/** The settings of a local emoji in a dialog; it closes once they are saved, and keeps a failure inside. */
function EditCustomEmoji({ emoji, actions, isPending }: RowActionProps) {
  const t = useT();
  const name = emojiInText(emoji);
  return (
    <DialogTrigger>
      <Button size="sm" isDisabled={isPending} aria-label={t("client:admin.custom_emojis.edit_emoji", { emoji: name })}>
        {t("client:admin.custom_emojis.edit")}
      </Button>
      <Dialog
        title={t("client:admin.custom_emojis.edit.title", { emoji: name })}
        closeLabel={t("client:admin.custom_emojis.edit.close")}
        isDismissable={false}
        size="md"
      >
        {({ close }) => <EditCustomEmojiForm emoji={emoji} actions={actions} close={close} />}
      </Dialog>
    </DialogTrigger>
  );
}

function EditCustomEmojiForm({
  emoji,
  actions,
  close,
}: {
  emoji: AdminCustomEmoji;
  actions: CustomEmojiActions;
  close: () => void;
}) {
  const t = useT();
  const [category, setCategory] = useState(emoji.category ?? "");
  const [aliases, setAliases] = useState(emoji.aliases.join(" "));
  const [visibleInPicker, setVisibleInPicker] = useState(emoji.visible_in_picker);
  const [enabled, setEnabled] = useState(emoji.enabled);
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState(false);
  const aliasCount = parseAliases(aliases).length;
  const onSubmit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    setIsSaving(true);
    setError(false);
    void actions
      .update(emoji, {
        category: category.trim(),
        aliases: parseAliases(aliases),
        visible_in_picker: visibleInPicker,
        enabled,
      })
      .then((saved) => {
        setIsSaving(false);
        if (saved) close();
        else setError(true);
      });
  };
  return (
    <Stack as="form" gap="4" onSubmit={onSubmit}>
      <TextField
        label={t("client:admin.custom_emojis.category")}
        description={t("client:admin.custom_emojis.category.description", { max: String(categoryLimit) })}
        value={category}
        onChange={setCategory}
        maxLength={categoryLimit}
        autoComplete="off"
        autoFocus
      />
      <TextField
        label={t("client:admin.custom_emojis.aliases")}
        description={t("client:admin.custom_emojis.aliases.description", { max: String(maxAliases) })}
        value={aliases}
        onChange={setAliases}
        {...(aliasCount > maxAliases && {
          isInvalid: true,
          errorMessage: t("client:admin.custom_emojis.aliases.too_many", { max: String(maxAliases) }),
        })}
        autoComplete="off"
        spellCheck="false"
      />
      <Checkbox
        isSelected={visibleInPicker}
        onChange={setVisibleInPicker}
        description={t("client:admin.custom_emojis.visible_in_picker.description")}
      >
        {t("client:admin.custom_emojis.visible_in_picker")}
      </Checkbox>
      <Checkbox
        isSelected={enabled}
        onChange={setEnabled}
        description={t("client:admin.custom_emojis.enabled.description")}
      >
        {t("client:admin.custom_emojis.enabled")}
      </Checkbox>
      {error && actions.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.custom_emojis.change_failed", { reason: actions.error })}
        </InlineError>
      )}
      <Cluster gap="2" justify="end">
        <Button onPress={close}>{t("client:admin.custom_emojis.cancel")}</Button>
        <Button type="submit" variant="primary" isPending={isSaving}>
          {t("client:admin.custom_emojis.save")}
        </Button>
      </Cluster>
    </Stack>
  );
}

/** Copying a remote emoji into one of this server, under a shortcode chosen in a dialog; it closes once copied. */
function CopyCustomEmoji({ emoji, actions, isPending }: RowActionProps) {
  const t = useT();
  const name = emojiInText(emoji);
  return (
    <DialogTrigger>
      <Button size="sm" isDisabled={isPending} aria-label={t("client:admin.custom_emojis.copy_emoji", { emoji: name })}>
        {t("client:admin.custom_emojis.copy")}
      </Button>
      <Dialog
        title={t("client:admin.custom_emojis.copy.title", { emoji: name })}
        closeLabel={t("client:admin.custom_emojis.copy.close")}
        size="md"
      >
        {({ close }) => <CopyCustomEmojiForm emoji={emoji} actions={actions} close={close} />}
      </Dialog>
    </DialogTrigger>
  );
}

function CopyCustomEmojiForm({
  emoji,
  actions,
  close,
}: {
  emoji: AdminCustomEmoji;
  actions: CustomEmojiActions;
  close: () => void;
}) {
  const t = useT();
  // A remote shortcode that a local one may not have (longer, or with other characters) is to be chosen anew.
  const [shortcode, setShortcode] = useState(() =>
    new RegExp(`^${shortcodePattern}$`, "u").test(emoji.shortcode) ? emoji.shortcode : "",
  );
  const [isCopying, setIsCopying] = useState(false);
  const [error, setError] = useState(false);
  const onSubmit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    setIsCopying(true);
    setError(false);
    void actions.copy(emoji, shortcode.trim()).then((copied) => {
      setIsCopying(false);
      if (copied) close();
      else setError(true);
    });
  };
  return (
    <Stack as="form" gap="4" onSubmit={onSubmit}>
      <Hint>{t("client:admin.custom_emojis.copy.description")}</Hint>
      <TextField
        label={t("client:admin.custom_emojis.shortcode")}
        description={t("client:admin.custom_emojis.shortcode.description")}
        value={shortcode}
        onChange={setShortcode}
        isRequired
        pattern={shortcodePattern}
        maxLength={30}
        autoComplete="off"
        spellCheck="false"
        errorMessage={t("client:admin.custom_emojis.shortcode.invalid")}
        autoFocus
      />
      {error && actions.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.custom_emojis.change_failed", { reason: actions.error })}
        </InlineError>
      )}
      <Cluster gap="2" justify="end">
        <Button onPress={close}>{t("client:admin.custom_emojis.cancel")}</Button>
        <Button type="submit" variant="primary" isPending={isCopying}>
          {t("client:admin.custom_emojis.copy.submit")}
        </Button>
      </Cluster>
    </Stack>
  );
}

/** Deleting a local emoji, after an alert dialog: it cannot be undone. */
function DeleteCustomEmoji({ emoji, actions, isPending }: RowActionProps) {
  const t = useT();
  const name = emojiInText(emoji);
  const [isDeleting, setIsDeleting] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <DialogTrigger>
      <Button
        size="sm"
        variant="danger"
        isDisabled={isPending}
        aria-label={t("client:admin.custom_emojis.delete_emoji", { emoji: name })}
      >
        {t("client:admin.custom_emojis.delete")}
      </Button>
      <ConfirmDialog
        title={t("client:admin.custom_emojis.delete.title", { emoji: name })}
        description={t("client:admin.custom_emojis.delete.description", { emoji: name })}
        confirmLabel={t("client:admin.custom_emojis.delete")}
        cancelLabel={t("client:admin.custom_emojis.cancel")}
        isPending={isDeleting}
        {...(failed &&
          actions.error !== undefined && {
            errorMessage: t("client:admin.custom_emojis.change_failed", { reason: actions.error }),
          })}
        onConfirm={(close) => {
          setIsDeleting(true);
          setFailed(false);
          void actions.remove(emoji).then((deleted) => {
            setIsDeleting(false);
            if (deleted) close();
            else setFailed(true);
          });
        }}
      />
    </DialogTrigger>
  );
}
