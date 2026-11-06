// SPDX-License-Identifier: AGPL-3.0-only
// The form of a new sign-up block (FR-719), the page /moderation/signup-blocks/new below its heading. The form
// follows the browser's constraint validation (React Aria's native validation); a block takes effect at once and is
// lifted as easily from its page, so the form does not ask first. What adding did stays under the form in a
// StatusMessage (role="status"), and the form empties but for the kind, which the next block often shares.
import {
  Box,
  Button,
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
import {
  signupBlockCommentLimit,
  type SignupBlockKind,
  signupBlockKinds,
  signupBlockSeverities,
  signupBlockValueLimit,
} from "../api/signupBlocks";
import { signupBlockKindLabels, signupBlockSeverityLabels, useSignupBlockForm } from "../hooks/useSignupBlocks";
import { RequirePermission, Section } from "./parts";
import {
  day,
  signupBlockDurations,
  signupBlockKindDescriptions,
  signupBlockSeverityDescriptions,
} from "./signupBlockFields";

/** The label, help and error of the value field for each kind. */
const valueFields: Readonly<
  Record<SignupBlockKind, { label: ClientMessageKey; description: ClientMessageKey; required: ClientMessageKey }>
> = {
  email_domain: {
    label: "client:admin.signup_blocks.value.email_domain",
    description: "client:admin.signup_blocks.value.email_domain.description",
    required: "client:admin.signup_blocks.value.email_domain.required",
  },
  email_hash: {
    label: "client:admin.signup_blocks.value.email_hash",
    description: "client:admin.signup_blocks.value.email_hash.description",
    required: "client:admin.signup_blocks.value.email_hash.required",
  },
  ip_range: {
    label: "client:admin.signup_blocks.value.ip_range",
    description: "client:admin.signup_blocks.value.ip_range.description",
    required: "client:admin.signup_blocks.value.ip_range.required",
  },
};

export function NewSignupBlock() {
  const t = useT();
  return (
    <RequirePermission permission="manage_accounts">
      <Section surface="card" title={t("client:admin.signup_blocks.new.form")}>
        <NewSignupBlockForm />
      </Section>
    </RequirePermission>
  );
}

function NewSignupBlockForm() {
  const t = useT();
  const form = useSignupBlockForm();
  const { draft } = form;
  const valueField = valueFields[draft.kind];
  return (
    <Stack as="form" onSubmit={form.submit} gap="6">
      <RadioGroup
        label={t("client:admin.signup_blocks.kind")}
        value={draft.kind}
        onChange={(kind) => {
          const chosen = signupBlockKinds.find((item) => item === kind);
          if (chosen !== undefined) form.change({ kind: chosen });
        }}
        isRequired
      >
        {signupBlockKinds.map((kind) => (
          <Radio key={kind} value={kind} description={t(signupBlockKindDescriptions[kind])}>
            {t(signupBlockKindLabels[kind])}
          </Radio>
        ))}
      </RadioGroup>
      <TextField
        label={t(valueField.label)}
        description={t(valueField.description)}
        value={draft.value}
        onChange={(value) => {
          form.change({ value });
        }}
        isRequired
        maxLength={signupBlockValueLimit}
        autoComplete="off"
        spellCheck="false"
        errorMessage={t(valueField.required)}
      />
      <RadioGroup
        label={t("client:admin.signup_blocks.severity")}
        value={draft.severity}
        onChange={(severity) => {
          const chosen = signupBlockSeverities.find((item) => item === severity);
          if (chosen !== undefined) form.change({ severity: chosen });
        }}
        isRequired
        errorMessage={t("client:admin.signup_blocks.severity.required")}
      >
        {signupBlockSeverities.map((severity) => (
          <Radio key={severity} value={severity} description={t(signupBlockSeverityDescriptions[severity])}>
            {t(signupBlockSeverityLabels[severity])}
          </Radio>
        ))}
      </RadioGroup>
      <DurationSelect
        label={t("client:admin.signup_blocks.duration")}
        description={t("client:admin.signup_blocks.duration.description")}
        durations={signupBlockDurations}
        formatDuration={(seconds) => t("client:admin.signup_blocks.duration.days", { count: seconds / day })}
        indefiniteLabel={t("client:admin.signup_blocks.duration.indefinite")}
        value={draft.expiresIn}
        onChange={(expiresIn) => {
          form.change({ expiresIn });
        }}
      />
      <ReasonField
        label={t("client:admin.signup_blocks.comment")}
        description={t("client:admin.signup_blocks.comment.description")}
        value={draft.comment}
        onChange={(comment) => {
          form.change({ comment });
        }}
        characterLimit={signupBlockCommentLimit}
        formatRemaining={(remaining) =>
          remaining < 0
            ? t("client:admin.signup_blocks.characters_over", { count: -remaining })
            : t("client:admin.signup_blocks.characters_left", { count: remaining })
        }
        errorMessage={t("client:admin.signup_blocks.too_long", { max: signupBlockCommentLimit })}
      />
      {form.error !== undefined && (
        <InlineError alert size="inherit">
          {t("client:admin.signup_blocks.add_failed", { reason: form.error })}
        </InlineError>
      )}
      <Box>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.signup_blocks.submit")}
        </Button>
      </Box>
      <StatusMessage>{form.status}</StatusMessage>
    </Stack>
  );
}
