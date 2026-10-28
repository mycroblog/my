// SPDX-License-Identifier: AGPL-3.0-only
// The service closure screen below its heading (manage_service_closure, FR-808, FR-809, ADR-0088). With no closure
// announced, the form announces it: the closing date, until when the users may export their data (30 days after the
// closing date when left empty) and a message. While it is announced, the screen shows the announcement, warns when it
// gives the users less than 30 days, changes its dates, withdraws it (an alert dialog), and checks what closing ends
// (the deliveries of the `Delete`s, the supporter subscriptions, the account migrations that have not ended, the
// retention holds) before the server is closed. Closing cannot be undone, so it asks for the server's domain in an
// alert dialog (ConfirmDialog, WAI-ARIA APG alert dialog; NFR-MOD-7). Once closed, the screen shows the progress of the
// deliveries, read again every few seconds. What a change did stays on the page in a StatusMessage; a failure
// stays in the form or the dialog that ran it (role="alert", web-client/README.md E2).
import {
  Button,
  Callout,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  DialogTrigger,
  Heading,
  Hint,
  InlineError,
  ProgressMeter,
  Stack,
  StatusMessage,
  Text,
  TextArea,
  TextField,
} from "@my/ui";
import { useEffect, useState } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { DateTime, Section, RequirePermission } from "../../moderation";
import { characterCount } from "../hooks/languages";
import {
  type AdminServiceClosure,
  type AdminServiceClosureProgress,
  closureMessageLimit,
  shortNoticeDays,
} from "../api/closure";
import {
  isServerDomain,
  useAdminClosure,
  useAnnounceForm,
  useCloseService,
  useClosureCheck,
  useWithdrawClosure,
} from "../hooks/useServiceClosure";

export function ServiceClosureSettings() {
  // The permission first: a viewer without it does not ask the server for the closure, as the route's loader does not.
  return (
    <RequirePermission permission="manage_service_closure">
      <ServiceClosure />
    </RequirePermission>
  );
}

function ServiceClosure() {
  const t = useT();
  const closure = useAdminClosure();
  const [status, setStatus] = useState<string | undefined>(undefined);
  // A change (announcing, withdrawing, closing) may take the pressed button off the page with the state it was for:
  // focus then moves to the page's heading, once the dialog that ran it is gone.
  useEffect(() => {
    if (status === undefined) return undefined;
    const timer = setTimeout(() => {
      const active = document.activeElement;
      if (active === null || active === document.body) document.querySelector<HTMLElement>("main h1")?.focus();
    }, 100);
    return () => {
      clearTimeout(timer);
    };
  }, [status]);
  return (
    <Stack gap="8">
      <Hint size="inherit">{t("client:admin.closure.description")}</Hint>
      {/* In the page from the first render, empty, so that what a change did is announced. */}
      <StatusMessage>{status}</StatusMessage>
      {closure.state === "none" && (
        <Section surface="card" title={t("client:admin.closure.announce.title")}>
          <AnnounceForm closure={closure} onDone={setStatus} />
        </Section>
      )}
      {closure.state === "announced" && <Announced closure={closure} onDone={setStatus} />}
      {(closure.state === "closed" || closure.state === "delivery_completed") && <Closed closure={closure} />}
    </Stack>
  );
}

/** The announcement in effect, its change and withdrawal, and the check and the closing. */
function Announced({ closure, onDone }: { closure: AdminServiceClosure; onDone: (status: string) => void }) {
  const t = useT();
  // Whether the closing date has passed, when the screen was drawn: only then can the server be closed.
  const [now] = useState(() => Date.now());
  const due = closure.closes_at !== undefined && new Date(closure.closes_at).getTime() <= now;
  return (
    <>
      <Section surface="card" title={t("client:admin.closure.current.title")}>
        <Stack gap="4">
          <Schedule closure={closure} />
          {closure.short_notice && <ShortNotice />}
          {due && <Callout>{t("client:admin.closure.due")}</Callout>}
          <Withdraw onDone={onDone} />
        </Stack>
      </Section>
      <Section surface="card" title={t("client:admin.closure.change.title")}>
        <AnnounceForm closure={closure} onDone={onDone} />
      </Section>
      <Section surface="card" title={t("client:admin.closure.close.title")}>
        <Stack gap="4">
          <Hint size="inherit">{t("client:admin.closure.close.description")}</Hint>
          <ClosureCheck />
          <CloseService due={due} closesAt={closure.closes_at} onDone={onDone} />
        </Stack>
      </Section>
    </>
  );
}

function ShortNotice() {
  const t = useT();
  return <Callout>{t("client:admin.closure.short_notice", { days: String(shortNoticeDays) })}</Callout>;
}

/** The dates and the message of the announcement in effect, or of the closed server. */
function Schedule({ closure }: { closure: AdminServiceClosure }) {
  const t = useT();
  return (
    <DescriptionList>
      {closure.closes_at !== undefined && (
        <DescriptionItem term={t("client:admin.closure.closes_at")}>
          <DateTime value={closure.closes_at} />
        </DescriptionItem>
      )}
      {closure.export_until !== undefined && (
        <DescriptionItem term={t("client:admin.closure.export_until")}>
          <DateTime value={closure.export_until} />
        </DescriptionItem>
      )}
      {closure.announced_at !== undefined && (
        <DescriptionItem term={t("client:admin.closure.announced_at")}>
          <DateTime value={closure.announced_at} />
        </DescriptionItem>
      )}
      {closure.message !== undefined && closure.message !== "" && (
        // Written by an administrator in any language, with line breaks.
        <DescriptionItem term={t("client:admin.closure.message")}>
          <Text dir="auto" wrap="anywhere" whitespace="preWrap">
            {closure.message}
          </Text>
        </DescriptionItem>
      )}
    </DescriptionList>
  );
}

/** The form that announces the closure or changes its dates. */
function AnnounceForm({ closure, onDone }: { closure: AdminServiceClosure; onDone: (status: string) => void }) {
  const t = useT();
  const form = useAnnounceForm(closure, onDone);
  const remaining = closureMessageLimit - characterCount(form.message);
  return (
    <Stack as="form" gap="6" onSubmit={form.submit} noValidate>
      <TextField
        type="datetime-local"
        label={t("client:admin.closure.closes_at")}
        description={t("client:admin.closure.closes_at.description")}
        value={form.closesAt}
        onChange={form.changeClosesAt}
        isRequired
        {...(form.errors.closesAt !== undefined && { isInvalid: true, errorMessage: form.errors.closesAt })}
      />
      {form.shortNotice && <ShortNotice />}
      <TextField
        type="datetime-local"
        label={t("client:admin.closure.export_until")}
        description={t("client:admin.closure.export_until.description", { days: String(shortNoticeDays) })}
        value={form.exportUntil}
        onChange={form.changeExportUntil}
        {...(form.errors.exportUntil !== undefined && { isInvalid: true, errorMessage: form.errors.exportUntil })}
      />
      <TextArea
        label={t("client:admin.closure.message")}
        description={t("client:admin.closure.message.description")}
        value={form.message}
        onChange={form.changeMessage}
        rows={5}
        characterLimit={closureMessageLimit}
        formatRemaining={() =>
          remaining < 0
            ? t("client:admin.settings.characters_over", { count: -remaining })
            : t("client:admin.settings.characters_left", { count: remaining })
        }
        {...(form.errors.message !== undefined && { isInvalid: true, errorMessage: form.errors.message })}
      />
      {form.failure !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.closure.announce_failed", { reason: form.failure })}
        </InlineError>
      )}
      <div>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t(form.isChange ? "client:admin.closure.change" : "client:admin.closure.announce")}
        </Button>
      </div>
    </Stack>
  );
}

/** Withdrawing the announcement, after an alert dialog. */
function Withdraw({ onDone }: { onDone: (status: string) => void }) {
  const t = useT();
  const action = useWithdrawClosure(onDone);
  return (
    <div>
      <DialogTrigger
        onOpenChange={(isOpen) => {
          if (!isOpen) action.clearError();
        }}
      >
        <Button variant="danger">{t("client:admin.closure.withdraw")}</Button>
        <ConfirmDialog
          title={t("client:admin.closure.withdraw.title")}
          description={t("client:admin.closure.withdraw.description")}
          confirmLabel={t("client:admin.closure.withdraw")}
          cancelLabel={t("client:admin.closure.cancel")}
          isPending={action.isPending}
          {...(action.error !== undefined && {
            errorMessage: t("client:admin.closure.withdraw_failed", { reason: action.error }),
          })}
          onConfirm={(close) => {
            void action.withdraw().then((withdrawn) => {
              if (withdrawn) close();
            });
          }}
        />
      </DialogTrigger>
    </div>
  );
}

/** What closing the server would end: shown before the confirmation (ADR-0088 §1). */
function ClosureCheck() {
  const t = useT();
  const intl = useIntl();
  const check = useClosureCheck();
  const count = (value: number) => intl.formatNumber(value);
  return (
    <Stack gap="3">
      <Heading level={3}>{t("client:admin.closure.check.title")}</Heading>
      <DescriptionList>
        <DescriptionItem term={t("client:admin.closure.check.deletes")}>
          {t("client:admin.closure.check.deletes.value", {
            deletes: count(check.estimated_deletes),
            accounts: count(check.local_accounts),
            inboxes: count(check.delivery_inboxes),
          })}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.closure.check.subscriptions")}>
          {count(check.live_supporter_subscriptions)}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.closure.check.migrations")}>
          {count(check.pending_migrations)}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.closure.check.holds")}>{count(check.retention_holds)}</DescriptionItem>
      </DescriptionList>
      {check.live_supporter_subscriptions > 0 && (
        <Callout>{t("client:admin.closure.check.subscriptions.warning")}</Callout>
      )}
      {check.pending_migrations > 0 && <Callout>{t("client:admin.closure.check.migrations.warning")}</Callout>}
      {check.retention_holds > 0 && <Callout>{t("client:admin.closure.check.holds.warning")}</Callout>}
      <Hint size="inherit">{t("client:admin.closure.check.federation")}</Hint>
    </Stack>
  );
}

/** Closing the server for good, after an alert dialog that asks for the server's domain. */
function CloseService({
  due,
  closesAt,
  onDone,
}: {
  due: boolean;
  closesAt: string | undefined;
  onDone: (status: string) => void;
}) {
  const t = useT();
  const intl = useIntl();
  const action = useCloseService(onDone);
  return (
    <Stack gap="2">
      {!due && closesAt !== undefined && (
        <Hint size="inherit">
          {t("client:admin.closure.close.not_yet", {
            date: intl.formatDate(closesAt, { dateStyle: "medium", timeStyle: "short" }),
          })}
        </Hint>
      )}
      <div>
        <DialogTrigger
          onOpenChange={(isOpen) => {
            if (!isOpen) action.reset();
          }}
        >
          <Button variant="danger" isDisabled={!due}>
            {t("client:admin.closure.close")}
          </Button>
          <ConfirmDialog
            title={t("client:admin.closure.close.confirm.title")}
            description={t("client:admin.closure.close.confirm.description", { domain: action.domain })}
            confirmLabel={t("client:admin.closure.close.confirm")}
            cancelLabel={t("client:admin.closure.cancel")}
            isPending={action.isPending}
            {...(action.error !== undefined && {
              errorMessage: t("client:admin.closure.close_failed", { reason: action.error }),
            })}
            onConfirm={(close) => {
              void action.close().then((closed) => {
                if (closed) close();
              });
            }}
          >
            <TextField
              label={t("client:admin.closure.close.domain")}
              description={t("client:admin.closure.close.domain.description", { domain: action.domain })}
              value={action.typed}
              onChange={action.changeTyped}
              isRequired
              autoComplete="off"
              dir="ltr"
              validate={(value) =>
                isServerDomain(value, action.domain) ? null : t("client:admin.closure.close.domain.mismatch")
              }
            />
          </ConfirmDialog>
        </DialogTrigger>
      </div>
    </Stack>
  );
}

/** The closed server: its schedule and the progress of the deliveries of the `Delete`s. */
function Closed({ closure }: { closure: AdminServiceClosure }) {
  const t = useT();
  const completed = closure.state === "delivery_completed";
  return (
    <>
      <Section surface="card" title={t("client:admin.closure.closed.title")}>
        <Schedule closure={closure} />
      </Section>
      <Section surface="card" title={t("client:admin.closure.progress.title")}>
        <Stack gap="4">
          <Callout>
            {t(completed ? "client:admin.closure.progress.completed" : "client:admin.closure.progress.running")}
          </Callout>
          {closure.progress === undefined ? (
            <Hint size="inherit">{t("client:admin.closure.progress.none")}</Hint>
          ) : (
            <Progress progress={closure.progress} />
          )}
        </Stack>
      </Section>
    </>
  );
}

/**
 * The deliveries as a progress bar (ADR-0113): the finished ones (delivered, refused for good, skipped or failed) of all
 * the recorded ones; the total grows while inboxes are still being computed. Then the counts behind it.
 */
function Progress({ progress }: { progress: AdminServiceClosureProgress }) {
  const t = useT();
  const intl = useIntl();
  const count = (value: number) => intl.formatNumber(value);
  const { deliveries } = progress;
  const done = deliveries.succeeded + deliveries.permanent_failure + deliveries.skipped + deliveries.dead;
  const total = done + deliveries.pending;
  return (
    // Stacked rather than side by side: a description list is its own size container, so it has no width of its
    // own to give a flex row and would shrink to nothing there.
    <Stack gap="4">
      <ProgressMeter
        kind="progress"
        label={t("client:admin.closure.progress.meter")}
        value={done}
        maxValue={Math.max(total, 1)}
        valueText={t("client:admin.closure.progress.meter.value", { done, total })}
      />
      <DescriptionList columns={2}>
        <DescriptionItem term={t("client:admin.closure.progress.accounts")}>{count(progress.accounts)}</DescriptionItem>
        <DescriptionItem term={t("client:admin.closure.progress.fanouts_pending")}>
          {count(progress.fanouts_pending)}
        </DescriptionItem>
      </DescriptionList>
      <DescriptionList columns={3}>
        <DescriptionItem term={t("client:admin.closure.progress.pending")}>{count(deliveries.pending)}</DescriptionItem>
        <DescriptionItem term={t("client:admin.closure.progress.succeeded")}>
          {count(deliveries.succeeded)}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.closure.progress.permanent_failure")}>
          {count(deliveries.permanent_failure)}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.closure.progress.skipped")}>{count(deliveries.skipped)}</DescriptionItem>
        <DescriptionItem term={t("client:admin.closure.progress.dead")}>{count(deliveries.dead)}</DescriptionItem>
      </DescriptionList>
    </Stack>
  );
}
