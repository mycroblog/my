// SPDX-License-Identifier: AGPL-3.0-only
// The tips as users see them (ADR-0162 §1, §3, §6, §7, §13, §17, §19): the user's page of the tips
// (`/settings/supporter/tip`) with the amounts, the final confirmation before the provider's hosted checkout and the
// thanks once the tip is paid; and the entries to it on the information page, the funding meter's panel and the page
// of the supporter plans. A tip is a tip for running the server, never a donation: the words say there is nothing in
// return and never speak of a deduction. The Tauri app shows none of it.
import {
  Box,
  Button,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  DialogTrigger,
  EmptyState,
  Hint,
  Link,
  Radio,
  RadioGroup,
  RegionError,
  Section,
  Spinner,
  Stack,
  StatusMessage,
  Text,
} from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useInstanceName } from "../../../lib/instanceName";
import { signInPath } from "../../../lib/paths";
import { useFormatMoney } from "../../costs";
import { DocumentLink } from "../../documents";
import type { SupporterIneligibleReason } from "../api/supporters";
import { type Money, tipPath } from "../api/tips";
import {
  type TipReturn,
  type TipReturnNote,
  useStartTip,
  useTipAmountChoice,
  useTipEntry,
  useTipPage,
} from "../hooks/useTip";

/**
 * The body of the user's page of the tips: the thanks once the tip that the checkout returned from is paid, the wait
 * for the provider's confirmation, or the amounts with the final confirmation before the checkout. A user who may not
 * tip reads what they need instead of the form.
 */
export function TipSettings({ returned }: { returned: TipReturn }) {
  const t = useT();
  const page = useTipPage(returned);
  switch (page.status) {
    case "app":
      return <EmptyState heading={t("client:tip.app")} />;
    case "disabled":
      return <EmptyState heading={t("client:tip.disabled")} />;
    case "loading":
      return (
        <div aria-busy="true">
          <Spinner label={t("client:tip.loading")} />
        </div>
      );
    case "error":
      return (
        <RegionError
          heading={t("client:tip.failed")}
          reason={page.reason}
          retryLabel={t("client:tip.retry")}
          onRetry={page.retry}
          isRetrying={page.isRetrying}
        />
      );
    case "confirming":
      return (
        <Stack gap="4" aria-busy="true">
          <StatusMessage>{t("client:tip.return.confirming")}</StatusMessage>
          <Spinner label={t("client:tip.loading")} />
        </Stack>
      );
    case "thanks":
      return <Thanks amount={page.tip.amount} />;
    case "ready":
      return (
        <Stack gap="8">
          <ReturnNote note={page.note} />
          <TipForm
            amounts={page.amounts}
            ineligible={page.ineligible}
            commerceDisclosureUrl={page.commerceDisclosureUrl}
          />
        </Stack>
      );
  }
}

/** The completion of a paid tip: the thanks, and nothing else (ADR-0162 §17). */
function Thanks({ amount }: { amount: Money }) {
  const t = useT();
  const format = useFormatMoney();
  const server = useInstanceName();
  return (
    <Stack gap="4">
      <StatusMessage tone="success">{t("client:tip.thanks.status")}</StatusMessage>
      <EmptyState
        heading={t("client:tip.thanks.title")}
        description={t("client:tip.thanks.description", { amount: format(amount), server: `⁨${server}⁩` })}
      />
    </Stack>
  );
}

/** What the return from the checkout came to when it was not a paid tip. */
function ReturnNote({ note }: { note: TipReturnNote }) {
  const t = useT();
  const messages = {
    canceled: "client:tip.return.canceled",
    unconfirmed: "client:tip.return.unconfirmed",
  } as const satisfies Record<NonNullable<TipReturnNote>, ClientMessageKey>;
  return <StatusMessage>{note === undefined ? undefined : t(messages[note])}</StatusMessage>;
}

const ineligibleMessages = {
  not_local: "client:tip.ineligible.not_local",
  email_unverified: "client:tip.ineligible.email_unverified",
  restricted: "client:tip.ineligible.restricted",
} as const satisfies Record<SupporterIneligibleReason, ClientMessageKey>;

/** What a tip is, the choice of its amount, and the final confirmation before the checkout. */
function TipForm({
  amounts,
  ineligible,
  commerceDisclosureUrl,
}: {
  amounts: readonly Money[];
  ineligible: SupporterIneligibleReason | undefined;
  commerceDisclosureUrl: string | undefined;
}) {
  const t = useT();
  const format = useFormatMoney();
  const server = useInstanceName();
  const choice = useTipAmountChoice(amounts);
  const start = useStartTip();
  const { amount } = choice;
  return (
    <Section title={t("client:tip.form.title", { server: `⁨${server}⁩` })}>
      <Hint size="inherit">{t("client:tip.form.description")}</Hint>
      {ineligible !== undefined ? (
        <Box as="p" border="lg" padding="4" role="note">
          {t(ineligibleMessages[ineligible])}
        </Box>
      ) : amount === undefined ? null : (
        <>
          <RadioGroup
            label={t("client:tip.form.amount")}
            value={choice.value ?? ""}
            onChange={(value) => {
              choice.choose(value);
            }}
          >
            {choice.choices.map((c) => (
              <Radio key={c.value} value={c.value}>
                {format(c.money)}
              </Radio>
            ))}
          </RadioGroup>
          <div>
            <DialogTrigger>
              <Button variant="primary">{t("client:tip.form.submit")}</Button>
              <TipConfirmation
                amount={amount}
                server={server}
                commerceDisclosureUrl={commerceDisclosureUrl}
                isPending={start.isPending}
                error={start.error}
                onConfirm={() => {
                  start.start(amount);
                }}
              />
            </DialogTrigger>
          </div>
        </>
      )}
    </Section>
  );
}

/**
 * The final confirmation before the checkout (ADR-0162 §19): the amount, that it is paid once and at once, that
 * nothing is given in return, and the link to the commerce disclosures.
 */
function TipConfirmation({
  amount,
  server,
  commerceDisclosureUrl,
  isPending,
  error,
  onConfirm,
}: {
  amount: Money;
  server: string;
  commerceDisclosureUrl: string | undefined;
  isPending: boolean;
  error: string | undefined;
  onConfirm: () => void;
}) {
  const t = useT();
  const format = useFormatMoney();
  return (
    <ConfirmDialog
      title={t("client:tip.confirm.title", { amount: format(amount), server: `⁨${server}⁩` })}
      description={t("client:tip.confirm.description")}
      confirmLabel={t("client:tip.confirm.go")}
      cancelLabel={t("client:tip.confirm.cancel")}
      tone="primary"
      isPending={isPending}
      {...(error !== undefined && { errorMessage: t("client:tip.confirm.failed", { reason: error }) })}
      onConfirm={onConfirm}
    >
      <DescriptionList columns={1}>
        <DescriptionItem term={t("client:tip.confirm.amount")}>{format(amount)}</DescriptionItem>
        <DescriptionItem term={t("client:tip.confirm.payment")}>
          {t("client:tip.confirm.payment.value")}
        </DescriptionItem>
        <DescriptionItem term={t("client:tip.confirm.return")}>{t("client:tip.confirm.return.value")}</DescriptionItem>
      </DescriptionList>
      {commerceDisclosureUrl !== undefined && (
        <p>
          <DocumentLink href={commerceDisclosureUrl}>{t("client:tip.commerce")}</DocumentLink>
        </p>
      )}
    </ConfirmDialog>
  );
}

/** The way to the page of the tips, or, for a visitor who is not signed in, to sign in first. */
function TipEntryLink({ entry }: { entry: "signed-in" | "signed-out" }) {
  const t = useT();
  return entry === "signed-in" ? (
    <Link href={tipPath} variant="standalone">
      {t("client:tip.entry.go")}
    </Link>
  ) : (
    <Link href={signInPath(tipPath)} variant="standalone">
      {t("client:tip.entry.sign_in")}
    </Link>
  );
}

/**
 * The tips on the information page (ADR-0162 §17): what a tip is, and the way to the page of the tips or, for a
 * visitor who is not signed in, who may tip and the way to sign in. Absent in the Tauri app and while the server
 * takes no tips.
 */
export function AboutTipsSection() {
  const t = useT();
  const server = useInstanceName();
  const entry = useTipEntry();
  if (entry === "hidden") return null;
  return (
    <Section title={t("client:tip.about.title")}>
      <p>{t("client:tip.about.description", { server: `⁨${server}⁩` })}</p>
      {entry === "signed-out" && <p>{t("client:tip.entry.eligibility")}</p>}
      <p>
        <TipEntryLink entry={entry} />
      </p>
    </Section>
  );
}

/**
 * The way to the tips under the funding meter in the third column: one line, and for a visitor who is not signed in
 * the way to sign in first. Nothing in the Tauri app and while the server takes no tips.
 */
export function FundingMeterTipLink() {
  const entry = useTipEntry();
  if (entry === "hidden") return null;
  return (
    <Text as="p" size="sm">
      <TipEntryLink entry={entry} />
    </Text>
  );
}

/** The way from the page of the supporter plans to the page of the tips, while the server takes them. */
export function SupporterTipLink() {
  const t = useT();
  const entry = useTipEntry();
  if (entry !== "signed-in") return null;
  return (
    <Section title={t("client:tip.about.title")}>
      <Hint size="inherit">{t("client:tip.settings.description")}</Hint>
      <p>
        <TipEntryLink entry={entry} />
      </p>
    </Section>
  );
}
