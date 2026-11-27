// SPDX-License-Identifier: AGPL-3.0-only

// Supporter, the entry of the settings at /settings/supporter while the server takes the readers' support
// (ADR-0072 §6–§8, ADR-0162, ADR-0070 §13; FR-911, FR-914, FR-918): the reader's subscription,
// its plan and period and where it stands (renewing, ending after a cancel, a change scheduled for the period's end,
// a payment not made yet, a failed payment in its grace or past it), what they may do with it (keep the plan a change
// would replace, resume a cancelled one, the provider's portal of payment methods and invoices, cancel it, after a
// question that says the perks stay until the period's end); the way to the plans on the supporters' page, where the
// reader subscribes or changes the plan or the period, or why they may not subscribe, in one sentence; their choice to
// be on the supporters list, for a member where the server shows the list; and the way to tip the server once.
//
// Back from the provider (its checkout or its portal: `?checkout=…` or `?portal=returned` on the address, which the
// screen takes off once read) the screen has the server ask the provider again, then says what came of the checkout:
// paid, left unpaid, or a payment not settled yet, which it reads again until it is. What came of an action is said in
// the subscription's section, in the screen's one polite live region, which takes the focus once the control that did
// it is gone (with the dialog over it, once that has left).

import { supporterSettingsPath, supportersPagePath, supporterTipPath } from "@my/app-components";
import {
  isSupporterMember,
  loadSupporterAccess,
  supporterActions,
  supporterRefusal,
  supporterReturnOf,
  supporterStanding,
  useInstance,
  useOwnSubscription,
  useSupporterAccess,
  useSupporterListing,
  useSupporterWrites,
  type AppData,
  type SupporterReturn,
  type SupporterSubscription,
} from "@my/app-data";
import {
  Box,
  Button,
  ModalPresence,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsNote,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
  SubscriptionState,
  useLocale,
} from "@my/design-system";
import { useEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import { stringsIn } from "../locale";
import { barSaid, barSentence, sentences, useMoney } from "../supporter-words";
import { saidOf } from "./confirmation";
import { Toggle } from "./controls";
import type { SettingsEntryProps } from "./entries";
import { Asking, ReadingNotice, useFocusAfterDialog, useMoment } from "./parts";

type T = ReturnType<typeof stringsIn>["t"];

/** What the route's loader waits for: who may subscribe and tip, and the reader's subscription. */
export async function loadSupporterSettings(data: AppData): Promise<void> {
  await loadSupporterAccess(data);
}

/** What the section says came of the last action: its words, each a new element of the live region. */
type Said = { readonly text: string; readonly n: number };

/**
 * What came of the reader's last action in the subscription's section: `say` puts its words in the section's live
 * region and gives it the focus, at once (`now`), or once the dialog over the section has left (`settle`, its
 * `onLeft`), since the control that did it went with what it changed.
 */
function useSaid(): {
  said: Said | undefined;
  ref: RefObject<HTMLParagraphElement | null>;
  say: (text: string, now?: boolean) => void;
  settle: () => void;
} {
  const ref = useRef<HTMLParagraphElement>(null);
  const [said, setSaid] = useState<Said>();
  const { expect, settle } = useFocusAfterDialog(ref);
  return {
    said,
    ref,
    say: (text, now = false) => {
      setSaid((before) => ({ text, n: (before?.n ?? 0) + 1 }));
      expect();
      if (now) settle();
    },
    settle,
  };
}

/** The screen of the entry. */
export function SupporterSettings({ back, search, onSearchRead, leave }: SettingsEntryProps) {
  const { t } = stringsIn(useLocale().locale);
  const access = useSupporterAccess(true);
  const program = access.program === true;
  const [returned] = useState<SupporterReturn | null>(() => {
    const r = supporterReturnOf(search ?? "");
    // A tip's return is its own screen's.
    return r?.kind === "tip" ? null : r;
  });
  const own = useOwnSubscription(program, returned?.kind === "checkout" && returned.paid);
  const writes = useSupporterWrites();

  // The return is read as the screen opened; it stays on the address until the app has taken it off.
  const onAddress = returned !== null && search !== undefined && search !== "";
  useEffect(() => {
    if (onAddress) onSearchRead?.();
  }, [onAddress, onSearchRead]);

  // Back from the provider, the server asks it again, once, before the screen says what came of it.
  const [synced, setSynced] = useState(returned === null);
  const syncing = useRef(false);
  useEffect(() => {
    if (synced || syncing.current || !program) return;
    syncing.current = true;
    // A sync that failed leaves the subscription as the server last read it, which the screen says.
    void writes
      .sync()
      .catch(() => undefined)
      .finally(() => {
        setSynced(true);
      });
  }, [synced, program, writes]);

  const failed = access.failed || (program && own.status === "error");
  const read = access.program !== undefined && (!program || own.read);
  return (
    <SettingsScreen
      icon="funding"
      title={t("settings.entry.supporter")}
      back={back}
      notice={
        read ? undefined : (
          <ReadingNotice
            status={failed ? "failed" : "loading"}
            retry={() => {
              access.retry();
              own.refetch();
            }}
            words={{ loading: t("settings.supporter.loading"), failed: t("settings.supporter.failed") }}
          />
        )
      }
    >
      {read && program && (
        <Subscription subscription={own.subscription} returned={synced ? returned : null} leave={leave} t={t} />
      )}
      {read && program && <Plans subscription={own.subscription} t={t} />}
      {read && program && <Listing subscription={own.subscription} t={t} />}
      {read && access.tips === true && (
        <SettingsSection heading={t("settings.supporter.tip")}>
          <SettingsList label={t("settings.supporter.tip")}>
            <SettingsListRow
              href={supporterTipPath}
              title={t("supporters.tip")}
              caption={t("settings.supporter.tip.caption")}
            />
          </SettingsList>
        </SettingsSection>
      )}
    </SettingsScreen>
  );
}

/** What the screen says of the provider's return, once the server asked it again; nothing for the portal's. */
function returnWords(
  returned: SupporterReturn | null,
  subscription: SupporterSubscription | undefined,
  t: T,
): string | undefined {
  if (returned?.kind !== "checkout") return undefined;
  if (!returned.paid) return t("settings.supporter.returned.canceled");
  // Settling while the server holds the subscription pending; else what the server says of it, the state line's.
  if (subscription?.state === "pending") return t("settings.supporter.returned.settling");
  if (subscription === undefined || !isSupporterMember(subscription)) return undefined;
  return t("settings.supporter.returned.paid", { plan: subscription.plan_name, interval: subscription.interval });
}

/**
 * The reader's subscription: where it stands, what they may do with it, and what came of the last action or of the
 * provider's return; that they have none, without one.
 */
function Subscription({
  subscription,
  returned,
  leave,
  t,
}: {
  subscription: SupporterSubscription | undefined;
  returned: SupporterReturn | null;
  leave: (href: string) => void;
  t: T;
}) {
  const writes = useSupporterWrites();
  const money = useMoney();
  const { locale } = useLocale();
  const day = useMoment(false);
  const { said, ref, say, settle } = useSaid();
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState<"portal" | "resume" | "keep">();
  const standing = supporterStanding(subscription);
  const actions = supporterActions(subscription);
  const dateOf = (iso: string | undefined) => (iso === undefined ? "" : day(iso));

  /** Runs a write of the section's own buttons, saying what came of it where the button was. */
  const run = (which: "resume" | "keep", write: () => Promise<SupporterSubscription>, done: string) => {
    setBusy(which);
    write().then(
      () => {
        setBusy(undefined);
        say(done, true);
      },
      (error: unknown) => {
        setBusy(undefined);
        say(saidOf(error, t), true);
      },
    );
  };
  const portal = () => {
    setBusy("portal");
    writes.portal().then(
      (url) => {
        // The page leaves for the provider's portal; the button spins until it has.
        leave(url);
      },
      (error: unknown) => {
        setBusy(undefined);
        say(supporterRefusal(error) === "ended" ? t("settings.supporter.portal.ended") : saidOf(error, t), true);
      },
    );
  };

  let line: ReactNode = null;
  if (subscription !== undefined) {
    const named = { plan: subscription.plan_name, interval: subscription.interval };
    switch (standing.kind) {
      case "pending":
        line = (
          <SettingsNote instead>
            {t("settings.supporter.state.pending", { ...named, price: money(subscription.price) })}
          </SettingsNote>
        );
        break;
      case "active":
        line = (
          <SubscriptionState
            state="renewing"
            detail={t("settings.supporter.detail.price", { ...named, price: money(subscription.price) })}
          >
            {t("settings.supporter.state.renewing", { date: dateOf(standing.renewsAt) })}
          </SubscriptionState>
        );
        break;
      case "scheduled":
        line = (
          <SubscriptionState
            state="changing"
            detail={t("settings.supporter.detail.changing", {
              ...named,
              now: money(subscription.price),
              price: money(standing.change.price),
              after: standing.change.interval,
            })}
            action={
              <Button
                size="target"
                busy={busy === "keep"}
                onPress={() => {
                  run("keep", writes.cancelScheduledChange, t("settings.supporter.kept", named));
                }}
              >
                {t("supporters.change.keep")}
              </Button>
            }
          >
            {t("settings.supporter.state.changing", {
              plan: standing.change.plan_name,
              interval: standing.change.interval,
              date: dateOf(standing.change.effective_at),
            })}
          </SubscriptionState>
        );
        break;
      case "canceling":
        line = (
          <SubscriptionState
            state="ending"
            detail={t("settings.supporter.detail.ending", { ...named, price: money(subscription.price) })}
            action={
              <Button
                size="target"
                busy={busy === "resume"}
                onPress={() => {
                  run("resume", writes.resume, t("settings.supporter.resumed", { date: dateOf(standing.endsAt) }));
                }}
              >
                {t("settings.supporter.resume")}
              </Button>
            }
          >
            {t("settings.supporter.state.ending", { date: dateOf(standing.endsAt) })}
          </SubscriptionState>
        );
        break;
      case "past-due":
      case "lapsed":
        // The failure is said here alone: no summary of the screen says it again.
        line = (
          <SubscriptionState
            state="failed"
            // What failed, or is charged once the payment method is updated.
            detail={t("settings.supporter.detail.price", { ...named, price: money(subscription.price) })}
            action={
              <Button kind="primary" size="target" busy={busy === "portal"} onPress={portal}>
                {t("settings.supporter.update_payment")}
              </Button>
            }
          >
            {standing.kind === "past-due"
              ? t("settings.supporter.state.failed", { date: dateOf(standing.graceEndsAt) })
              : t("settings.supporter.state.lapsed")}
          </SubscriptionState>
        );
        break;
      case "none":
        break;
    }
  }
  const failing = standing.kind === "past-due" || standing.kind === "lapsed";
  const shown = said?.text ?? returnWords(returned, subscription, t);
  return (
    <SettingsSection heading={t("supporters.subscription")}>
      {line ?? <SettingsEmpty>{t("settings.supporter.none")}</SettingsEmpty>}
      {actions.portal && !failing && (
        <Box>
          <Button size="target" busy={busy === "portal"} onPress={portal}>
            {t("settings.supporter.portal")}
          </Button>
        </Box>
      )}
      <SettingsResult ref={ref}>
        {shown !== undefined && <span key={said?.n ?? "returned"}>{shown}</span>}
      </SettingsResult>
      {/* What ends the subscription stands last, apart from what keeps it going. */}
      {actions.cancel && (
        <Box>
          <Button
            size="target"
            onPress={() => {
              setAsking(true);
            }}
          >
            {t("settings.supporter.cancel.open")}
          </Button>
        </Box>
      )}
      <ModalPresence>
        {asking && subscription !== undefined && (
          <Asking
            title={t("settings.supporter.cancel.title")}
            confirm={t("settings.supporter.cancel")}
            cancel={t("settings.supporter.cancel.keep")}
            run={async () => {
              const now = await writes.cancel();
              say(t("settings.supporter.canceled", { date: dateOf(now.period_ends_at) }));
            }}
            onClose={() => {
              setAsking(false);
            }}
            onLeft={settle}
          >
            {sentences(locale, [
              t("settings.supporter.cancel.text", { date: dateOf(subscription.period_ends_at) }),
              ...(subscription.scheduled_change === undefined
                ? []
                : [
                    t("settings.supporter.cancel.scheduled", {
                      plan: subscription.scheduled_change.plan_name,
                      interval: subscription.scheduled_change.interval,
                    }),
                  ]),
            ])}
          </Asking>
        )}
      </ModalPresence>
    </SettingsSection>
  );
}

/**
 * The way to the plans on the supporters' page: to subscribe, for a reader who may; to change the plan or the period,
 * for a member whose subscription is active; or why the reader may not subscribe, in one sentence.
 */
function Plans({ subscription, t }: { subscription: SupporterSubscription | undefined; t: T }) {
  const { subscribe } = useSupporterAccess(true);
  const change = supporterActions(subscription).change;
  if (subscribe.may !== true && !change) {
    if (!barSaid(subscribe.bar) || subscribe.bar === "subscribed") return null;
    return (
      <SettingsSection heading={t("supporters.plans")}>
        <SettingsNote instead>{barSentence(subscribe.bar, "subscribe", t)}</SettingsNote>
      </SettingsSection>
    );
  }
  return (
    <SettingsSection heading={t("supporters.plans")}>
      <SettingsList label={t("supporters.plans")}>
        <SettingsListRow
          href={supportersPagePath}
          title={t("settings.supporter.plans.door", { change: change ? "yes" : "no" })}
          caption={t("settings.supporter.plans.door.caption", { change: change ? "yes" : "no" })}
        />
      </SettingsList>
    </SettingsSection>
  );
}

/** The reader's choice to be on the supporters list, for a member where the server shows the list (FR-914). */
function Listing({ subscription, t }: { subscription: SupporterSubscription | undefined; t: T }) {
  const visibility = useInstance()?.supporter_list_visibility;
  const shown = isSupporterMember(subscription) && visibility !== undefined && visibility !== "off";
  const listing = useSupporterListing(shown);
  const writes = useSupporterWrites();
  if (!shown || listing.listed === undefined) return null;
  return (
    <SettingsSection heading={t("settings.supporter.listing")}>
      <Toggle
        page={supporterSettingsPath}
        label={t("settings.supporter.listing.label")}
        about={t("settings.supporter.listing.about", { audience: visibility })}
        on={listing.listed}
        onChange={writes.setListed}
      />
    </SettingsSection>
  );
}
