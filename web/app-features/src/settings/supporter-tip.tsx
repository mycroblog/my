// SPDX-License-Identifier: AGPL-3.0-only

// Tip the server, a screen under the settings' Supporter at /settings/supporter/tip, where the server takes tips and
// never in the native shell (ADR-0162 §1, §3, §6, §7, §13; FR-918, FR-919): the operator's amounts, one chosen, beside
// what a tip is (a thank-you for the server's service, not a donation, with no perks and no role), and the one primary
// action, which starts the tip and leaves for the provider's checkout in the same tab. One tip is open at a time:
// starting another ends the checkout the reader left open, which the screen says before. The server limits how often
// a tip starts, and the screen says how long to wait; what else it refused stands before the button.
//
// Back from the checkout (`?tip=<id>&checkout=…`, which the screen takes off once read) the screen reads that tip, the
// server asking the provider again while it is open, and says first what came of it (paid, with thanks; not paid; a
// payment not settled yet, read again until it is), its title taking the focus as it comes and as it changes.

import {
  refusalDetail,
  supporterRefusal,
  supporterRetryAfterMs,
  supporterReturnOf,
  tipAmountsQuery,
  useInstance,
  useSupporterAccess,
  useSupporterTip,
  useSupporterWrites,
  useTipAmounts,
  type AppData,
  type Money,
} from "@my/app-data";
import {
  BackButton,
  Box,
  FrameFoot,
  Button,
  FormAlert,
  SegmentChoice,
  SettingsNote,
  SettingsScreen,
  SettingsSection,
  Stack,
  StatusSummary,
  useLocale,
} from "@my/design-system";
import { useEffect, useRef, useState } from "react";
import { stringsIn } from "../locale";
import { barSaid, barSentence, useMoney } from "../supporter-words";
import type { SettingsSubProps } from "./entries";
import { ReadingNotice } from "./parts";

type T = ReturnType<typeof stringsIn>["t"];

/** What the route's loader waits for: who may tip, and the operator's amounts. */
export async function loadSupporterTip(data: AppData): Promise<void> {
  await data.queryClient.query({ ...tipAmountsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The title of the screen, for the route's heading. */
export function useSupporterTipTitle(): string {
  return stringsIn(useLocale().locale).t("supporters.tip");
}

/** The key of an amount among the choices. */
const keyOf = (money: Money) => `${money.amount} ${money.currency}`;

/** Why the server refused to start the tip, in one sentence. */
function tipRefusal(error: unknown, t: T): string {
  const kind = supporterRefusal(error);
  if (kind === "rate-limited") {
    const ms = supporterRetryAfterMs(error);
    return ms === undefined
      ? t("settings.supporter.tip.limited.later")
      : t("settings.supporter.tip.limited", { minutes: Math.max(1, Math.ceil(ms / 60_000)) });
  }
  // Where the program is off the screen cannot know beforehand why the reader may not tip (no read of the API says
  // whether their address is confirmed): the server's words say it.
  if (kind === "ineligible") return refusalDetail(error) ?? t("settings.supporter.tip.refused", { kind: "ineligible" });
  switch (kind) {
    case "tip-amount":
    case "tip-pending":
    case "tips-unavailable":
    case "closing":
      return t("settings.supporter.tip.refused", { kind: kind.replace("-", "_") });
    default:
      return t("settings.supporter.tip.refused", { kind: "other" });
  }
}

/** What came of the tip the reader is back from: its state, said first, its title taking the focus. */
function Returned({ tipId, paid, t }: { tipId: string; paid: boolean; t: T }) {
  const read = useSupporterTip(tipId);
  const money = useMoney();
  const server = useInstance()?.name ?? "";
  const titleRef = useRef<HTMLHeadingElement>(null);
  let shown: "paid" | "unpaid" | "settling" | "gone" | undefined;
  if (read.gone) shown = "gone";
  else if (read.tip === undefined) shown = undefined;
  else if (read.tip.state === "paid") shown = "paid";
  // Left unpaid, the reader came back from the checkout without paying: nothing was charged.
  else if (!paid || read.tip.state === "expired") shown = "unpaid";
  else shown = "settling";
  useEffect(() => {
    if (shown !== undefined) titleRef.current?.focus();
  }, [shown]);
  if (shown === undefined)
    return read.status === "error" && !read.gone ? (
      <SettingsSection heading={t("settings.supporter.tip.returned")}>
        <SettingsNote instead>{t("settings.supporter.tip.returned.failed")}</SettingsNote>
      </SettingsSection>
    ) : null;
  return (
    <StatusSummary
      icon={shown === "paid" ? "accepted" : shown === "settling" ? "schedule" : "caution"}
      titleRef={titleRef}
      title={t("settings.supporter.tip.result", {
        result: shown,
        amount: read.tip === undefined ? "" : money(read.tip.amount),
      })}
      about={t("settings.supporter.tip.result.about", { result: shown, server })}
    />
  );
}

/** The screen: what came of a tip the reader is back from, the amounts, and the button that tips. */
export function SupporterTip({ onBack, search, onSearchRead, leave }: SettingsSubProps) {
  const { t } = stringsIn(useLocale().locale);
  const access = useSupporterAccess(true);
  const offered = access.tips === true && !access.app;
  const amounts = useTipAmounts(offered);
  const writes = useSupporterWrites();
  const money = useMoney();
  const server = useInstance()?.name ?? "";
  const [returned] = useState(() => {
    const r = supporterReturnOf(search ?? "");
    return r?.kind === "tip" ? r : null;
  });
  const onAddress = returned !== null && search !== undefined && search !== "";
  useEffect(() => {
    if (onAddress) onSearchRead?.();
  }, [onAddress, onSearchRead]);
  const [chosen, setChosen] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState<{ said: string; n: number }>();

  const list = amounts.amounts ?? [];
  // The smallest amount, until the reader chooses another.
  const amount = list.find((a) => keyOf(a) === chosen) ?? list[0];
  const may = access.tip.may === true;
  const read = access.tips !== undefined && (!offered || amounts.amounts !== undefined);
  const failed = access.failed || amounts.status === "error";

  const tip = () => {
    if (amount === undefined) return;
    setBusy(true);
    writes.tip(amount).then(
      (checkout) => {
        // The page leaves for the provider's checkout; the button spins until it has.
        leave(checkout.redirectUrl);
      },
      (error: unknown) => {
        setBusy(false);
        setRefused((before) => ({ said: tipRefusal(error, t), n: (before?.n ?? 0) + 1 }));
        if (supporterRefusal(error) === "tip-amount") amounts.refetch();
      },
    );
  };

  return (
    <SettingsScreen
      sub
      title={t("supporters.tip")}
      back={<BackButton onPress={onBack}>{t("settings.entry.supporter")}</BackButton>}
      notice={
        !read ? (
          <ReadingNotice
            status={failed ? "failed" : "loading"}
            retry={() => {
              access.retry();
              amounts.refetch();
            }}
            words={{ loading: t("settings.supporter.tip.loading"), failed: t("settings.supporter.tip.failed") }}
          />
        ) : undefined
      }
      foot={
        offered && may && amount !== undefined ? (
          // The screen's one action, the foot's whole width, where the thumb is; what came of it right over it, in a
          // polite live region there from the start.
          <FrameFoot>
            <Stack gap="screen-row-gap">
              <Box role="status">
                {refused === undefined ? null : <FormAlert key={refused.n}>{refused.said}</FormAlert>}
              </Box>
              <Button kind="primary" busy={busy} onPress={tip}>
                {t("settings.supporter.tip.pay", { amount: money(amount) })}
              </Button>
            </Stack>
          </FrameFoot>
        ) : undefined
      }
    >
      {returned !== null && <Returned tipId={returned.tipId} paid={returned.paid} t={t} />}
      {read && !offered && (
        <SettingsSection heading={t("settings.supporter.tip.amount")}>
          <SettingsNote instead>{t("settings.supporter.tip.off")}</SettingsNote>
        </SettingsSection>
      )}
      {read && offered && (
        <SettingsSection heading={t("settings.supporter.tip.amount")}>
          <SegmentChoice
            label={t("settings.supporter.tip.amount")}
            options={list.map((a) => ({ value: keyOf(a), label: money(a) }))}
            value={amount === undefined ? null : keyOf(amount)}
            onChange={(value) => {
              setChosen(value);
              setRefused(undefined);
            }}
            isDisabled={!may || busy}
            about={t("supporters.tip.about", { server })}
          />
          {may ? (
            <SettingsNote>{t("settings.supporter.tip.once")}</SettingsNote>
          ) : (
            barSaid(access.tip.bar) && <SettingsNote instead>{barSentence(access.tip.bar, "tip", t)}</SettingsNote>
          )}
        </SettingsSection>
      )}
    </SettingsScreen>
  );
}
