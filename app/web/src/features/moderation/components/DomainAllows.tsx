// SPDX-License-Identifier: AGPL-3.0-only
// The page of the allowlist of federation below the heading and local navigation of the domain actions screen
// (/moderation/domain-actions/allowed, ADR-0159 §1, §6, manage_domains): the allowed domains by domain, in the admin
// mode's table (40 rows a page on the client, as the API returns them whole), each with "Remove allow" after an alert
// dialog, since removing one while federation_scope is `allowlist` cuts the domain off as a suspension does; then the
// form that allows a domain with a comment that is never published. What the last change did stays above the table in
// a StatusMessage (role="status").
import {
  Button,
  ConfirmDialog,
  DialogTrigger,
  EmptyState,
  FormActionBar,
  Hint,
  Link,
  ReasonField,
  Stack,
  StatusMessage,
  Text,
  TextField,
} from "@my/ui";
import { useRef, useState } from "react";
import { useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { type DomainAllow, domainAllowCommentLimit, domainLimit } from "../api/domainLists";
import { domainInText } from "../hooks/useDomainActions";
import { type DomainAllowsForm, useDomainAllowList, useDomainAllows } from "../hooks/useDomainAllows";
import { wholeList } from "../hooks/pagedList";
import { useHasPermission } from "../hooks/usePermissions";
import { DateTime, DomainName, type RecordColumns, RecordTable, RequirePermission, Section } from "./parts";

export function DomainAllows() {
  return (
    <RequirePermission permission="manage_domains">
      <DomainAllowsBody />
    </RequirePermission>
  );
}

function DomainAllowsBody() {
  const t = useT();
  const allows = useDomainAllowList();
  const listRef = useRef<HTMLDivElement>(null);
  const formRef = useRef<HTMLDivElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    allows.map((allow) => allow.domain),
    () => formRef.current?.querySelector("input"),
  );
  const form = useDomainAllows(removed);
  const canChangeScope = useHasPermission("manage_settings");
  const columns: RecordColumns<DomainAllow> = [
    {
      id: "domain",
      header: t("client:admin.domain_allows.column.domain"),
      wrap: "anywhere",
      cell: (allow) => <DomainName domain={allow.domain} />,
    },
    {
      id: "comment",
      header: t("client:admin.domain_allows.column.comment"),
      wrap: "anywhere",
      cell: (allow) =>
        allow.comment === "" ? (
          <Text tone="muted">{t("client:admin.domain_allows.no_comment")}</Text>
        ) : (
          <Text dir="auto" lang="" wrap="anywhere">
            {allow.comment}
          </Text>
        ),
    },
    {
      id: "created_at",
      header: t("client:admin.domain_allows.column.created_at"),
      cell: (allow) => <DateTime value={allow.created_at} />,
    },
    {
      id: "actions",
      kind: "actions",
      header: t("client:admin.list.column.actions"),
      cell: (allow) => <RemoveAllow allow={allow} form={form} />,
    },
  ];
  return (
    <Stack gap="8">
      <Section surface="card" title={t("client:admin.domain_allows.list.title")}>
        <Stack gap="4">
          <Hint size="inherit">{t("client:admin.domain_allows.description")}</Hint>
          {canChangeScope && (
            <div>
              <Link href="/admin/settings/federation" variant="standalone">
                {t("client:admin.domain_allows.scope_link")}
              </Link>
            </div>
          )}
          <StatusMessage>{form.status}</StatusMessage>
          <RecordTable
            list={wholeList(allows)}
            listRef={listRef}
            caption={t("client:admin.domain_allows.list.title")}
            columns={columns}
            getId={(allow) => allow.domain}
            headingLevel={3}
            empty={
              <EmptyState
                headingLevel={3}
                heading={t("client:admin.domain_allows.empty")}
                description={t("client:admin.domain_allows.empty.description")}
              />
            }
          />
        </Stack>
      </Section>
      <Section surface="card" title={t("client:admin.domain_allows.new.title")}>
        <div ref={formRef}>
          <AllowForm form={form} />
        </div>
      </Section>
    </Stack>
  );
}

function AllowForm({ form }: { form: DomainAllowsForm }) {
  const t = useT();
  return (
    <Stack as="form" gap="6" onSubmit={form.submit}>
      <TextField
        label={t("client:admin.domain_allows.domain")}
        description={t("client:admin.domain_allows.domain.description")}
        value={form.draft.domain}
        onChange={(domain) => {
          form.change({ domain });
        }}
        isRequired
        maxLength={domainLimit}
        autoComplete="off"
        spellCheck="false"
        errorMessage={t("client:admin.domain_allows.domain.required")}
      />
      <ReasonField
        label={t("client:admin.domain_allows.comment")}
        description={t("client:admin.domain_allows.comment.description")}
        value={form.draft.comment}
        onChange={(comment) => {
          form.change({ comment });
        }}
        characterLimit={domainAllowCommentLimit}
        formatRemaining={(remaining) =>
          remaining < 0
            ? t("client:admin.domain_allows.characters_over", { count: -remaining })
            : t("client:admin.domain_allows.characters_left", { count: remaining })
        }
        errorMessage={t("client:admin.domain_allows.comment.too_long", { max: domainAllowCommentLimit })}
      />
      <FormActionBar
        result={
          form.addError === undefined
            ? undefined
            : { tone: "danger", message: t("client:admin.domain_allows.add_failed", { reason: form.addError }) }
        }
      >
        <Button type="submit" variant="primary" isPending={form.isAdding}>
          {t("client:admin.domain_allows.submit")}
        </Button>
      </FormActionBar>
    </Stack>
  );
}

/** Removing an allow, after an alert dialog that says what it cuts off while the federation is limited to the allowlist. */
function RemoveAllow({ allow, form }: { allow: DomainAllow; form: DomainAllowsForm }) {
  const t = useT();
  const domain = domainInText(t, allow.domain);
  const [failed, setFailed] = useState(false);
  return (
    <DialogTrigger>
      <Button
        size="sm"
        variant="danger"
        isDisabled={form.removingDomain === allow.domain}
        aria-label={t("client:admin.domain_allows.remove_domain", { domain })}
      >
        {t("client:admin.domain_allows.remove")}
      </Button>
      <ConfirmDialog
        title={t("client:admin.domain_allows.remove.title", { domain })}
        description={t("client:admin.domain_allows.remove.description")}
        confirmLabel={t("client:admin.domain_allows.remove")}
        cancelLabel={t("client:admin.domain_allows.cancel")}
        isPending={form.removingDomain === allow.domain}
        {...(failed &&
          form.removeError !== undefined && {
            errorMessage: t("client:admin.domain_allows.remove_failed", { reason: form.removeError }),
          })}
        onConfirm={(close) => {
          setFailed(false);
          void form.remove(allow.domain).then((done) => {
            if (done) close();
            else setFailed(true);
          });
        }}
      />
    </DialogTrigger>
  );
}
