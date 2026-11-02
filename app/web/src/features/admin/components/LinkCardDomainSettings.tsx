// SPDX-License-Identifier: AGPL-3.0-only
// The link card domains screen below its heading (web-client/screens/admin-link-card-domains.md, manage_settings): the
// form that sets the rule of a domain, then the rules, by domain. The mode of a rule changes in its row and saves at
// once (G24); a rule is removed after an alert dialog, since its note goes with it. What a removal did stays above the
// list in a StatusMessage (role="status"), and a failure stays there too (role="alert"). The rules load inside their
// section, whose failure stays on screen while it tries again (G16).
import {
  Button,
  Card,
  Cluster,
  ConfirmDialog,
  DialogTrigger,
  EmptyState,
  Hint,
  InlineError,
  Radio,
  RadioGroup,
  ReasonField,
  RegionError,
  SaveIndicator,
  Select,
  SelectItem,
  Spinner,
  Stack,
  StatusMessage,
  TableCell,
  TableColumnHeader,
  TableRow,
  TableRowHeader,
  Text,
  TextField,
} from "@my/ui";
import { useRef, useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import {
  DateTime,
  domainInText,
  DomainName,
  RecordTable,
  RequirePermission,
  Section,
  wholeList,
} from "../../moderation";
import { useSaveLabels } from "../../settings";
import {
  linkCardDomainLimit,
  type LinkCardDomainMode,
  linkCardDomainModes,
  linkCardDomainNoteLimit,
  type LinkCardDomainRule,
} from "../api/linkCardDomains";
import {
  type LinkCardDomainRemoval,
  useLinkCardDomainForm,
  useLinkCardDomainMode,
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

export function LinkCardDomainSettings() {
  const t = useT();
  return (
    <RequirePermission permission="manage_settings">
      <Stack gap="8">
        <Section surface="card" title={t("client:admin.link_card_domains.new.title")}>
          <Card>
            <LinkCardDomainForm />
          </Card>
        </Section>
        <Section surface="card" title={t("client:admin.link_card_domains.list.title")}>
          <LinkCardDomainList />
        </Section>
      </Stack>
    </RequirePermission>
  );
}

function LinkCardDomainForm() {
  const t = useT();
  const form = useLinkCardDomainForm();
  return (
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
      <RadioGroup
        label={t("client:admin.link_card_domains.mode")}
        value={form.draft.mode}
        onChange={(mode) => {
          const choice = linkCardDomainModes.find((m) => m === mode);
          if (choice !== undefined) form.change({ mode: choice });
        }}
      >
        {linkCardDomainModes.map((mode) => (
          <Radio key={mode} value={mode} description={t(modeDescriptions[mode])}>
            {t(modeLabels[mode])}
          </Radio>
        ))}
      </RadioGroup>
      <ReasonField
        label={t("client:admin.link_card_domains.note")}
        description={t("client:admin.link_card_domains.note.description")}
        value={form.draft.note}
        onChange={(note) => {
          form.change({ note });
        }}
        characterLimit={linkCardDomainNoteLimit}
        formatRemaining={(remaining) =>
          remaining < 0
            ? t("client:admin.link_card_domains.characters_over", { count: -remaining })
            : t("client:admin.link_card_domains.characters_left", { count: remaining })
        }
        errorMessage={t("client:admin.link_card_domains.too_long", { max: linkCardDomainNoteLimit })}
      />
      {form.error?.field === "form" && (
        <InlineError size="inherit" alert>
          {t("client:admin.link_card_domains.save_failed", { reason: form.error.message })}
        </InlineError>
      )}
      <div>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.link_card_domains.submit")}
        </Button>
      </div>
      <StatusMessage>{form.status}</StatusMessage>
    </Stack>
  );
}

function LinkCardDomainList() {
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
      return <LinkCardDomainTable rules={rules.rules} />;
  }
}

function LinkCardDomainTable({ rules }: { rules: LinkCardDomainRule[] }) {
  const t = useT();
  const tableRef = useRef<HTMLTableElement>(null);
  const removed = useFocusAfterRemoval(
    tableRef,
    rules.map((rule) => rule.domain),
  );
  const removal = useLinkCardDomainRemoval(removed);
  return (
    <Stack gap="3">
      <StatusMessage>{removal.status}</StatusMessage>
      {removal.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.link_card_domains.remove_failed", { reason: removal.error })}
        </InlineError>
      )}
      <RecordTable
        list={wholeList(rules)}
        caption={t("client:admin.link_card_domains.list.title")}
        getId={(rule) => rule.domain}
        tableRef={tableRef}
        empty={
          <EmptyState
            headingLevel={3}
            heading={t("client:admin.link_card_domains.empty")}
            description={t("client:admin.link_card_domains.empty.description")}
          />
        }
        head={
          <>
            <TableColumnHeader>{t("client:admin.link_card_domains.domain")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.link_card_domains.mode")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.link_card_domains.updated")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.list.column.actions")}</TableColumnHeader>
          </>
        }
      >
        {(rule) => <LinkCardDomainRow rule={rule} removal={removal} />}
      </RecordTable>
    </Stack>
  );
}

function LinkCardDomainRow({ rule, removal }: { rule: LinkCardDomainRule; removal: LinkCardDomainRemoval }) {
  const t = useT();
  const labels = useSaveLabels();
  const mode = useLinkCardDomainMode(rule);
  return (
    <TableRow data-row-id={rule.domain}>
      <TableRowHeader wrap="anywhere">
        <Stack gap="1">
          <Text weight="semibold" wrap="anywhere">
            <DomainName domain={rule.domain} />
          </Text>
          {rule.note !== "" && (
            <Text size="sm" tone="muted" wrap="anywhere" dir="auto">
              {rule.note}
            </Text>
          )}
        </Stack>
      </TableRowHeader>
      <TableCell>
        <Stack gap="1">
          <Cluster gap="2" align="center">
            <Select
              aria-label={t("client:admin.link_card_domains.mode_of", { domain: domainInText(t, rule.domain) })}
              value={mode.mode}
              onChange={(key) => {
                const choice = linkCardDomainModes.find((m) => m === key);
                if (choice !== undefined) mode.change(choice);
              }}
            >
              {linkCardDomainModes.map((choice) => (
                <SelectItem key={choice} id={choice} textValue={t(modeLabels[choice])}>
                  {t(modeLabels[choice])}
                </SelectItem>
              ))}
            </Select>
            <SaveIndicator state={mode.saveState} labels={labels} display="icon" />
          </Cluster>
          {mode.error !== undefined && (
            <InlineError size="sm" alert>
              {t("client:admin.link_card_domains.change_failed", { reason: mode.error })}
            </InlineError>
          )}
        </Stack>
      </TableCell>
      <TableCell>
        <DateTime value={rule.updated_at} />
      </TableCell>
      <TableCell>
        <RemoveRule rule={rule} removal={removal} />
      </TableCell>
    </TableRow>
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
      <Button size="sm" variant="danger" aria-label={t("client:admin.link_card_domains.remove_rule", { domain })}>
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
