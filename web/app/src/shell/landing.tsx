// SPDX-License-Identifier: AGPL-3.0-only

// The landing page, at / for a visitor without a session (ADR-0028 §2, ADR-0059 §8): the server's front page, a page
// of its own outside the app's frame, for a visitor deciding whether to join, in one reading column at every width. It
// introduces the server (its thumbnail as its banner, its logo or the built-in wordmark with its name, with Sign in at
// the end of that line while the server takes new accounts, its summary, the link to its about page) and its counts
// (GET /instance/overview, while the instance shows them); it offers the account the registration mode allows: Create
// an account while anyone may ask for one, with a line on what it takes (a moderator's approval), Sign up with an
// invite where an invite is needed, and, on a server that takes no new accounts, says so before Sign in. It tells, in
// the software's own words, how the fediverse works, with its figure, and what a member can do here (only what is on),
// each with a sample of its screen, each part shown unless the operator hides it; and, low-key, the places a visitor
// may go to without an account (the timelines shown to everyone, and search). The footer links the server's
// documents, its source code (AGPL-3.0 §13) and its status page. A one-person server shows its operator's profile at /
// instead (ADR-0152 §7), in the app's frame.

import { aboutPagePath, privacyPagePath, termsPagePath } from "@my/app-components";
import { rightsOf, useInstance, useInstanceCounts, useStatusPageUrl, type Instance } from "@my/app-data";
import {
  FediverseFigure,
  FigureTiles,
  FormActions,
  FormLink,
  FormLinks,
  FormPageFooter,
  FormText,
  LandingFrame,
  LandingHead,
  LandingPlaces,
  LandingSamples,
  LandingSection,
  LinkButton,
  ServerMark,
  Stack,
  useLocale,
} from "@my/design-system";
import { useEffect, useId } from "react";
import { useStrings } from "../locale";
import type { MessageId } from "../messages.gen";
import { signInPath, signUpPath } from "../sign-in";
import { useLandingSamples } from "./landing-samples";
import { pageServer, wordmark } from "./pages";
import { placesFor, type Place } from "./places";

/**
 * Which of the two parts the software tells the operator shows: how the fediverse works, and what this server offers
 * its members. Both by default; each its own instance setting.
 */
export type LandingParts = { readonly fediverse: boolean; readonly features: boolean };

const everyPart: LandingParts = { fediverse: true, features: true };

/** What the account part of the landing page offers in a registration mode. */
export type AccountOffer = {
  /** The words of the primary action that asks for an account, if the visitor may ask for one; else signing in is. */
  readonly signUp?: MessageId;
  /** The line on what an account takes, if anything but the form. */
  readonly note?: MessageId;
};

/** The account part of the landing page in the registration mode `mode`. */
export function accountOffer(mode: Instance["registrations"]["mode"]): AccountOffer {
  switch (mode) {
    case "open":
      return { signUp: "app.landing_sign_up" };
    case "approval":
      return { signUp: "app.landing_sign_up", note: "app.landing_approval" };
    case "invite":
      return { signUp: "app.landing_sign_up_invite", note: "app.landing_invite" };
    case "closed":
      return { note: "app.landing_closed" };
  }
}

/** A thing a member can do on this server, which the landing page shows while it is on: its name and its sentence. */
export type Feature = { readonly key: string; readonly name: MessageId; readonly text: MessageId };

/** What a member can do on the server that not every server of the fediverse offers, of what is on here. */
export function landingFeatures(instance: Instance): readonly Feature[] {
  const all: readonly (Feature & { on: boolean })[] = [
    {
      key: "articles",
      name: "app.landing_feature_articles_name",
      text: "app.landing_feature_articles",
      on: instance.article_authoring === "everyone",
    },
    {
      key: "events",
      name: "app.events_place",
      text: "app.landing_feature_events",
      on: instance.event_creators === "everyone",
    },
    { key: "voice", name: "app.voice_rooms", text: "app.landing_feature_voice", on: instance.voice_enabled },
    {
      key: "search",
      name: "app.search",
      text: "app.landing_feature_search",
      on: instance.full_text_search.available,
    },
    {
      key: "translation",
      name: "app.landing_feature_translation_name",
      text: "app.landing_feature_translation",
      on: instance.translation.providers.length > 0,
    },
    {
      key: "supporters",
      name: "app.supporters",
      text: "app.landing_feature_supporters",
      on: instance.supporters_enabled || instance.tips_enabled,
    },
  ];
  return all.filter((f) => f.on).map(({ key, name, text }) => ({ key, name, text }));
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
    // The app's own pages of the documents, which the instance's addresses are, opened in place.
    ...(instance?.terms_url === undefined ? [] : [{ href: termsPagePath, label: t("app.footer_terms") }]),
    ...(instance?.privacy_policy_url === undefined ? [] : [{ href: privacyPagePath, label: t("app.footer_privacy") }]),
    ...(instance === undefined ? [] : [{ href: instance.source_url, label: t("app.landing_source") }]),
    ...(statusPage === undefined ? [] : [{ href: statusPage, label: t("app.status_page") }]),
  ];
  return <FormPageFooter label={t("app.footer")} links={links} />;
}

/** The server's counts, while it shows them: its members, and their public posts of the last 7 days. */
function Counts() {
  const t = useStrings();
  const { locale } = useLocale();
  const counts = useInstanceCounts();
  if (counts === undefined) return null;
  const number = new Intl.NumberFormat(locale);
  const week = counts.daily_posts.reduce((a, b) => a + b, 0);
  return (
    <FigureTiles
      label={t("app.landing_counts")}
      items={[
        { figure: number.format(counts.members), label: t("app.landing_members", { count: counts.members }) },
        { figure: number.format(week), label: t("app.landing_week_posts", { count: week }) },
      ]}
    />
  );
}

/**
 * The account's part: Create an account (or Sign up with an invite) and the line on what an account takes, while the
 * server takes new accounts, Sign in standing at the head of the page; else that it takes none, then Sign in.
 */
function Account({ instance }: { instance: Instance }) {
  const t = useStrings();
  const offer = accountOffer(instance.registrations.mode);
  if (offer.signUp === undefined)
    return (
      <FormActions>
        {offer.note !== undefined && <FormText>{t(offer.note)}</FormText>}
        <LinkButton href={signInPath} kind="primary">
          {t("app.sign_in_action")}
        </LinkButton>
      </FormActions>
    );
  return (
    <FormActions>
      <LinkButton href={signUpPath} kind="primary">
        {t(offer.signUp)}
      </LinkButton>
      {offer.note !== undefined && <FormText muted>{t(offer.note)}</FormText>}
    </FormActions>
  );
}

/**
 * What the software tells of itself, each part the operator shows: the fediverse, its figure over its sentence; and
 * what a member can do here, a sample of each.
 */
function Tells({ instance, parts }: { instance: Instance; parts: LandingParts }) {
  const t = useStrings();
  const fediverse = useId();
  const offers = useId();
  const samples = useLandingSamples(instance.domain);
  const features = parts.features ? landingFeatures(instance) : [];
  return (
    <>
      {parts.fediverse && (
        <LandingSection id={fediverse} heading={t("app.landing_fediverse_heading")}>
          <FediverseFigure />
          <FormText>{t("app.landing_fediverse", { domain: instance.domain })}</FormText>
        </LandingSection>
      )}
      {features.length > 0 && (
        <LandingSection id={offers} heading={t("app.landing_features_heading")}>
          <LandingSamples
            items={features.map((f) => ({ key: f.key, name: t(f.name), text: t(f.text), sample: samples[f.key] }))}
          />
        </LandingSection>
      )}
    </>
  );
}

/** The places a visitor may go to without an account, low-key. */
function Places({ instance }: { instance: Instance }) {
  const t = useStrings();
  return (
    <LandingPlaces
      heading={t("app.landing_without_account")}
      items={visitorPlaces(instance).map((place) => ({
        href: place.href,
        icon: place.icon,
        label: t(placeLabels[place.href] ?? place.label),
      }))}
    />
  );
}

/**
 * The landing page: the introduction, with Sign in at the end of its head while the server takes new accounts; the
 * account's part; what the software tells; the places without an account; the footer.
 */
export function Landing({
  serverName,
  parts = everyPart,
}: {
  /** The server's name, which stands for it until the instance answered. */
  serverName: string;
  /** The parts the software tells that the operator shows. */
  parts?: LandingParts | undefined;
}) {
  const t = useStrings();
  const instance = useInstance();
  const server = { ...pageServer(instance, serverName), banner: instance?.thumbnail?.url };
  useEffect(() => {
    document.title = server.name;
  }, [server.name]);
  const takes = instance !== undefined && accountOffer(instance.registrations.mode).signUp !== undefined;
  return (
    <LandingFrame
      banner={server.banner}
      label={t("app.landing_account")}
      intro={
        <>
          <LandingHead
            mark={<ServerMark server={server} wordmark={wordmark} size="intro" title />}
            end={
              takes && (
                <LinkButton href={signInPath} kind="secondary">
                  {t("app.sign_in_action")}
                </LinkButton>
              )
            }
          />
          {/* What leads on from the summary stays with it. */}
          <Stack gap="form-page-head-gap">
            {server.description !== undefined && <FormText>{server.description}</FormText>}
            <FormLinks>
              <FormLink href={aboutPagePath}>{t("app.landing_about")}</FormLink>
            </FormLinks>
          </Stack>
          <Counts />
        </>
      }
      join={instance !== undefined && <Account instance={instance} />}
      footer={<LandingFooter instance={instance} />}
    >
      {instance !== undefined && (
        <>
          <Tells instance={instance} parts={parts} />
          <Places instance={instance} />
        </>
      )}
    </LandingFrame>
  );
}
