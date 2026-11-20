// SPDX-License-Identifier: AGPL-3.0-only

// The landing page, at / for a visitor without a session (ADR-0028 §2, ADR-0059 §8): the server's front page, a page
// of its own outside the app's frame, for a visitor deciding whether to join. It introduces the server (its thumbnail
// as its banner, its logo or the built-in wordmark with its name, its summary, the link to its about page) and its
// counts (GET /instance/overview, while the instance shows them); it offers the account the registration mode allows:
// Create an account while anyone may ask for one, with a line on what it takes (a moderator's approval), Sign up with
// an invite where an invite is needed, and Sign in, which is the primary action of a server that takes no new
// accounts and stays on the page for a member who is signed out. It tells, in the software's own words, how the
// fediverse works and what this server offers its members (only what is on here), each part shown unless the
// operator hides it; and, low-key, the places a visitor may go to without an account (the timelines shown to
// everyone, and search). The footer links the server's documents, its source code (AGPL-3.0 §13) and its status
// page. A one-person server shows its operator's profile at / instead (ADR-0152 §7), in the app's frame.
//
// Three arrangements of the same parts are offered for the decider's pick (the stories' `landing` options): `panel`,
// the frame of the form pages, the introduction and its counts on the panel at the start from `desktop`; `page`, one
// reading column at every width, Sign in at the end of the introduction's head; `aside`, the introduction and what
// the page tells in the main column, the account's part beside it from `tablet`.

import { aboutPagePath, privacyPagePath, termsPagePath } from "@my/app-components";
import { rightsOf, useInstance, useInstanceCounts, useStatusPageUrl, type Instance } from "@my/app-data";
import {
  FigureTiles,
  FormActions,
  FormLink,
  FormLinks,
  FormPage,
  FormPageFooter,
  FormText,
  LandingFeatures,
  LandingFrame,
  LandingPlaces,
  LandingSection,
  LinkButton,
  Row,
  ServerMark,
  Stack,
  useLocale,
  type IconName,
} from "@my/design-system";
import { useEffect, useId, type ReactNode } from "react";
import { useStrings } from "../locale";
import type { MessageId } from "../messages.gen";
import { signInPath, signUpPath } from "../sign-in";
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

/** A thing a member can do on this server, which the landing page names while it is on. */
export type Feature = { readonly key: string; readonly icon: IconName; readonly text: MessageId };

/** What a member can do on the server that not every server of the fediverse offers, of what is on here. */
export function landingFeatures(instance: Instance): readonly Feature[] {
  const all: readonly (Feature & { on: boolean })[] = [
    {
      key: "articles",
      icon: "article",
      text: "app.landing_feature_articles",
      on: instance.article_authoring === "everyone",
    },
    { key: "events", icon: "events", text: "app.landing_feature_events", on: instance.event_creators === "everyone" },
    { key: "voice", icon: "room", text: "app.landing_feature_voice", on: instance.voice_enabled },
    { key: "search", icon: "search", text: "app.landing_feature_search", on: instance.full_text_search.available },
    {
      key: "translation",
      icon: "translate",
      text: "app.landing_feature_translation",
      on: instance.translation.providers.length > 0,
    },
    {
      key: "supporters",
      icon: "funding",
      text: "app.landing_feature_supporters",
      on: instance.supporters_enabled || instance.tips_enabled,
    },
  ];
  return all.filter((f) => f.on).map(({ key, icon, text }) => ({ key, icon, text }));
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

/** The link to the server's about page. */
function AboutLink() {
  const t = useStrings();
  return (
    <FormLinks>
      <FormLink href={aboutPagePath}>{t("app.landing_about")}</FormLink>
    </FormLinks>
  );
}

/**
 * The account's actions, and the line on what an account takes; Sign in among them unless `signIn` is false, where
 * the page offers it elsewhere while the server takes new accounts.
 */
function Account({ instance, signIn = true }: { instance: Instance; signIn?: boolean }) {
  const t = useStrings();
  const offer = accountOffer(instance.registrations.mode);
  return (
    <FormActions>
      {offer.signUp !== undefined && (
        <LinkButton href={signUpPath} kind="primary">
          {t(offer.signUp)}
        </LinkButton>
      )}
      {(signIn || offer.signUp === undefined) && (
        <LinkButton href={signInPath} kind={offer.signUp === undefined ? "primary" : "secondary"}>
          {t("app.sign_in_action")}
        </LinkButton>
      )}
      {offer.note !== undefined && <FormText muted>{t(offer.note)}</FormText>}
    </FormActions>
  );
}

/** What the software tells of itself, each part the operator shows: the fediverse, and what this server offers. */
function Tells({ instance, parts }: { instance: Instance; parts: LandingParts }) {
  const t = useStrings();
  const fediverse = useId();
  const offers = useId();
  const features = parts.features ? landingFeatures(instance) : [];
  return (
    <>
      {parts.fediverse && (
        <LandingSection id={fediverse} heading={t("app.landing_fediverse_heading")}>
          <FormText>{t("app.landing_fediverse", { domain: instance.domain })}</FormText>
        </LandingSection>
      )}
      {features.length > 0 && (
        <LandingSection id={offers} heading={t("app.landing_features_heading")}>
          <LandingFeatures items={features.map((f) => ({ key: f.key, icon: f.icon, text: t(f.text) }))} />
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

/** The server as the landing page introduces it, and the page's title kept at its name. */
function useServer(serverName: string) {
  const instance = useInstance();
  const server = { ...pageServer(instance, serverName), banner: instance?.thumbnail?.url };
  useEffect(() => {
    document.title = server.name;
  }, [server.name]);
  return { instance, server };
}

type LandingProps = {
  /** The server's name, which stands for it until the instance answered. */
  serverName: string;
  /** The parts the software tells that the operator shows. */
  parts?: LandingParts | undefined;
};

/**
 * `panel`: the frame of the form pages. The introduction, its counts and the about link head the page on a phone and
 * a tablet, and stand on the panel at the start from `desktop`; the column holds the account, what the software
 * tells, and the places without an account.
 */
export function LandingPanel({ serverName, parts = everyPart }: LandingProps) {
  const { instance, server } = useServer(serverName);
  return (
    <FormPage
      server={server}
      wordmark={wordmark}
      front={{
        more: (
          <Stack gap="form-page-intro-gap">
            <AboutLink />
            <Counts />
          </Stack>
        ),
      }}
      footer={<LandingFooter instance={instance} />}
    >
      {instance !== undefined && (
        <>
          <Account instance={instance} />
          <Tells instance={instance} parts={parts} />
          <Places instance={instance} />
        </>
      )}
    </FormPage>
  );
}

/** The introduction of `page` and `aside` under the banner: the mark and name, the summary, the about link, the counts. */
function Intro({
  server,
  end,
}: {
  server: ReturnType<typeof useServer>["server"];
  /** What stands at the end of the mark's line (Sign in). */
  end?: ReactNode;
}) {
  return (
    <>
      <Row wrap align="center" justify="between" gap="form-page-gap">
        <ServerMark server={server} wordmark={wordmark} size="intro" title />
        {end}
      </Row>
      {/* What leads on from the summary stays with it. */}
      <Stack gap="form-page-head-gap">
        {server.description !== undefined && <FormText>{server.description}</FormText>}
        <AboutLink />
      </Stack>
      <Counts />
    </>
  );
}

/**
 * `page`: one reading column at every width, as a site's front page. The introduction, with Sign in at the end of
 * its head while the server takes new accounts; the account's part; what the software tells; the places.
 */
export function LandingPage({ serverName, parts = everyPart }: LandingProps) {
  const t = useStrings();
  const { instance, server } = useServer(serverName);
  const takes = instance !== undefined && accountOffer(instance.registrations.mode).signUp !== undefined;
  return (
    <LandingFrame
      layout="column"
      banner={server.banner}
      label={t("app.landing_account")}
      intro={
        <Intro
          server={server}
          end={
            takes && (
              <LinkButton href={signInPath} kind="secondary">
                {t("app.sign_in_action")}
              </LinkButton>
            )
          }
        />
      }
      join={instance !== undefined && <Account instance={instance} signIn={false} />}
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

/**
 * `aside`: the introduction and what the software tells in the main column, the account's part beside it from
 * `tablet`, on the raised surface and in view while the page scrolls; on a phone, right after the introduction. The
 * places without an account stand under the account.
 */
export function LandingAside({ serverName, parts = everyPart }: LandingProps) {
  const t = useStrings();
  const { instance, server } = useServer(serverName);
  return (
    <LandingFrame
      layout="aside"
      banner={server.banner}
      label={t("app.landing_account")}
      intro={<Intro server={server} />}
      join={
        instance !== undefined && (
          <>
            <Account instance={instance} />
            <Places instance={instance} />
          </>
        )
      }
      footer={<LandingFooter instance={instance} />}
    >
      {instance !== undefined && <Tells instance={instance} parts={parts} />}
    </LandingFrame>
  );
}
