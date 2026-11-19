// SPDX-License-Identifier: AGPL-3.0-only

// The payment pages (ADR-0074 §11): the checkout, the customer portal and the error page of a provider without hosted
// ones (PAY.JP, and the fake provider of a test build), which the payment entry renders on their own page, the layout of
// the auth screens: the server that is paid introduced beside the form from desktop, named above it on a phone. Their
// forms post to the server, which answers with the page again or a redirect. Every amount is written as the app writes
// money, with its currency's symbol.

import {
  Button,
  Form,
  FormActions,
  FormAlert,
  FormCaution,
  FormHead,
  FormPage,
  FormRows,
  FormSection,
  FormText,
  LinkButton,
} from "@my/design-system";
import { use, type ReactNode } from "react";
import { CardForm, PayjpContext } from "./card-form";
import type { PayPage } from "./data";
import type { PayjpFactory } from "./payjp";
import { FocusOnLoadContext, useMoney, usePayStrings } from "./strings";

/**
 * The page of `page`, with the built-in wordmark that the app gives. `preview` shows it among others, where no refusal
 * takes the focus, and `payjp` stands in for PAY.JP's script there.
 */
export function PayScreen({
  page,
  wordmark,
  preview = false,
  payjp,
}: {
  page: PayPage;
  wordmark: { light: string; dark: string };
  preview?: boolean;
  payjp?: (() => Promise<PayjpFactory>) | undefined;
}) {
  const t = usePayStrings();
  // A test build's fake, or PAY.JP's test keys, is what the page is worth knowing first: no money moves.
  const band = page.fake ? t("pay.fake_notice") : page.testMode ? t("pay.test_mode") : undefined;
  const screen = (
    <FocusOnLoadContext value={!preview}>
      <FormPage server={page.server} wordmark={wordmark} band={band}>
        <Screen page={page} />
      </FormPage>
    </FocusOnLoadContext>
  );
  return payjp === undefined ? screen : <PayjpContext value={payjp}>{screen}</PayjpContext>;
}

function Screen({ page }: { page: PayPage }) {
  switch (page.screen) {
    case "checkout":
      return <Checkout page={page} />;
    case "portal":
      return <Portal page={page} />;
    case "error":
      return <ErrorPage page={page} />;
  }
}

/** What the server refused, right before the button that sent the form, taking the focus as the page opens. */
function Refused({ message }: { message: string | undefined }) {
  const focus = use(FocusOnLoadContext);
  return message ? <FormAlert focus={focus}>{message}</FormAlert> : null;
}

/** The way back to the screen of the app the reader came from, a finger's target tall. */
function Back({
  href,
  kind = "quiet",
  children,
}: {
  href: string | undefined;
  kind?: "quiet" | "secondary";
  children?: ReactNode;
}) {
  const t = usePayStrings();
  return href ? (
    <FormActions>
      <LinkButton kind={kind} href={href}>
        {children ?? t("pay.back")}
      </LinkButton>
    </FormActions>
  ) : null;
}

/**
 * The price of a plan or a tip and how to pay it: PAY.JP's card form, whose button says what is charged now, or the
 * fake's choice of an outcome, whose Cancel is the one way back.
 */
function Checkout({ page }: { page: PayPage }) {
  const t = usePayStrings();
  const { money, price } = useMoney();
  const p = page.price;
  return (
    <>
      <FormHead title={page.title}>{p && <FormText>{price(p)}</FormText>}</FormHead>
      {page.card ? (
        <>
          <CardForm
            card={page.card}
            submit={t("pay.checkout.submit", { amount: p ? money(p.amount, p.currency) : "" })}
            then={p && p.interval !== "once" ? t("pay.checkout.then", { price: price(p) }) : undefined}
            error={page.error}
          />
          <Back href={page.back} />
        </>
      ) : (
        page.choices && (
          <Form action={page.choices.action}>
            <FormActions>
              <Refused message={page.error} />
              <Button kind="primary" type="submit" name="action" value="pay">
                {t("pay.fake.pay")}
              </Button>
              <Button type="submit" name="action" value="fail">
                {t("pay.fake.fail")}
              </Button>
              <Button kind="quiet" type="submit" name="action" value="cancel">
                {t("pay.fake.cancel")}
              </Button>
            </FormActions>
          </Form>
        )
      )}
    </>
  );
}

/**
 * The subscription first (its plan and price, its next payment or that it is paused), then the card in use, a card
 * change or a first card, and the recent payments, date first and amount at the end; the fake has none of them.
 */
function Portal({ page }: { page: PayPage }) {
  const t = usePayStrings();
  const { money, price } = useMoney();
  const portal = page.portal;
  if (portal === undefined) {
    return (
      <>
        <FormHead title={page.title}>
          <FormText>{t("pay.fake.portal")}</FormText>
        </FormHead>
        <Back href={page.back} />
      </>
    );
  }
  const plan = portal.plan;
  const change = portal.card ? t("pay.portal.change_card") : t("pay.portal.add_card");
  return (
    <>
      <FormHead title={page.title}>
        {plan && <FormText>{t("pay.portal.plan", { plan: plan.name, price: price(plan.price) })}</FormText>}
        {portal.paused ? (
          <FormCaution strong>{t("pay.portal.paused")}</FormCaution>
        ) : (
          plan?.next && <FormText>{t("pay.portal.next", { date: plan.next })}</FormText>
        )}
        {page.notice && <FormText role="status">{page.notice}</FormText>}
      </FormHead>
      {portal.card && (
        <FormSection id="pay-portal-card" heading={t("pay.portal.card")}>
          <FormText>{portal.card}</FormText>
        </FormSection>
      )}
      {page.card && (
        <FormSection id="pay-portal-change" heading={change}>
          {!portal.card && <FormText>{t("pay.portal.no_card")}</FormText>}
          <CardForm card={page.card} submit={change} error={page.error} />
        </FormSection>
      )}
      <FormSection id="pay-portal-history" heading={t("pay.portal.history")}>
        {portal.payments.length === 0 ? (
          <FormText>{t("pay.portal.history_empty")}</FormText>
        ) : (
          <FormRows
            rows={portal.payments.map((p, i) => ({
              key: `${p.date}-${i}`,
              start: p.date,
              end: money(p.amount, p.currency),
              note: p.refunded ? t("pay.portal.refunded", { amount: money(p.refunded, p.currency) }) : undefined,
            }))}
          />
        )}
      </FormSection>
      <Back href={page.back} />
    </>
  );
}

/** What the server could not do, in its words, and the way back to the app. */
function ErrorPage({ page }: { page: PayPage }) {
  const t = usePayStrings();
  return (
    <>
      <FormHead title={page.title} />
      <Refused message={page.error} />
      <Back href={page.back ?? "/"} kind="secondary">
        {t("pay.back_to_app")}
      </Back>
    </>
  );
}
