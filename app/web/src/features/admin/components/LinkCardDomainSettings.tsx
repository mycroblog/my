// SPDX-License-Identifier: AGPL-3.0-only
// The link card domains screen below its heading (web-client/screens/admin-link-card-domains.md, manage_settings), in
// three pages (web-client/14-admin.md 画面の分け方):
// - the list (`LinkCardDomainSettings`): the rules by domain, 40 a page, with a search of the domain kept in the URL;
//   each domain links to its rule's page. Above it "Add a rule" opens the form.
// - the form (`NewLinkCardDomainRule`): the domain, what to stop and a note; saving opens the rule's page.
// - the page of one rule (`LinkCardDomainDetail`): when it was last set, the form of its mode and note (saved together,
//   the result next to the button), and removing it after an alert dialog, which goes back to the list with a toast.
// The rules load inside their section, whose failure stays on screen while it tries again (G16).
import {
  Button,
  ButtonLink,
  Card,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  DialogTrigger,
  EmptyState,
  FormActionBar,
  Hint,
  Link,
  Radio,
  RadioGroup,
  ReasonField,
  RegionError,
  Spinner,
  Stack,
  TextField,
  useToast,
} from "@my/ui";
import { useNavigate } from "@tanstack/react-router";
import { type ReactNode, type SubmitEvent, useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  DateTime,
  domainInText,
  DomainName,
  type RecordColumns,
  RecordTable,
  RequirePermission,
  Section,
  wholeList,
} from "../../moderation";
import {
  linkCardDomainLimit,
  type LinkCardDomainMode,
  linkCardDomainModes,
  linkCardDomainNoteLimit,
  type LinkCardDomainRule,
} from "../api/linkCardDomains";
import {
  findRule,
  type LinkCardDomainRemoval,
  searchRules,
  useLinkCardDomainEdit,
  useLinkCardDomainForm,
  useLinkCardDomainRemoval,
  useLinkCardDomainRules,
} from "../hooks/useLinkCardDomains";

const modeLabels: Readonly<Record<LinkCardDomainMode, ClientMessageKey>> = {
  no_images: "client:admin.link_card_domains.mode.no_images",
  no_fetch: "client:admin.link_card_domains.mode.no_fetch",
};

const modeDescriptions: Readonly<Record<LinkCardDomainMode, ClientMessageKey>> = {
  no_images: "client:admin.link_card_domains.mode.no_images.description",
  no_fetch: "client:admin.link_card_domains.mode.no_fetch.description",
};

/** Whether a value can be a host name: the server decides the rest, and answers next to the field. */
function looksLikeHost(value: string): boolean {
  return !/[\s/:@]/u.test(value.trim());
}

/** The path of a rule's page. */
function rulePath(domain: string): string {
  return `/admin/link-card-domains/${encodeURIComponent(domain)}`;
}

export type LinkCardDomainSettingsProps = {
  /** The search of the domain, from the URL; undefined lists every rule. */
  search: string | undefined;
  /** Puts another search into the URL; undefined clears it. */
  onSearchChange: (search: string | undefined) => void;
};

/** The list of the rules with its search, and the link to the form that adds one. */
export function LinkCardDomainSettings({ search, onSearchChange }: LinkCardDomainSettingsProps) {
  const t = useT();
  return (
    <RequirePermission permission="manage_settings">
      <Stack gap="6">
        <div>
          <ButtonLink href="/admin/link-card-domains/new" variant="primary">
            {t("client:admin.link_card_domains.new.title")}
          </ButtonLink>
        </div>
        <Section surface="card" title={t("client:admin.link_card_domains.list.title")}>
          <Stack gap="4">
            <SearchForm key={search ?? ""} search={search} onSearchChange={onSearchChange} />
            <Rules>
              {(rules) => (
                // Another search is another list, which starts at its first page.
                <LinkCardDomainTable key={search ?? ""} rules={searchRules(rules, search)} search={search} />
              )}
            </Rules>
          </Stack>
        </Section>
      </Stack>
    </RequirePermission>
  );
}

/** The rules once they are loaded; a spinner while they load, and the failure in their place (G16). */
function Rules({ children }: { children: (rules: LinkCardDomainRule[]) => ReactNode }) {
  const t = useT();
  const rules = useLinkCardDomainRules();
  switch (rules.status) {
    case "loading":
      return (
        <div aria-busy="true">
          <Spinner label={t("client:admin.link_card_domains.loading")} />
        </div>
      );
    case "error":
      return (
        <RegionError
          heading={t("client:admin.link_card_domains.failed")}
          headingLevel={3}
          reason={rules.reason}
          retryLabel={t("client:admin.link_card_domains.retry")}
          onRetry={rules.retry}
          isRetrying={rules.isRetrying}
        />
      );
    case "ready":
      return children(rules.rules);
  }
}

/** The search of the list by domain. Submitting puts it into the URL. */
function SearchForm({ search, onSearchChange }: LinkCardDomainSettingsProps) {
  const t = useT();
  const [value, setValue] = useState(search ?? "");
  const onSubmit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    const domain = value.trim();
    onSearchChange(domain === "" ? undefined : domain);
  };
  return (
    <Cluster
      as="form"
      gap="2"
      align="end"
      role="search"
      aria-label={t("client:admin.link_card_domains.search.label")}
      onSubmit={onSubmit}
    >
      <TextField
        type="search"
        label={t("client:admin.link_card_domains.search.domain")}
        value={value}
        onChange={setValue}
        maxLength={linkCardDomainLimit}
        autoComplete="off"
        spellCheck="false"
        dir="ltr"
      />
      <Button type="submit">{t("client:admin.link_card_domains.search.submit")}</Button>
      {search !== undefined && (
        <Button
          variant="ghost"
          onPress={() => {
            onSearchChange(undefined);
          }}
        >
          {t("client:admin.link_card_domains.search.clear")}
        </Button>
      )}
    </Cluster>
  );
}

function LinkCardDomainTable({ rules, search }: { rules: LinkCardDomainRule[]; search: string | undefined }) {
  const t = useT();
  const columns: RecordColumns<LinkCardDomainRule> = [
    {
      id: "domain",
      header: t("client:admin.link_card_domains.domain"),
      wrap: "anywhere",
      cell: (rule) => (
        <Link href={rulePath(rule.domain)} wrap="anywhere">
          <DomainName domain={rule.domain} />
        </Link>
      ),
    },
    {
      id: "mode",
      header: t("client:admin.link_card_domains.mode"),
      cell: (rule) => t(modeLabels[rule.mode]),
    },
    {
      id: "updated",
      header: t("client:admin.link_card_domains.updated"),
      cell: (rule) => <DateTime value={rule.updated_at} />,
    },
  ];
  return (
    <RecordTable
      list={wholeList(rules)}
      caption={t("client:admin.link_card_domains.list.title")}
      columns={columns}
      getId={(rule) => rule.domain}
      headingLevel={3}
      empty={
        search === undefined ? (
          <EmptyState
            headingLevel={3}
            heading={t("client:admin.link_card_domains.empty")}
            description={t("client:admin.link_card_domains.empty.description")}
          />
        ) : (
          <EmptyState
            headingLevel={3}
            heading={t("client:admin.link_card_domains.search.empty", { search: `⁦${search}⁩` })}
          />
        )
      }
    />
  );
}

/** What to stop on a domain. */
function ModeField({ value, onChange }: { value: LinkCardDomainMode; onChange: (mode: LinkCardDomainMode) => void }) {
  const t = useT();
  return (
    <RadioGroup
      label={t("client:admin.link_card_domains.mode")}
      value={value}
      onChange={(mode) => {
        const choice = linkCardDomainModes.find((m) => m === mode);
        if (choice !== undefined) onChange(choice);
      }}
    >
      {linkCardDomainModes.map((mode) => (
        <Radio key={mode} value={mode} description={t(modeDescriptions[mode])}>
          {t(modeLabels[mode])}
        </Radio>
      ))}
    </RadioGroup>
  );
}

/** The note of a rule. */
function NoteField({ value, onChange }: { value: string; onChange: (note: string) => void }) {
  const t = useT();
  return (
    <ReasonField
      label={t("client:admin.link_card_domains.note")}
      description={t("client:admin.link_card_domains.note.description")}
      value={value}
      onChange={onChange}
      characterLimit={linkCardDomainNoteLimit}
      formatRemaining={(remaining) =>
        remaining < 0
          ? t("client:admin.link_card_domains.characters_over", { count: -remaining })
          : t("client:admin.link_card_domains.characters_left", { count: remaining })
      }
      errorMessage={t("client:admin.link_card_domains.too_long", { max: linkCardDomainNoteLimit })}
    />
  );
}

/** The form that sets the rule of a domain; saving opens the rule's page. */
export function NewLinkCardDomainRule() {
  return (
    <RequirePermission permission="manage_settings">
      <NewRuleForm />
    </RequirePermission>
  );
}

function NewRuleForm() {
  const t = useT();
  const navigate = useNavigate();
  const form = useLinkCardDomainForm((rule) => {
    void navigate({ to: "/admin/link-card-domains/$domain", params: { domain: rule.domain } });
  });
  return (
    <Card>
      <Stack as="form" gap="6" onSubmit={form.submit}>
        <Hint size="inherit">{t("client:admin.link_card_domains.new.description")}</Hint>
        <TextField
          label={t("client:admin.link_card_domains.domain")}
          description={t("client:admin.link_card_domains.domain.description")}
          value={form.draft.domain}
          onChange={(domain) => {
            form.change({ domain });
          }}
          isRequired
          maxLength={linkCardDomainLimit}
          autoComplete="off"
          spellCheck="false"
          dir="ltr"
          validate={(value) => (looksLikeHost(value) ? null : t("client:admin.link_card_domains.domain.invalid"))}
          {...(form.error?.field === "domain" && { isInvalid: true, errorMessage: form.error.message })}
        />
        <ModeField
          value={form.draft.mode}
          onChange={(mode) => {
            form.change({ mode });
          }}
        />
        <NoteField
          value={form.draft.note}
          onChange={(note) => {
            form.change({ note });
          }}
        />
        <FormActionBar
          result={
            form.error?.field === "form"
              ? {
                  tone: "danger",
                  message: t("client:admin.link_card_domains.save_failed", { reason: form.error.message }),
                }
              : undefined
          }
        >
          <Button type="submit" variant="primary" isPending={form.isPending}>
            {t("client:admin.link_card_domains.submit")}
          </Button>
        </FormActionBar>
      </Stack>
    </Card>
  );
}

/** The page of the rule of `domain`: when it was set, its mode and note, and removing it. */
export function LinkCardDomainDetail({ domain }: { domain: string }) {
  return (
    <RequirePermission permission="manage_settings">
      <Rules>{(rules) => <RuleOrNone rule={findRule(rules, domain)} domain={domain} />}</Rules>
    </RequirePermission>
  );
}

function RuleOrNone({ rule, domain }: { rule: LinkCardDomainRule | undefined; domain: string }) {
  const t = useT();
  if (rule !== undefined) return <RulePage rule={rule} />;
  return (
    <EmptyState
      heading={t("client:admin.link_card_domains.not_found", { domain: domainInText(t, domain) })}
      description={t("client:admin.link_card_domains.not_found.description")}
      action={
        <ButtonLink href="/admin/link-card-domains/new" variant="primary">
          {t("client:admin.link_card_domains.new.title")}
        </ButtonLink>
      }
    />
  );
}

function RulePage({ rule }: { rule: LinkCardDomainRule }) {
  const t = useT();
  const navigate = useNavigate();
  const toast = useToast();
  const removal = useLinkCardDomainRemoval((removed) => {
    void navigate({ to: "/admin/link-card-domains" });
    // The toast is announced too.
    toast.show({
      message: t("client:admin.link_card_domains.removed", { domain: domainInText(t, removed.domain) }),
      tone: "success",
    });
  });
  return (
    <Stack gap="8">
      <Section surface="card" title={<DomainName domain={rule.domain} />}>
        <Card>
          <Stack gap="6">
            <DescriptionList>
              <DescriptionItem term={t("client:admin.link_card_domains.updated")}>
                <DateTime value={rule.updated_at} />
              </DescriptionItem>
            </DescriptionList>
            <EditRuleForm rule={rule} />
          </Stack>
        </Card>
      </Section>
      <Section surface="card" title={t("client:admin.link_card_domains.remove.section")}>
        <Card>
          <Stack gap="4">
            <Hint size="inherit">{t("client:admin.link_card_domains.remove.description")}</Hint>
            <div>
              <RemoveRule rule={rule} removal={removal} />
            </div>
          </Stack>
        </Card>
      </Section>
    </Stack>
  );
}

/** The mode and the note of a rule, saved together. */
function EditRuleForm({ rule }: { rule: LinkCardDomainRule }) {
  const t = useT();
  const form = useLinkCardDomainEdit(rule);
  return (
    <Stack as="form" gap="6" onSubmit={form.submit}>
      <ModeField
        value={form.draft.mode}
        onChange={(mode) => {
          form.change({ mode });
        }}
      />
      <NoteField
        value={form.draft.note}
        onChange={(note) => {
          form.change({ note });
        }}
      />
      <FormActionBar result={form.result}>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.link_card_domains.save")}
        </Button>
      </FormActionBar>
    </Stack>
  );
}

/** Removing a rule, after an alert dialog: crawling starts again, and the note cannot be brought back. */
function RemoveRule({ rule, removal }: { rule: LinkCardDomainRule; removal: LinkCardDomainRemoval }) {
  const t = useT();
  const domain = domainInText(t, rule.domain);
  const [isRemoving, setIsRemoving] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <DialogTrigger>
      <Button variant="danger" aria-label={t("client:admin.link_card_domains.remove_rule", { domain })}>
        {t("client:admin.link_card_domains.remove")}
      </Button>
      <ConfirmDialog
        title={t("client:admin.link_card_domains.remove.title", { domain })}
        description={t("client:admin.link_card_domains.remove.description")}
        confirmLabel={t("client:admin.link_card_domains.remove")}
        cancelLabel={t("client:admin.link_card_domains.cancel")}
        isPending={isRemoving}
        {...(failed &&
          removal.error !== undefined && {
            errorMessage: t("client:admin.link_card_domains.remove_failed", { reason: removal.error }),
          })}
        onConfirm={(close) => {
          setIsRemoving(true);
          setFailed(false);
          void removal.remove(rule).then((removed) => {
            setIsRemoving(false);
            if (removed) close();
            else setFailed(true);
          });
        }}
      />
    </DialogTrigger>
  );
}
