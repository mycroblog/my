// SPDX-License-Identifier: AGPL-3.0-only
// The page of one blocklist subscription (/moderation/domain-actions/subscriptions/$id, ADR-0159 §5, manage_domains),
// below its heading (the subscription's title) and the link back to the list, in four sections:
// - the drafts: the differences waiting for a moderator, accepted or rejected one by one or all those loaded together
//   (accepting a suspension asks first, NFR-MOD-7), with what was done in a StatusMessage above them;
// - the fetches: the list's URL and format, its last fetch, "Fetch now" (which then reads the subscription again
//   until the fetch has run), and the fetches newest first;
// - the settings: the title and how the subscription applies its list, saved together;
// - deleting it, after an alert dialog that asks whether to lift the actions it issued; it goes back to the list with
//   a toast.
import {
  Box,
  Button,
  Checkbox,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  DialogTrigger,
  EmptyState,
  FormActionBar,
  Hint,
  InlineError,
  Stack,
  StatusMessage,
  Text,
  TextField,
  useToast,
} from "@my/ui";
import { useNavigate } from "@tanstack/react-router";
import { useRef, useState } from "react";
import { useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import {
  type DomainListDraft,
  type DomainListFetch,
  type DomainListSubscription,
  subscriptionTitleLimit,
} from "../api/domainLists";
import { domainActionKindLabels, domainInText } from "../hooks/useDomainActions";
import {
  useDecideDrafts,
  useDeleteSubscription,
  useDomainListDrafts,
  useDomainListSubscription,
  useSubscriptionSettings,
} from "../hooks/useDomainListSubscriptions";
import { useHasPermission } from "../hooks/usePermissions";
import { ApplyFields, FetchOutcome, formatLabels, holdLabels, LastFetch } from "./domainListParts";
import { wholeList } from "../hooks/pagedList";
import {
  DateTime,
  DomainName,
  type RecordColumns,
  RecordTable,
  RequirePermission,
  Section,
  useFocusAfterChange,
} from "./parts";

/** The text of the page's heading: the subscription's title; for a viewer without manage_domains, the screen's name. */
export function DomainListSubscriptionTitle({ id }: { id: string }) {
  const t = useT();
  if (!useHasPermission("manage_domains")) return t("client:admin.domain_lists.detail.title");
  return <PermittedTitle id={id} />;
}

function PermittedTitle({ id }: { id: string }) {
  const { subscription } = useDomainListSubscription(id);
  // An administrator writes the title in any language; a long one wraps anywhere rather than widen the page.
  return (
    <Text dir="auto" lang="" wrap="anywhere">
      {subscription.title}
    </Text>
  );
}

export function DomainListSubscriptionDetail({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_domains">
      <SubscriptionPage id={id} />
    </RequirePermission>
  );
}

function SubscriptionPage({ id }: { id: string }) {
  const page = useDomainListSubscription(id);
  return (
    <Stack gap="8">
      <DraftsSection subscription={page.subscription} />
      <FetchesSection page={page} />
      <SettingsSection subscription={page.subscription} />
      <DeleteSection subscription={page.subscription} />
    </Stack>
  );
}

function DraftsSection({ subscription }: { subscription: DomainListSubscription }) {
  const t = useT();
  const drafts = useDomainListDrafts(subscription.id);
  const listRef = useRef<HTMLDivElement>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const keys = drafts.map((draft) => draft.key);
  const removed = useFocusAfterRemoval(listRef, keys, () => headingRef.current);
  const emptied = useFocusAfterChange(headingRef, drafts.length);
  const decisions = useDecideDrafts(subscription.id, (decided) => {
    const [only] = decided;
    if (decided.length === 1 && only !== undefined) removed(only);
    else emptied();
  });
  const isPending = (draft: DomainListDraft) => decisions.pendingKeys.includes(draft.key);
  const columns: RecordColumns<DomainListDraft> = [
    {
      id: "domain",
      header: t("client:admin.domain_lists.drafts.column.domain"),
      wrap: "anywhere",
      cell: (draft) => <DomainName domain={draft.domain} />,
    },
    {
      id: "change",
      header: t("client:admin.domain_lists.drafts.column.change"),
      wrap: "anywhere",
      cell: (draft) => <DraftChange draft={draft} />,
    },
    {
      id: "actions",
      kind: "actions",
      header: t("client:admin.list.column.actions"),
      cell: (draft) => {
        const domain = domainInText(t, draft.domain);
        return (
          <Cluster gap="2">
            <Button
              size="sm"
              isPending={isPending(draft) && decisions.confirmation === null}
              isDisabled={decisions.isPending && !isPending(draft)}
              aria-label={t("client:admin.domain_lists.drafts.accept_domain", { domain })}
              onPress={() => {
                decisions.decide("accept", [draft]);
              }}
            >
              {t("client:admin.domain_lists.drafts.accept")}
            </Button>
            <Button
              size="sm"
              isDisabled={decisions.isPending}
              aria-label={t("client:admin.domain_lists.drafts.reject_domain", { domain })}
              onPress={() => {
                decisions.decide("reject", [draft]);
              }}
            >
              {t("client:admin.domain_lists.drafts.reject")}
            </Button>
          </Cluster>
        );
      },
    },
  ];
  return (
    <Section surface="card" title={t("client:admin.domain_lists.drafts.title")} headingRef={headingRef}>
      <Stack gap="4">
        <Hint size="inherit">
          {t(
            subscription.mode === "draft"
              ? "client:admin.domain_lists.drafts.description.draft"
              : "client:admin.domain_lists.drafts.description.automatic",
          )}
        </Hint>
        <StatusMessage>{decisions.status}</StatusMessage>
        {decisions.confirmation === null && decisions.error !== undefined && (
          <InlineError alert size="inherit">
            {t("client:admin.domain_lists.drafts.failed", { reason: decisions.error })}
          </InlineError>
        )}
        {drafts.length > 0 && (
          <Stack gap="3">
            <Text as="p">{t("client:admin.domain_lists.drafts.count", { count: drafts.length })}</Text>
            <Cluster gap="2">
              <Button
                variant="primary"
                isPending={decisions.isPending && decisions.pendingKeys.length === drafts.length && drafts.length > 1}
                isDisabled={decisions.isPending}
                onPress={() => {
                  decisions.decide("accept", drafts);
                }}
              >
                {t("client:admin.domain_lists.drafts.accept_all")}
              </Button>
              <Button
                isDisabled={decisions.isPending}
                onPress={() => {
                  decisions.decide("reject", drafts);
                }}
              >
                {t("client:admin.domain_lists.drafts.reject_all")}
              </Button>
            </Cluster>
          </Stack>
        )}
        <RecordTable
          list={wholeList(drafts)}
          listRef={listRef}
          caption={t("client:admin.domain_lists.drafts.title")}
          columns={columns}
          getId={(draft) => draft.key}
          headingLevel={3}
          empty={<EmptyState headingLevel={3} heading={t("client:admin.domain_lists.drafts.empty")} />}
        />
      </Stack>
      <ConfirmDialog
        isOpen={decisions.confirmation !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) decisions.confirmation?.cancel();
        }}
        title={t("client:admin.domain_lists.drafts.confirm.title", {
          count: decisions.confirmation?.suspensions ?? 0,
        })}
        description={t("client:admin.domain_lists.drafts.confirm.description")}
        confirmLabel={t("client:admin.domain_lists.drafts.accept")}
        cancelLabel={t("client:admin.domain_lists.cancel")}
        onConfirm={() => {
          decisions.confirmation?.confirm();
        }}
        isPending={decisions.isPending}
        {...(decisions.confirmation !== null && decisions.error !== undefined
          ? { errorMessage: t("client:admin.domain_lists.drafts.failed", { reason: decisions.error }) }
          : {})}
      />
    </Section>
  );
}

/** What accepting a draft does, why it waits when that is not the draft mode, and the list's public comment. */
function DraftChange({ draft }: { draft: DomainListDraft }) {
  const t = useT();
  const kind = t(domainActionKindLabels[draft.kind]);
  return (
    <Stack gap="1">
      <span>
        {draft.change === "issue"
          ? t("client:admin.domain_lists.drafts.change.issue", { kind })
          : t("client:admin.domain_lists.drafts.change.lift", { kind })}
      </span>
      {draft.held !== "draft_mode" && (
        <Text size="sm" tone="warning">
          {t(holdLabels[draft.held])}
        </Text>
      )}
      {draft.public_comment !== undefined && (
        <Text size="sm" tone="muted" dir="auto" lang="" wrap="anywhere">
          {draft.public_comment}
        </Text>
      )}
    </Stack>
  );
}

function FetchesSection({ page }: { page: ReturnType<typeof useDomainListSubscription> }) {
  const t = useT();
  const { subscription } = page;
  const columns: RecordColumns<DomainListFetch> = [
    {
      id: "fetched_at",
      header: t("client:admin.domain_lists.fetches.column.fetched_at"),
      cell: (fetch) => <DateTime value={fetch.fetched_at} />,
    },
    {
      id: "outcome",
      header: t("client:admin.domain_lists.fetches.column.outcome"),
      wrap: "anywhere",
      cell: (fetch) => <FetchOutcome fetch={fetch} />,
    },
    {
      id: "entry_count",
      header: t("client:admin.domain_lists.fetches.column.entry_count"),
      cell: (fetch) => t("client:admin.domain_lists.domains", { count: fetch.entry_count }),
    },
  ];
  return (
    <Section surface="card" title={t("client:admin.domain_lists.fetches.title")}>
      <Stack gap="4">
        <DescriptionList columns={2}>
          <DescriptionItem term={t("client:admin.domain_lists.url")}>
            <Text mono wrap="anywhere" dir="ltr">
              {subscription.url}
            </Text>
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.domain_lists.format")}>
            {t(formatLabels[subscription.format])}
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.domain_lists.last_fetch")}>
            <LastFetch fetch={subscription.last_fetch} failures={subscription.consecutive_failures} />
          </DescriptionItem>
        </DescriptionList>
        <Hint size="inherit">{t("client:admin.domain_lists.fetches.description")}</Hint>
        <Box>
          <Button onPress={page.fetchNow} isPending={page.isFetching}>
            {t("client:admin.domain_lists.fetch")}
          </Button>
        </Box>
        <StatusMessage>{page.fetchStatus}</StatusMessage>
        {page.fetchError !== undefined && (
          <InlineError alert size="inherit">
            {t("client:admin.domain_lists.fetch.queue_failed", { reason: page.fetchError })}
          </InlineError>
        )}
        <RecordTable
          list={page.fetches}
          caption={t("client:admin.domain_lists.fetches.title")}
          columns={columns}
          getId={(fetch) => fetch.id}
          headingLevel={3}
          empty={<EmptyState headingLevel={3} heading={t("client:admin.domain_lists.fetches.empty")} />}
        />
      </Stack>
    </Section>
  );
}

function SettingsSection({ subscription }: { subscription: DomainListSubscription }) {
  const t = useT();
  const form = useSubscriptionSettings(subscription);
  return (
    <Section surface="card" title={t("client:admin.domain_lists.settings.title")}>
      <Stack as="form" gap="6" onSubmit={form.submit}>
        <Hint size="inherit">{t("client:admin.domain_lists.settings.description")}</Hint>
        <TextField
          label={t("client:admin.domain_lists.title_field")}
          description={t("client:admin.domain_lists.title_field.description")}
          value={form.draft.title}
          onChange={(title) => {
            form.change({ title });
          }}
          isRequired
          maxLength={subscriptionTitleLimit}
          autoComplete="off"
          dir="auto"
          errorMessage={t("client:admin.domain_lists.title_field.required")}
        />
        <ApplyFields draft={form.draft} change={form.change} />
        <FormActionBar
          result={
            form.error !== undefined
              ? { tone: "danger", message: t("client:admin.domain_lists.settings.save_failed", { reason: form.error }) }
              : form.status === undefined
                ? undefined
                : { tone: "success", message: form.status }
          }
        >
          <Button type="submit" variant="primary" isPending={form.isSaving}>
            {t("client:admin.domain_lists.settings.save")}
          </Button>
        </FormActionBar>
      </Stack>
    </Section>
  );
}

function DeleteSection({ subscription }: { subscription: DomainListSubscription }) {
  const t = useT();
  const navigate = useNavigate();
  const toast = useToast();
  const title = `⁨${subscription.title}⁩`;
  const removal = useDeleteSubscription(subscription.id, (lifted) => {
    void navigate({ to: "/moderation/domain-actions/subscriptions" });
    // The toast is announced too.
    toast.show({
      message: lifted
        ? t("client:admin.domain_lists.deleted_lifted", { title })
        : t("client:admin.domain_lists.deleted", { title }),
      tone: "success",
    });
  });
  const [liftActions, setLiftActions] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <Section surface="card" title={t("client:admin.domain_lists.delete.title")}>
      <Stack gap="4">
        <Hint size="inherit">{t("client:admin.domain_lists.delete.description")}</Hint>
        <div>
          <DialogTrigger>
            <Button variant="danger">{t("client:admin.domain_lists.delete")}</Button>
            <ConfirmDialog
              title={t("client:admin.domain_lists.delete.confirm.title", { title })}
              description={t("client:admin.domain_lists.delete.confirm.description")}
              confirmLabel={t("client:admin.domain_lists.delete")}
              cancelLabel={t("client:admin.domain_lists.cancel")}
              isPending={removal.isPending}
              {...(failed &&
                removal.error !== undefined && {
                  errorMessage: t("client:admin.domain_lists.delete_failed", { reason: removal.error }),
                })}
              onConfirm={(close) => {
                setFailed(false);
                void removal.remove(liftActions).then((done) => {
                  if (done) close();
                  else setFailed(true);
                });
              }}
            >
              <Checkbox
                isSelected={liftActions}
                onChange={setLiftActions}
                description={t("client:admin.domain_lists.delete.lift_actions.description")}
              >
                {t("client:admin.domain_lists.delete.lift_actions")}
              </Checkbox>
            </ConfirmDialog>
          </DialogTrigger>
        </div>
      </Stack>
    </Section>
  );
}
