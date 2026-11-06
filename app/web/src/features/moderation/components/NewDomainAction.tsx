// SPDX-License-Identifier: AGPL-3.0-only
// The form of a new domain action (FR-702), the page /moderation/domain-actions/new below its heading. The form
// follows the browser's constraint validation (React Aria's native validation); a suspension asks first in an alert
// dialog (ConfirmDialog, NFR-MOD-7), the other kinds take effect at once. What an action did stays under the form in
// a StatusMessage (role="status"), and the form empties but for the domain, which the next action often shares.
import {
  Box,
  Button,
  Checkbox,
  ConfirmDialog,
  DurationSelect,
  InlineError,
  Radio,
  RadioGroup,
  ReasonField,
  Stack,
  StatusMessage,
  TextField,
} from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { type DomainActionKind, domainActionKinds, domainActionTextLimit } from "../api/staff";
import { domainActionKindLabels, domainInText, useDomainActionForm } from "../hooks/useDomainActions";
import { RequirePermission, Section } from "./parts";

const kindDescriptions: Readonly<Record<DomainActionKind, ClientMessageKey>> = {
  limit: "client:admin.domain_actions.kind.limit.description",
  suspend: "client:admin.domain_actions.kind.suspend.description",
  reject_media: "client:admin.domain_actions.kind.reject_media.description",
  reject_reports: "client:admin.domain_actions.kind.reject_reports.description",
  force_sensitive: "client:admin.domain_actions.kind.force_sensitive.description",
};

/** The durations an action can last, in seconds, from the shortest. */
const day = 86_400;
const durations = [day, 7 * day, 30 * day, 90 * day, 180 * day, 365 * day] as const;

export type NewDomainActionProps = {
  /** The domain to fill in, from the URL (the link from a remote server's page); undefined leaves it empty. */
  domain: string | undefined;
};

export function NewDomainAction({ domain }: NewDomainActionProps) {
  const t = useT();
  return (
    <RequirePermission permission="manage_domains">
      <Section surface="card" title={t("client:admin.domain_actions.new.form")}>
        {/* Another domain from the URL starts another form (web-client/README.md R10). */}
        <NewDomainActionForm key={domain ?? ""} initialDomain={domain ?? ""} />
      </Section>
    </RequirePermission>
  );
}

function NewDomainActionForm({ initialDomain }: { initialDomain: string }) {
  const t = useT();
  const form = useDomainActionForm(initialDomain);
  const { draft } = form;
  const formatRemaining = (remaining: number) =>
    remaining < 0
      ? t("client:admin.domain_actions.characters_over", { count: -remaining })
      : t("client:admin.domain_actions.characters_left", { count: remaining });
  const tooLong = t("client:admin.domain_actions.too_long", { max: domainActionTextLimit });
  return (
    <>
      <Stack as="form" onSubmit={form.submit} gap="6">
        <TextField
          label={t("client:admin.domain_actions.domain")}
          description={t("client:admin.domain_actions.domain.description")}
          value={draft.domain}
          onChange={(domain) => {
            form.change({ domain });
          }}
          isRequired
          maxLength={253}
          autoComplete="off"
          errorMessage={t("client:admin.domain_actions.domain.required")}
        />
        <RadioGroup
          label={t("client:admin.domain_actions.kind")}
          value={draft.kind}
          onChange={(kind) => {
            const chosen = domainActionKinds.find((item) => item === kind);
            if (chosen !== undefined) form.change({ kind: chosen });
          }}
          isRequired
          errorMessage={t("client:admin.domain_actions.kind.required")}
        >
          {domainActionKinds.map((kind) => (
            <Radio key={kind} value={kind} description={t(kindDescriptions[kind])}>
              {t(domainActionKindLabels[kind])}
            </Radio>
          ))}
        </RadioGroup>
        <DurationSelect
          label={t("client:admin.domain_actions.duration")}
          description={t("client:admin.domain_actions.duration.description")}
          durations={durations}
          formatDuration={(seconds) => t("client:admin.domain_actions.duration.days", { count: seconds / day })}
          indefiniteLabel={t("client:admin.domain_actions.duration.indefinite")}
          value={draft.expiresIn}
          onChange={(expiresIn) => {
            form.change({ expiresIn });
          }}
        />
        <ReasonField
          label={t("client:admin.domain_actions.public_comment")}
          description={t("client:admin.domain_actions.public_comment.description")}
          value={draft.publicComment}
          onChange={(publicComment) => {
            form.change({ publicComment });
          }}
          characterLimit={domainActionTextLimit}
          formatRemaining={formatRemaining}
          errorMessage={tooLong}
        />
        <ReasonField
          label={t("client:admin.domain_actions.private_note")}
          description={t("client:admin.domain_actions.private_note.description")}
          value={draft.privateNote}
          onChange={(privateNote) => {
            form.change({ privateNote });
          }}
          characterLimit={domainActionTextLimit}
          formatRemaining={formatRemaining}
          errorMessage={tooLong}
        />
        <Checkbox
          isSelected={draft.obfuscate}
          onChange={(obfuscate) => {
            form.change({ obfuscate });
          }}
          description={t("client:admin.domain_actions.obfuscate.description")}
        >
          {t("client:admin.domain_actions.obfuscate")}
        </Checkbox>
        {form.confirmation === null && form.error !== undefined && (
          <InlineError alert size="inherit">
            {t("client:admin.domain_actions.issue_failed", { reason: form.error })}
          </InlineError>
        )}
        <Box>
          <Button type="submit" variant="primary" isPending={form.isPending && form.confirmation === null}>
            {t("client:admin.domain_actions.submit")}
          </Button>
        </Box>
        <StatusMessage>{form.status}</StatusMessage>
      </Stack>
      {/* Outside the form: React sends the dialog's own submit event up its component tree, through a portal too. */}
      <ConfirmDialog
        isOpen={form.confirmation !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) form.confirmation?.cancel();
        }}
        title={t("client:admin.domain_actions.confirm_suspend.title", {
          domain: domainInText(t, form.confirmation?.domain ?? draft.domain),
        })}
        description={t("client:admin.domain_actions.confirm_suspend.description")}
        confirmLabel={t("client:admin.domain_actions.confirm_suspend.confirm")}
        cancelLabel={t("client:admin.domain_actions.confirm_suspend.cancel")}
        onConfirm={() => {
          form.confirmation?.confirm();
        }}
        isPending={form.isPending}
        {...(form.confirmation !== null && form.error !== undefined
          ? { errorMessage: t("client:admin.domain_actions.issue_failed", { reason: form.error }) }
          : {})}
      />
    </>
  );
}
