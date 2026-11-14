// SPDX-License-Identifier: AGPL-3.0-only

// The landing page, at / for a visitor without a session (ADR-0028 §2, ADR-0059 §8): the server's front page, a page
// of its own outside the app's frame, as its sign-in is. Its introduction (the thumbnail as its banner, its logo or the
// built-in wordmark with its name, its summary, and the link to its about page) heads it on a phone and stands on the
// panel at the start from `desktop`. The column holds the account first: Create an account as the primary action while
// the registration mode lets anyone ask for one, with a line on what it takes (a moderator's approval, an invite), and
// Sign in, which is the primary action of a server that takes no new accounts; then what a visitor may do without an
// account, the places of the navigation they have (the timelines shown to everyone, and search), each with its glyph.
// The footer links the server's documents, its source code (AGPL-3.0 §13) and its status page. A one-person server
// shows its operator's profile at / instead (ADR-0152 §7), in the app's frame.

import { rightsOf, useInstance, useStatusPageUrl, type Instance } from "@my/app-data";
import {
  FormActions,
  FormLink,
  FormLinks,
  FormOthers,
  FormPage,
  FormPageFooter,
  FormText,
  GlyphLabel,
  LinkButton,
} from "@my/design-system";
import { useEffect } from "react";
import wordmarkDark from "../../../brand/Wordmark-DarkBg.svg";
import wordmarkLight from "../../../brand/Wordmark-Primary-Color.svg";
import { useStrings } from "../locale";
import type { MessageId } from "../messages.gen";
import { signInPath, signUpPath } from "../sign-in";
import { pageServer } from "./pages";
import { placesFor, type Place } from "./places";

const wordmark = { light: wordmarkLight, dark: wordmarkDark };

/** What the account part of the landing page offers in a registration mode. */
export type AccountOffer = {
  /** Whether the visitor may ask for an account, which is then the primary action; else signing in is. */
  readonly signUp: boolean;
  /** The line under the actions on what an account takes, if anything but the form. */
  readonly note?: MessageId;
};

/** The account part of the landing page in the registration mode `mode`. */
export function accountOffer(mode: Instance["registrations"]["mode"]): AccountOffer {
  switch (mode) {
    case "open":
      return { signUp: true };
    case "approval":
      return { signUp: true, note: "app.landing_approval" };
    case "invite":
      return { signUp: true, note: "app.landing_invite" };
    case "closed":
      return { signUp: false, note: "app.landing_closed" };
  }
}

/** The words of each place a visitor may go to from the landing page, which start with what they do there. */
const placeLabels: Readonly<Record<string, MessageId>> = {
  "/local": "app.landing_local",
  "/public": "app.landing_federated",
  "/search": "app.landing_search",
};

/** The places a visitor may go to from the landing page: the navigation's own, in its order. */
export function visitorPlaces(instance: Instance): readonly Place[] {
  return placesFor(false, instance, rightsOf([])).tabs;
}

/** The footer: the server's documents, its source code, and its status page, each when it has one. */
function LandingFooter({ instance }: { instance: Instance | undefined }) {
  const t = useStrings();
  const statusPage = useStatusPageUrl();
  const links = [
    ...(instance?.terms_url === undefined ? [] : [{ href: instance.terms_url, label: t("app.footer_terms") }]),
    ...(instance?.privacy_policy_url === undefined
      ? []
      : [{ href: instance.privacy_policy_url, label: t("app.footer_privacy") }]),
    ...(instance === undefined ? [] : [{ href: instance.source_url, label: t("app.landing_source") }]),
    ...(statusPage === undefined ? [] : [{ href: statusPage, label: t("app.status_page") }]),
  ];
  return <FormPageFooter label={t("app.footer")} links={links} />;
}

/** The account's actions, and the line on what an account takes. */
function Account({ instance }: { instance: Instance }) {
  const t = useStrings();
  const offer = accountOffer(instance.registrations.mode);
  return (
    <FormActions>
      {offer.signUp && (
        <LinkButton href={signUpPath} kind="primary">
          {t("app.landing_sign_up")}
        </LinkButton>
      )}
      <LinkButton href={signInPath} kind={offer.signUp ? "secondary" : "primary"}>
        {t("app.sign_in_action")}
      </LinkButton>
      {offer.note !== undefined && <FormText muted>{t(offer.note)}</FormText>}
    </FormActions>
  );
}

/** What a visitor may do without an account, under its heading; nothing when there is nothing. */
function WithoutAccount({ instance }: { instance: Instance }) {
  const t = useStrings();
  const places = visitorPlaces(instance);
  if (places.length === 0) return null;
  return (
    <FormOthers heading={t("app.landing_without_account")}>
      {places.map((place) => (
        <LinkButton key={place.href} href={place.href}>
          <GlyphLabel icon={place.icon}>{t(placeLabels[place.href] ?? place.label)}</GlyphLabel>
        </LinkButton>
      ))}
    </FormOthers>
  );
}

/** The landing page; the server's name (`serverName`) stands for it until the instance answered. */
export function Landing({ serverName }: { serverName: string }) {
  const t = useStrings();
  const instance = useInstance();
  const server = { ...pageServer(instance, serverName), banner: instance?.thumbnail?.url };
  useEffect(() => {
    document.title = server.name;
  }, [server.name]);
  return (
    <FormPage
      server={server}
      wordmark={wordmark}
      front={{
        more: (
          <FormLinks>
            <FormLink href="/about">{t("app.landing_about")}</FormLink>
          </FormLinks>
        ),
      }}
      footer={<LandingFooter instance={instance} />}
    >
      {instance !== undefined && (
        <>
          <Account instance={instance} />
          <WithoutAccount instance={instance} />
        </>
      )}
    </FormPage>
  );
}
