// SPDX-License-Identifier: AGPL-3.0-only

// A webhook as a holder of `manage_webhooks` keeps it (FR-722, ADR-0060), at /admin/webhooks/{id}: first where it
// stands and what to do with it (Send a test while it is on; Turn off or Turn on), then what it is, and its form saved
// as a whole (its address, what it is for, its events); after the form's Save, apart, the deliveries of the last 14
// days (each its event, where it stands, its attempts and the last one's outcome, and Send again), a new secret (asked
// first, then shown once in the section's place, the previous one still signing for 24 hours), and, last, Delete,
// confirmed with the host typed. The API gives no delivery's payload, so none is shown.

import {
  useAdminWebhook,
  useAdminWebhookWrites,
  usePermissions,
  useWebhookDeliveries,
  webhookEvents,
  type AdminWebhook,
  type WebhookChange,
  type WebhookDelivery,
} from "@my/app-data";
import {
  Box,
  Button,
  ConfirmDialog,
  ModalPresence,
  MoreRecords,
  Row,
  SettingsEmpty,
  SettingsFacts,
  SettingsList,
  SettingsListRow,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
  StatusSummary,
  type IconName,
  type SettingsRowSaid,
} from "@my/design-system";
import { useState, type ReactNode } from "react";
import { useMoment, useStaffStrings, type T } from "../parts";
import { Irreversible, LeaveDialog, ScreenNotice, useStillThere, type AdminSubProps } from "./parts";
import {
  eventName,
  formOf,
  hostOf,
  useFocusOnArrival,
  useWebhookForm,
  WebhookBar,
  WebhookEventFields,
  WebhookPlaceFields,
  WebhookSecret,
  type WebhookFormState,
} from "./webhook-form";
import { adminWebhooksPath, BackToWebhooks } from "./webhooks";

/** Where a webhook stands, as its summary says it: its glyph, its sentence, and what it means. */
function summaryOf(
  t: T,
  moment: (iso: string) => string,
  w: AdminWebhook,
): { icon: IconName; title: string; about: string } {
  if (w.state === "disabled") {
    if (w.state_reason === "failing")
      return {
        icon: "caution",
        title: t("staff_webhooks.summary.failing_off"),
        about: t("staff_webhooks.summary.failing_off.about"),
      };
    if (w.state_reason === "gone")
      return {
        icon: "caution",
        title: t("staff_webhooks.summary.gone"),
        about: t("staff_webhooks.summary.gone.about"),
      };
    return {
      icon: "stateDisabled",
      title: t("staff_webhooks.summary.off"),
      about: t("staff_webhooks.summary.off.about"),
    };
  }
  if (w.failure !== undefined)
    return {
      icon: "caution",
      title: t("staff_webhooks.summary.failing"),
      about: t("staff_webhooks.summary.failing.about", {
        count: w.failure.consecutive_failures,
        since: moment(w.failure.first_failed_at),
      }),
    };
  return {
    icon: "accepted",
    title: t("staff_webhooks.summary.on"),
    about: t("staff_webhooks.summary.on.about"),
  };
}

/** Where a webhook stands, with Send a test while it is on, and Turn off or Turn on; what came of them beside. */
function WebhookState({ webhook }: { webhook: AdminWebhook }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const writes = useAdminWebhookWrites();
  const there = useStillThere();
  const [busy, setBusy] = useState<"test" | "switch" | undefined>(undefined);
  const [said, setSaid] = useState<{ text: string; n: number } | undefined>(undefined);
  const say = (text: string) => {
    setSaid((was) => ({ text, n: (was?.n ?? 0) + 1 }));
  };
  const on = webhook.state === "enabled";
  const run = (what: "test" | "switch", write: () => Promise<unknown>, done: string, failed: string) => {
    setBusy(what);
    write().then(
      () => {
        if (!there.current) return;
        setBusy(undefined);
        say(done);
      },
      () => {
        if (!there.current) return;
        setBusy(undefined);
        say(failed);
      },
    );
  };
  const s = summaryOf(t, moment, webhook);
  return (
    <StatusSummary
      icon={s.icon}
      title={s.title}
      about={s.about}
      action={
        <Row wrap align="center" gap="screen-row-gap">
          {on && (
            <Button
              size="timeline"
              touch
              busy={busy === "test"}
              isDisabled={busy !== undefined}
              onPress={() => {
                run(
                  "test",
                  () => writes.test(webhook.id),
                  t("staff_webhooks.test.sent"),
                  t("staff_webhooks.test.failed"),
                );
              }}
            >
              {t("staff_webhooks.test")}
            </Button>
          )}
          <Button
            size="timeline"
            touch
            busy={busy === "switch"}
            isDisabled={busy !== undefined}
            onPress={() => {
              run(
                "switch",
                () => (on ? writes.disable(webhook.id) : writes.enable(webhook.id)),
                t(on ? "staff_webhooks.turned_off" : "staff_webhooks.turned_on"),
                t("staff_webhooks.switch.failed"),
              );
            }}
          >
            {t(on ? "staff_webhooks.turn_off" : "staff_webhooks.turn_on")}
          </Button>
        </Row>
      }
      result={said === undefined ? null : <span key={said.n}>{said.text}</span>}
    />
  );
}

/** What a webhook is: its whole address, when it was set up, and when its address or events last changed. */
function WebhookFacts({ webhook }: { webhook: AdminWebhook }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  return (
    <SettingsSection heading={t("staff_webhooks.facts")}>
      <SettingsFacts
        label={t("staff_webhooks.facts")}
        items={[
          { text: t("staff_webhooks.url"), about: [webhook.url] },
          {
            text: t("staff_webhooks.facts.created"),
            about: [moment(webhook.created_at)],
          },
          {
            text: t("staff_webhooks.facts.revised"),
            about: [moment(webhook.revised_at)],
          },
        ]}
      />
    </SettingsSection>
  );
}

/** The outcome of a delivery's last attempt, in words: what came back and when. */
function attemptOf(t: T, moment: (iso: string) => string, d: WebhookDelivery): string | undefined {
  const a = d.last_attempt;
  if (a === undefined) return undefined;
  return t("staff_webhooks.delivery.attempt", {
    outcome: a.outcome,
    code: a.status_code ?? 0,
    at: moment(a.attempted_at),
  });
}

/** The deliveries of the last 14 days, newest first, each with Send again; Show more past the first page. */
function Deliveries({ webhook }: { webhook: AdminWebhook }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const list = useWebhookDeliveries(webhook.id);
  const writes = useAdminWebhookWrites();
  const there = useStillThere();
  const [busy, setBusy] = useState<string | undefined>(undefined);
  const [said, setSaid] = useState<Readonly<Record<string, SettingsRowSaid>>>({});
  const heading = t("staff_webhooks.deliveries");
  const sayOn = (id: string, text: string) => {
    setSaid((was) => ({ ...was, [id]: { text, n: (was[id]?.n ?? 0) + 1 } }));
  };
  if (list.status !== "success" && list.items.length === 0)
    return (
      <SettingsSection heading={heading} about={t("staff_webhooks.deliveries.about")}>
        <ScreenNotice
          status={list.status === "error" ? "failed" : "loading"}
          retry={list.refetch}
          words={{
            loading: t("staff_webhooks.deliveries.loading"),
            failed: t("staff_webhooks.deliveries.failed"),
          }}
        />
      </SettingsSection>
    );
  return (
    <SettingsSection heading={heading} about={t("staff_webhooks.deliveries.about")}>
      {list.items.length === 0 ? (
        <SettingsEmpty>{t("staff_webhooks.deliveries.empty")}</SettingsEmpty>
      ) : (
        <SettingsList label={heading}>
          {list.items.map((d) => {
            const attempt = attemptOf(t, moment, d);
            return (
              <SettingsListRow
                key={d.id}
                title={eventName(t, d.event_type)}
                facts={[
                  moment(d.created_at),
                  t("staff_webhooks.delivery.attempts", { count: d.attempts }),
                  ...(attempt === undefined ? [] : [attempt]),
                ]}
                state={t("staff_webhooks.delivery.status", {
                  status: d.status,
                })}
                said={said[d.id] ?? null}
                end={
                  <Button
                    size="timeline"
                    touch
                    busy={busy === d.id}
                    isDisabled={busy !== undefined}
                    onPress={() => {
                      setBusy(d.id);
                      writes.redeliver(webhook.id, d.id).then(
                        () => {
                          if (!there.current) return;
                          setBusy(undefined);
                          sayOn(d.id, t("staff_webhooks.delivery.again"));
                        },
                        () => {
                          if (!there.current) return;
                          setBusy(undefined);
                          sayOn(d.id, t("staff_webhooks.delivery.again_failed"));
                        },
                      );
                    }}
                  >
                    {t("staff_webhooks.delivery.redeliver")}
                  </Button>
                }
              />
            );
          })}
        </SettingsList>
      )}
      {list.failedNextPage && <SettingsResult>{t("staff_webhooks.deliveries.more_failed")}</SettingsResult>}
      {list.hasNextPage && <MoreRecords onPress={list.fetchNextPage} busy={list.isFetchingNextPage} />}
    </SettingsSection>
  );
}

/** A new secret: asked first, then shown once in the section's place, until the reader is done with it. */
function NewSecret({ webhook, shown }: { webhook: AdminWebhook; shown?: string | undefined }) {
  const { t } = useStaffStrings();
  const writes = useAdminWebhookWrites();
  const there = useStillThere();
  const [asking, setAsking] = useState(false);
  const [sending, setSending] = useState(false);
  const [failed, setFailed] = useState(false);
  const [secret, setSecret] = useState<string | undefined>(shown);
  const titleRef = useFocusOnArrival<HTMLHeadingElement>(secret !== undefined);
  const rotate = () => {
    setSending(true);
    writes.rotate(webhook.id).then(
      (s) => {
        if (!there.current) return;
        setSending(false);
        setAsking(false);
        setFailed(false);
        setSecret(s);
      },
      () => {
        if (!there.current) return;
        setSending(false);
        setAsking(false);
        setFailed(true);
      },
    );
  };
  if (secret !== undefined)
    return (
      <WebhookSecret
        kind="rotated"
        secret={secret}
        titleRef={titleRef}
        done={
          <Row justify="end">
            <Button
              size="timeline"
              touch
              onPress={() => {
                setSecret(undefined);
              }}
            >
              {t("staff_webhooks.secret.done")}
            </Button>
          </Row>
        }
      />
    );
  return (
    <>
      <SettingsSection heading={t("staff_webhooks.rotate")} about={t("staff_webhooks.rotate.about")}>
        <Box>
          <Button
            size="timeline"
            touch
            onPress={() => {
              setAsking(true);
            }}
          >
            {t("staff_webhooks.rotate.press")}
          </Button>
        </Box>
        <SettingsResult>{failed ? t("staff_webhooks.rotate.failed") : undefined}</SettingsResult>
      </SettingsSection>
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            key="rotate"
            title={t("staff_webhooks.rotate.title")}
            cancel={t("staff_webhooks.rotate.cancel")}
            confirm={t("staff_webhooks.rotate.press")}
            busy={sending}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={rotate}
          >
            {t("staff_webhooks.rotate.text")}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </>
  );
}

/** What a change of the form sends: only what changed, an emptied description sent empty, which removes it. */
function changeOf(f: WebhookFormState): WebhookChange {
  const { form, base } = f;
  const url = form.url.trim();
  const description = form.description.trim();
  const events = webhookEvents.filter((e) => form.events.has(e));
  return {
    ...(url !== base.url.trim() ? { url } : {}),
    ...(description !== base.description.trim() ? { description } : {}),
    ...(events.length !== base.events.size || events.some((e) => !base.events.has(e)) ? { events } : {}),
  };
}

/** The webhook's page, once it is read. */
function WebhookPage({
  webhook,
  go,
  onUnsaved,
  leaving,
  defaultSecret,
}: AdminSubProps & {
  webhook: AdminWebhook;
  defaultSecret?: string | undefined;
}) {
  const { t } = useStaffStrings();
  const writes = useAdminWebhookWrites();
  const permissions = usePermissions();
  const there = useStillThere();
  const f = useWebhookForm(formOf(webhook), permissions, onUnsaved);
  const host = hostOf(webhook.url);
  const [asking, setAsking] = useState(false);
  const [deleting, setDeleting] = useState<"idle" | "sending" | "failed">("idle");
  const save = () => {
    if (f.busy || !f.check()) return;
    const change = changeOf(f);
    if (Object.keys(change).length === 0) {
      go(adminWebhooksPath);
      return;
    }
    f.setSent({ kind: "sending" });
    writes.change(webhook.id, change).then(
      () => {
        if (!there.current) return;
        f.setBase(f.form);
        f.setSent({ kind: "saved" });
      },
      (error: unknown) => {
        if (there.current) f.refused(error);
      },
    );
  };
  const remove = () => {
    setDeleting("sending");
    writes.remove(webhook.id).then(
      () => {
        if (!there.current) return;
        setAsking(false);
        go(adminWebhooksPath);
      },
      () => {
        if (!there.current) return;
        setAsking(false);
        setDeleting("failed");
      },
    );
  };
  return (
    <>
      <SettingsScreen
        sub
        title={host}
        back={<BackToWebhooks go={go} />}
        // What the form does not hold stands after its Save, apart, and what ends the webhook last.
        foot={
          <>
            <WebhookBar f={f} onSave={save} label={t("staff_webhooks.save")} />
            <Deliveries webhook={webhook} />
            <NewSecret webhook={webhook} shown={defaultSecret} />
            <SettingsSection heading={t("staff_webhooks.delete")} about={t("staff_webhooks.delete.about")}>
              <Irreversible>{t("staff_webhooks.irreversible")}</Irreversible>
              <Box>
                <Button
                  size="timeline"
                  touch
                  isDisabled={f.busy}
                  onPress={() => {
                    setAsking(true);
                  }}
                >
                  {t("staff_webhooks.delete.press")}
                </Button>
              </Box>
              <SettingsResult>{deleting === "failed" ? t("staff_webhooks.delete.failed") : undefined}</SettingsResult>
            </SettingsSection>
          </>
        }
      >
        <WebhookState webhook={webhook} />
        <WebhookFacts webhook={webhook} />
        <WebhookPlaceFields f={f} />
        <WebhookEventFields f={f} />
      </SettingsScreen>
      <ModalPresence>
        {asking ? (
          <ConfirmDialog
            key="delete"
            title={t("staff_webhooks.delete.title", { host })}
            cancel={t("staff_webhooks.delete.cancel")}
            confirm={t("staff_webhooks.delete.press")}
            phrase={host}
            busy={deleting === "sending"}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={remove}
          >
            {t("staff_webhooks.delete.text")}
          </ConfirmDialog>
        ) : (
          leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />
        )}
      </ModalPresence>
    </>
  );
}

/**
 * The page of the webhook of `id`, or that it loads, could not be read, or is none. `defaultSecret` opens it with a
 * new secret just made (a specimen's), shown once in its section's place.
 */
export function WebhookScreen({
  id,
  defaultSecret,
  ...sub
}: AdminSubProps & {
  id: string;
  defaultSecret?: string | undefined;
}): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminWebhook(id);
  if (read.webhook === undefined)
    return (
      <SettingsScreen
        sub
        title={t("staff_webhooks.entry")}
        back={<BackToWebhooks go={sub.go} />}
        notice={
          <ScreenNotice
            status={read.missing ? "missing" : read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{
              loading: t("staff_webhooks.one.loading"),
              failed: t("staff_webhooks.one.failed"),
              missing: t("staff_webhooks.missing"),
            }}
            all={{ label: t("staff_webhooks.all"), href: adminWebhooksPath }}
          />
        }
      />
    );
  return <WebhookPage key={read.webhook.id} webhook={read.webhook} defaultSecret={defaultSecret} {...sub} />;
}
