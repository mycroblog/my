// SPDX-License-Identifier: AGPL-3.0-only

// The supporter program as a holder of `manage_supporters` keeps it, at /admin/supporters (ADR-0072, ADR-0162;
// FR-910, FR-914, FR-918, FR-919): the doors to the plans and the subscriptions; the program, on or off, with what it
// needs, the payment provider of the configuration (which no screen changes) and what to look at; the commerce
// disclosure, published or not, and where it is written; who may read the supporter list, which a holder of
// `manage_settings` changes; the tips, their amounts and what the provider allows; and the payment provider's state,
// with a reconciliation run now. What a press did is said in the screen's status, which takes the focus.

import {
  amountFits,
  loadAdminPlans,
  loadAdminSettings,
  loadSupporterProgram,
  programRefusal,
  useAdminPlans,
  useAdminSettings,
  useInstance,
  usePaymentStatus,
  usePermissions,
  useProgramActions,
  useSaveAdminSettings,
  useSupporterProgram,
  useSupporterTips,
  viewerQuery,
  type AppData,
  type PaymentStatus,
  type SupporterProgram,
  type SupporterTips,
} from "@my/app-data";
import {
  Box,
  Button,
  CaptionLine,
  CheckField,
  FactList,
  FeedNotice,
  FormAlert,
  Row,
  ScreenStack,
  ScreenTitle,
  Select,
  SettingsGroup,
  SettingsItem,
  SettingsMenu,
  Stack,
  StateMark,
  TextField,
  TextLink,
  type Fact,
} from "@my/design-system";
import { useId, useState, type ReactNode } from "react";
import { moneyText } from "../../finance-totals";
import { Field, ReadNotice, Titled, useMoment, useStaffStrings, useSaid, type T } from "../parts";
import { supporterCurrencies, supportersPaths, tipAmountsOf } from "./parts";

type Say = (words: string, alert?: boolean) => void;

/** The first page of Supporters. */
export function SupportersAdminScreen(): ReactNode {
  const { t } = useStaffStrings();
  const program = useSupporterProgram();
  const { say, status } = useSaid();
  const title = <ScreenTitle>{t("staff_supporters.entry")}</ScreenTitle>;
  if (program.forbidden) return <FeedNotice lead>{t("staff_supporters.forbidden")}</FeedNotice>;
  if (program.value === undefined)
    return (
      <ScreenStack>
        {title}
        <ReadNotice
          status={program.status === "error" ? "error" : "pending"}
          loading={t("staff_supporters.program.loading")}
          failed={t("staff_supporters.program.failed")}
          retry={program.refetch}
        />
      </ScreenStack>
    );
  return (
    <ScreenStack>
      {title}
      {status}
      <Warnings program={program.value} />
      <Doors />
      <Program program={program.value} say={say} />
      <Disclosure program={program.value} />
      <ListAudience say={say} />
      <Tips say={say} />
      <Payments say={say} />
    </ScreenStack>
  );
}

/**
 * What to look at, first, in the look of a failure: what the program lacks, the payment provider's signs of webhooks
 * not set up right, and a reconciliation that failed.
 */
function Warnings({ program }: { program: SupporterProgram }): ReactNode {
  const { t } = useStaffStrings();
  const payments = usePaymentStatus().value;
  const run = payments?.last_reconciliation;
  const lines = [
    ...program.warnings.map((w) => t("staff_supporters.program.warning", { warning: w })),
    ...(payments?.warnings ?? []).map((w) => t("staff_supporters.payments.warning", { warning: w })),
    ...(run !== undefined && run.outcome === "failed"
      ? [t("staff_supporters.payments.error", { kind: run.error_kind })]
      : []),
  ];
  if (lines.length === 0) return null;
  return (
    <Stack gap="entry-line">
      {lines.map((line) => (
        <StateMark key={line} state="failed">
          {line}
        </StateMark>
      ))}
    </Stack>
  );
}

/** Reads what the first page of Supporters shows, for the route's loader. */
export async function loadSupportersAdmin(data: AppData): Promise<void> {
  const viewer = await data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }).catch(() => undefined);
  await Promise.all([
    loadSupporterProgram(data),
    loadAdminPlans(data),
    viewer?.permissions.includes("manage_settings") === true ? loadAdminSettings(data) : undefined,
  ]);
}

/** The doors to the plans, with how many take subscriptions, and to the subscriptions. */
function Doors() {
  const { t } = useStaffStrings();
  const plans = useAdminPlans().value;
  const live = plans?.filter((p) => !p.retired).length;
  return (
    <SettingsMenu label={t("staff_supporters.doors")}>
      <SettingsGroup heading={t("staff_supporters.doors")}>
        <SettingsItem
          icon="funding"
          label={t("staff_supporters.plans")}
          hint={live === undefined ? undefined : t("staff_supporters.plans.count", { count: live })}
          count={undefined}
          href={supportersPaths.plans}
        />
        <SettingsItem
          icon="members"
          label={t("staff_supporters.subscriptions")}
          hint={t("staff_supporters.subscriptions.hint")}
          count={undefined}
          href={supportersPaths.subscriptions}
        />
      </SettingsGroup>
    </SettingsMenu>
  );
}

/** The program: its state, what it needs, what to look at, and turning it on or off where it can be. */
function Program({ program, say }: { program: SupporterProgram; say: Say }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const { setProgram } = useProgramActions();
  const [busy, setBusy] = useState(false);
  const facts: Fact[] = [
    {
      term: t("staff_supporters.program.state"),
      value:
        program.changed_at === undefined
          ? t(program.enabled ? "staff_supporters.program.on" : "staff_supporters.program.off")
          : t(program.enabled ? "staff_supporters.program.on_since" : "staff_supporters.program.off_since", {
              at: moment(program.changed_at),
            }),
    },
    {
      term: t("staff_supporters.program.configured"),
      value: program.configured_provider ?? t("staff_supporters.program.no_provider"),
    },
    // The provider it was enabled with, only where the configuration names another.
    ...(program.provider === undefined || program.provider === program.configured_provider
      ? []
      : [{ term: t("staff_supporters.program.enabled_with"), value: program.provider }]),
    {
      term: t("staff_supporters.program.email"),
      value: t(program.sends_email ? "staff_supporters.program.email.yes" : "staff_supporters.program.email.no"),
    },
    {
      term: t("staff_supporters.program.live"),
      value: t("staff_supporters.program.live.count", { count: program.live_subscriptions }),
    },
  ];
  const turn = (enabled: boolean) => {
    setBusy(true);
    setProgram(enabled).then(
      () => {
        setBusy(false);
        say(t(enabled ? "staff_supporters.program.done.on" : "staff_supporters.program.done.off"));
      },
      (e: unknown) => {
        setBusy(false);
        const why = programRefusal(e).kind;
        say(
          t(
            why === "unavailable"
              ? "staff_supporters.program.refused.unavailable"
              : why === "live-subscriptions"
                ? "staff_supporters.program.refused.live"
                : "staff_supporters.write_failed",
          ),
          true,
        );
      },
    );
  };
  // What cannot be done now is a sentence, never a disabled button.
  const action = program.enabled ? (
    program.live_subscriptions > 0 ? (
      <CaptionLine icon="caution">{t("staff_supporters.program.cannot_off")}</CaptionLine>
    ) : (
      <Row>
        <Button
          busy={busy}
          onPress={() => {
            turn(false);
          }}
        >
          {t("staff_supporters.program.turn_off")}
        </Button>
      </Row>
    )
  ) : program.can_enable ? (
    <Row>
      <Button
        kind="primary"
        busy={busy}
        onPress={() => {
          turn(true);
        }}
      >
        {t("staff_supporters.program.turn_on")}
      </Button>
    </Row>
  ) : (
    <CaptionLine icon="caution">{t("staff_supporters.program.cannot_on")}</CaptionLine>
  );
  return (
    <Titled title={t("staff_supporters.program")}>
      <Stack gap="entry-room">
        <FactList label={t("staff_supporters.program")} items={facts} />
        {action}
      </Stack>
    </Titled>
  );
}

/** Whether the commerce disclosure is published, the way to read it, and, for who writes it, where. */
function Disclosure({ program }: { program: SupporterProgram }) {
  const { t } = useStaffStrings();
  const may = usePermissions().includes("manage_settings");
  return (
    <Titled title={t("staff_supporters.disclosure")}>
      <Stack gap="entry-line">
        <Box as="p">
          {t(
            program.commerce_disclosure ? "staff_supporters.disclosure.published" : "staff_supporters.disclosure.none",
          )}
        </Box>
        <Row wrap gap="screen-tab-gap">
          {program.commerce_disclosure && (
            <TextLink href="/about/commerce">{t("staff_supporters.disclosure.read")}</TextLink>
          )}
          {may && (
            <TextLink href="/admin/documents/commerce-disclosure">{t("staff_supporters.disclosure.edit")}</TextLink>
          )}
        </Row>
      </Stack>
    </Titled>
  );
}

const audiences = ["off", "signed_in", "everyone"] as const;
type Audience = (typeof audiences)[number];

/**
 * Who may read the supporter list: a choice and Save for a holder of `manage_settings`, else what the server says now
 * as a sentence.
 */
function ListAudience({ say }: { say: Say }) {
  const { t } = useStaffStrings();
  const may = usePermissions().includes("manage_settings");
  const instance = useInstance();
  const settings = useAdminSettings(["supporter_list_visibility"], may);
  const save = useSaveAdminSettings();
  const id = useId();
  const [chosen, setChosen] = useState<Audience | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const stored = audiences.find((a) => a === settings.value?.supporter_list_visibility.value);
  if (!may) {
    const now = instance?.supporter_list_visibility;
    return (
      <Titled title={t("staff_supporters.list")}>
        <FeedNotice start lead>
          {now === undefined ? t("staff_supporters.list.loading") : t("staff_supporters.list.now", { audience: now })}
        </FeedNotice>
      </Titled>
    );
  }
  if (stored === undefined)
    return (
      <Titled title={t("staff_supporters.list")}>
        <ReadNotice
          status={settings.status === "error" ? "error" : "pending"}
          loading={t("staff_supporters.list.loading")}
          failed={t("staff_supporters.list.failed")}
          retry={settings.refetch}
        />
      </Titled>
    );
  const value = chosen ?? stored;
  return (
    <Titled title={t("staff_supporters.list")}>
      <Stack
        gap="screen-field-gap"
        as={
          <form
            onSubmit={(e) => {
              e.preventDefault();
              if (busy) return;
              setBusy(true);
              setError(undefined);
              save({ supporter_list_visibility: value }).then(
                () => {
                  setBusy(false);
                  setChosen(undefined);
                  say(t("staff_supporters.list.done", { audience: value }));
                },
                () => {
                  setBusy(false);
                  setError(t("staff_supporters.write_failed"));
                },
              );
            }}
          />
        }
      >
        <Field id={id} label={t("staff_supporters.list.field")}>
          <Select
            id={id}
            options={audiences.map((a) => ({ value: a, label: t("staff_supporters.list.audience", { audience: a }) }))}
            value={value}
            onChange={(v) => {
              setChosen(audiences.find((a) => a === v));
            }}
          />
        </Field>
        {error !== undefined && <FormAlert regular>{error}</FormAlert>}
        <Row>
          <Button type="submit" busy={busy}>
            {t("staff_supporters.list.save")}
          </Button>
        </Row>
      </Stack>
    </Titled>
  );
}

/** The amounts of `tips` as the field holds them, comma-separated. */
const amountsText = (tips: SupporterTips) => tips.amounts.map((m) => m.amount).join("\n");

/** The tips: what the provider allows and whether users may tip now, and the form that turns them on or off. */
function Tips({ say }: { say: Say }) {
  const { t, locale } = useStaffStrings();
  const read = useSupporterTips();
  if (read.value === undefined)
    return (
      <Titled title={t("staff_supporters.tips")}>
        <ReadNotice
          status={read.status === "error" ? "error" : "pending"}
          loading={t("staff_supporters.tips.loading")}
          failed={t("staff_supporters.tips.failed")}
          retry={read.refetch}
        />
      </Titled>
    );
  const tips = read.value;
  const facts: Fact[] = [
    {
      term: t("staff_supporters.tips.state"),
      value: t(
        tips.accepting
          ? "staff_supporters.tips.accepting"
          : tips.enabled
            ? "staff_supporters.tips.not_accepting"
            : "staff_supporters.tips.off",
      ),
    },
    ...tips.limits.map((l) => ({
      term: t("staff_supporters.tips.limits", { currency: l.minimum.currency }),
      value: t("staff_supporters.tips.range", { min: moneyText(locale, l.minimum), max: moneyText(locale, l.maximum) }),
    })),
  ];
  return (
    <Titled title={t("staff_supporters.tips")}>
      <Stack gap="entry-room">
        <FactList label={t("staff_supporters.tips")} items={facts} />
        {!tips.one_time_payments && (
          <CaptionLine icon="caution" alert>
            {t("staff_supporters.tips.no_one_time")}
          </CaptionLine>
        )}
        {tips.server_limit_reached && (
          <CaptionLine icon="caution" alert>
            {t("staff_supporters.tips.limit_reached")}
          </CaptionLine>
        )}
        <TipsForm key={`${String(tips.enabled)}:${amountsText(tips)}`} tips={tips} say={say} />
      </Stack>
    </Titled>
  );
}

/** Whether the server takes tips, in which currency, and their amounts; Save the tips. */
function TipsForm({ tips, say }: { tips: SupporterTips; say: Say }) {
  const { t } = useStaffStrings();
  const { setTips } = useProgramActions();
  const onId = useId();
  const currencyId = useId();
  const amountsId = useId();
  const [on, setOn] = useState(tips.enabled);
  const [currency, setCurrency] = useState<string>(tips.amounts[0]?.currency ?? "JPY");
  const [amounts, setAmounts] = useState(amountsText(tips));
  const [error, setError] = useState<string | undefined>(undefined);
  const [failed, setFailed] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const send = () => {
    if (busy) return;
    setFailed(undefined);
    const list = tipAmountsOf(amounts);
    if (on && (list.length === 0 || list.some((a) => !amountFits(a, currency)))) {
      setError(t("staff_supporters.tips.amounts.invalid", { currency }));
      document.getElementById(amountsId)?.focus();
      return;
    }
    setError(undefined);
    setBusy(true);
    setTips(on ? list.map((amount) => ({ amount, currency })) : undefined).then(
      () => {
        setBusy(false);
        say(t(on ? "staff_supporters.tips.done.on" : "staff_supporters.tips.done.off"));
      },
      (e: unknown) => {
        setBusy(false);
        const why = programRefusal(e);
        if (why.kind === "amounts") {
          setError(t("staff_supporters.tips.refused", { reason: why.reason ?? "range" }));
          document.getElementById(amountsId)?.focus();
        } else
          setFailed(
            t(why.kind === "unavailable" ? "staff_supporters.tips.unavailable" : "staff_supporters.write_failed"),
          );
      },
    );
  };
  return (
    <Stack
      gap="screen-field-gap"
      as={
        <form
          onSubmit={(e) => {
            e.preventDefault();
            send();
          }}
        />
      }
    >
      <CheckField
        id={onId}
        name="tips"
        value="on"
        label={t("staff_supporters.tips.take")}
        checked={on}
        onChange={setOn}
        touch
      />
      {on && (
        <>
          <Field id={currencyId} label={t("staff_supporters.tips.currency")}>
            <Select
              id={currencyId}
              options={supporterCurrencies.map((c) => ({ value: c, label: c }))}
              value={currency}
              onChange={setCurrency}
            />
          </Field>
          <TextField
            id={amountsId}
            name="amounts"
            label={t("staff_supporters.tips.amounts")}
            hint={error === undefined ? t("staff_supporters.tips.amounts.hint") : undefined}
            defaultValue={amounts}
            multiline
            rows={3}
            error={error}
            onChange={(v) => {
              setAmounts(v);
              setError(undefined);
            }}
          />
        </>
      )}
      {failed !== undefined && <FormAlert regular>{failed}</FormAlert>}
      <Row>
        <Button type="submit" busy={busy}>
          {t("staff_supporters.tips.save")}
        </Button>
      </Row>
    </Stack>
  );
}

/** The facts of the payment provider's state. */
function paymentFacts(t: T, moment: (iso: string) => string, s: PaymentStatus): Fact[] {
  const run = s.last_reconciliation;
  return [
    { term: t("staff_supporters.payments.mode"), value: t("staff_supporters.payments.mode.value", { mode: s.mode }) },
    { term: t("staff_supporters.payments.zone"), value: s.time_zone },
    {
      term: t("staff_supporters.payments.webhook"),
      value: s.last_webhook_at === undefined ? t("staff_supporters.payments.webhook.none") : moment(s.last_webhook_at),
    },
    {
      term: t("staff_supporters.payments.reconciliation"),
      value: (() => {
        if (run === undefined) return t("staff_supporters.payments.reconciliation.none");
        const words = t("staff_supporters.payments.reconciliation.run", {
          outcome: run.outcome,
          at: moment(run.occurred_at),
          checked: run.checked_subscriptions,
          corrected: run.corrected_subscriptions,
        });
        return <StateMark state={run.outcome === "failed" ? "failed" : "active"}>{words}</StateMark>;
      })(),
    },
  ];
}

/** The payment provider of the configuration and its state, with a reconciliation now; or that none is configured. */
function Payments({ say }: { say: Say }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const read = usePaymentStatus();
  const { reconcile } = useProgramActions();
  const [busy, setBusy] = useState(false);
  let body: ReactNode;
  if (read.missing)
    body = (
      <FeedNotice start lead>
        {t("staff_supporters.payments.none")}
      </FeedNotice>
    );
  else if (read.value === undefined)
    body = (
      <ReadNotice
        status={read.status === "error" ? "error" : "pending"}
        loading={t("staff_supporters.payments.loading")}
        failed={t("staff_supporters.payments.failed")}
        retry={read.refetch}
      />
    );
  else
    body = (
      <Stack gap="entry-room">
        <FactList label={t("staff_supporters.payments")} items={paymentFacts(t, moment, read.value)} />
        <Row>
          <Button
            busy={busy}
            onPress={() => {
              setBusy(true);
              reconcile().then(
                (run) => {
                  setBusy(false);
                  say(
                    t(run === "queued" ? "staff_supporters.payments.done" : "staff_supporters.payments.too_soon"),
                    run !== "queued",
                  );
                },
                () => {
                  setBusy(false);
                  say(t("staff_supporters.write_failed"), true);
                },
              );
            }}
          >
            {t("staff_supporters.payments.reconcile")}
          </Button>
        </Row>
      </Stack>
    );
  return <Titled title={t("staff_supporters.payments")}>{body}</Titled>;
}
