// SPDX-License-Identifier: AGPL-3.0-only

// The payment pages (ADR-0074 §11): the checkout, the customer portal and the error page of a provider without hosted
// ones (PAY.JP, and the fake provider of a test build), which the payment entry renders on their own page. Their forms
// post to the server, which answers with the page again or a redirect.

import {
  Button,
  Form,
  FormActions,
  FormAlert,
  FormCaution,
  FormHead,
  FormLink,
  FormLinks,
  FormList,
  FormPage,
  FormSection,
  FormText,
} from "@my/design-system";
import { use, type ReactNode } from "react";
import { CardForm, PayjpContext } from "./card-form";
import type { PayPage } from "./data";
import type { PayjpFactory } from "./payjp";
import { FocusOnLoadContext, usePayStrings } from "./strings";

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
  const screen = (
    <FocusOnLoadContext value={!preview}>
      <FormPage wordmark={wordmark}>
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

/** The title, with what the page is worth knowing first: a test build's fake, or PAY.JP's test keys. */
function Head({ page, children }: { page: PayPage; children?: ReactNode }) {
  const t = usePayStrings();
  return (
    <>
      {page.fake ? (
        <FormCaution>{t("pay.fake_notice")}</FormCaution>
      ) : (
        page.testMode && <FormCaution>{t("pay.test_mode")}</FormCaution>
      )}
      <FormHead title={page.title}>{children}</FormHead>
    </>
  );
}

/** What the server refused, right before the button that sent the form, taking the focus as the page opens. */
function Refused({ message }: { message: string | undefined }) {
  const focus = use(FocusOnLoadContext);
  return message ? <FormAlert focus={focus}>{message}</FormAlert> : null;
}

function Back({ page }: { page: PayPage }) {
  const t = usePayStrings();
  return page.back ? (
    <FormLinks>
      <FormLink href={page.back}>{t("pay.back")}</FormLink>
    </FormLinks>
  ) : null;
}

/** The price of a plan or a tip and how to pay it: PAY.JP's card form, or the fake's choice of an outcome. */
function Checkout({ page }: { page: PayPage }) {
  const t = usePayStrings();
  return (
    <>
      <Head page={page}>{page.price && <FormText>{page.price}</FormText>}</Head>
      {page.card ? (
        <CardForm card={page.card} submit={t("pay.checkout.submit")} error={page.error} />
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
      <Back page={page} />
    </>
  );
}

/** The card in use, a card change, and the recent payments; the fake has none of them. */
function Portal({ page }: { page: PayPage }) {
  const t = usePayStrings();
  const portal = page.portal;
  if (portal === undefined) {
    return (
      <>
        <Head page={page}>
          <FormText>{t("pay.fake.portal")}</FormText>
        </Head>
        <Back page={page} />
      </>
    );
  }
  return (
    <>
      <Head page={page}>
        {page.notice && <FormText role="status">{page.notice}</FormText>}
        {portal.paused && <FormCaution strong>{t("pay.portal.paused")}</FormCaution>}
      </Head>
      <FormSection id="pay-portal-card" heading={t("pay.portal.card")}>
        <FormText>{portal.card || t("pay.portal.no_card")}</FormText>
      </FormSection>
      {page.card && (
        <FormSection id="pay-portal-change" heading={t("pay.portal.change_card")}>
          <CardForm card={page.card} submit={t("pay.portal.change_card")} error={page.error} />
        </FormSection>
      )}
      <FormSection id="pay-portal-history" heading={t("pay.portal.history")}>
        {portal.payments.length === 0 ? (
          <FormText>{t("pay.portal.history_empty")}</FormText>
        ) : (
          <FormList
            items={portal.payments.map((p) =>
              p.refunded
                ? t("pay.portal.payment_refunded", { date: p.date, amount: p.amount, refunded: p.refunded })
                : t("pay.portal.payment", { date: p.date, amount: p.amount }),
            )}
          />
        )}
      </FormSection>
      <Back page={page} />
    </>
  );
}

/** What the server could not do, in its words. */
function ErrorPage({ page }: { page: PayPage }) {
  return (
    <>
      <Head page={page} />
      <Refused message={page.error} />
      <Back page={page} />
    </>
  );
}
