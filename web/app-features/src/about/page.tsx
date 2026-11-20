// SPDX-License-Identifier: AGPL-3.0-only

// The server's about page, at /about, for everyone, signed in or not (FR-705, FR-718, FR-725, FR-810, FR-903, FR-905,
// FR-914; ADR-0057 §5, ADR-0067 §8, ADR-0072 §2, ADR-0088 §20). Its head is the server's as a profile's is: its
// thumbnail as the banner, its icon (the built-in one without), its name over its domain, its summary, then its
// description. Under it, each section the server sets or shows, in the order a person deciding to join reads them: who
// runs it, its rules and its documents, how it moderates (the domains it moderates, what it sends to an external
// review, how long it keeps posts), what it states to search engines and AI, its costs and funding, how to support it,
// and its version, source code and status page. A section the server says nothing of is left out, so the page reads
// whole with every section and with nearly none; a page of its own (the costs, the supporters, a document) is a row
// that opens it. The moderated domains show where the instance lists them to the reader; for a visitor of a server that
// lists them to signed-in readers, one line says so with the way to sign in. The page takes what it shows in its props:
// the instance, and the list of the moderated domains as the screen reads it.

import {
  AccountLine,
  commercePagePath,
  costsPagePath,
  crawlerPagePath,
  ProfileHead,
  privacyPagePath,
  supportersPagePath,
  termsPagePath,
} from "@my/app-components";
import { financePublished, type Instance, type InstanceDomainAction, type PagedList } from "@my/app-data";
import {
  Avatar,
  Box,
  ContentLink,
  FactList,
  FeedNotice,
  OpenRow,
  Row,
  ScreenStack,
  ShowMore,
  Stack,
  TextButton,
  TextLink,
  useAddedFocus,
  useLocale,
  type Fact,
} from "@my/design-system";
import { useId, type ReactNode } from "react";
import { FundingMeterView } from "../finance/meter";
import { stringsIn } from "../locale";
import { sentences } from "../supporter-words";

type T = ReturnType<typeof stringsIn>["t"];

/** The about page's strings, in the reader's locale. */
export function useAboutStrings(): T {
  return stringsIn(useLocale().locale).t;
}

/** A section of the page: its heading, close over what it holds. */
function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Stack as="section" aria-label={title} gap="content-paragraph" text="body">
      <Box as="h2" text="title" weight="strong" breaks="phrase">
        {title}
      </Box>
      {children}
    </Stack>
  );
}

/** Text people wrote, in paragraphs where it has blank lines, in its own direction and an unknown language. */
function Paragraphs({ text }: { text: string }) {
  return (
    <Stack gap="content-paragraph" text="body" lang="">
      {text
        .split(/\n\s*\n/)
        .map((p) => p.trim())
        .filter((p) => p !== "")
        .map((p, i) => (
          <Box key={i} as="p" keeps="lines" breaks="phrase">
            <bdi>{p}</bdi>
          </Box>
        ))}
    </Stack>
  );
}

/**
 * The moderated domains as the page shows them: none where the instance lists them to nobody, nor to this reader
 * (undefined); for a visitor where it lists them to signed-in readers, the way to sign in (`signIn`); else the list.
 */
export type AboutDomains = { readonly signIn: string } | { readonly list: PagedList<InstanceDomainAction> } | undefined;

export type AboutViewProps = {
  /** The instance; undefined until it is read. */
  instance: Instance | undefined;
  /** Reads the instance again, once it could not be read: its Try again. */
  retry?: (() => void) | undefined;
  domains: AboutDomains;
  /** The built-in icon, which a server without its own shows. */
  builtInIcon: string;
};

/**
 * In place of a page of the server's information until the instance is read: that it loads, or, with `retry`, that it
 * could not be read, with Try again.
 */
export function AboutWaiting({ retry }: { retry?: (() => void) | undefined }): ReactNode {
  const t = useAboutStrings();
  return retry === undefined ? (
    <FeedNotice lead>{t("about.loading")}</FeedNotice>
  ) : (
    <FeedNotice alert lead action={<TextButton onPress={retry}>{t("about.retry")}</TextButton>}>
      {t("about.failed")}
    </FeedNotice>
  );
}

/** The about page of the server. */
export function AboutView({ instance, retry, domains, builtInIcon }: AboutViewProps): ReactNode {
  const t = useAboutStrings();
  if (instance === undefined) return <AboutWaiting retry={retry} />;
  return (
    <ScreenStack>
      <Head instance={instance} builtInIcon={builtInIcon} />
      {instance.contact !== undefined && <Contact contact={instance.contact} t={t} />}
      {instance.rules.length > 0 && <Rules rules={instance.rules} t={t} />}
      <Documents instance={instance} t={t} />
      {domains !== undefined && <ModeratedDomains domains={domains} t={t} />}
      {instance.external_reviews !== undefined && instance.external_reviews.length > 0 && (
        <Reviews hooks={instance.external_reviews} scope={instance.external_review_scope} t={t} />
      )}
      {instance.auto_delete_limit !== undefined && <AutoDelete limit={instance.auto_delete_limit} t={t} />}
      <Crawlers instance={instance} t={t} />
      <Funding instance={instance} t={t} />
      <Support instance={instance} t={t} />
      <Server instance={instance} t={t} />
    </ScreenStack>
  );
}

/**
 * The server's head: with a thumbnail, as a profile's (the banner, the icon over its edge, the name over the domain);
 * without, no empty band, the icon beside the name and the domain. Then its summary and its description.
 */
function Head({ instance, builtInIcon }: { instance: Instance; builtInIcon: string }) {
  const nameId = useId();
  const avatar = <Avatar name={instance.name} src={instance.icon?.svg_url ?? builtInIcon} size="preview" decorative />;
  const text = (
    <>
      {instance.short_description !== undefined && <Paragraphs text={instance.short_description} />}
      {instance.description !== undefined && <Paragraphs text={instance.description} />}
    </>
  );
  if (instance.thumbnail !== undefined)
    return (
      <ProfileHead
        nameId={nameId}
        banner={instance.thumbnail.url}
        avatar={avatar}
        name={instance.name}
        handle={instance.domain}
      >
        {text}
      </ProfileHead>
    );
  return (
    <Stack as="section" aria-labelledby={nameId} gap="screen-profile-gap">
      <Row align="center" gap="screen-head-room">
        <Row inline rigid>
          {avatar}
        </Row>
        <Stack gap="hair">
          <Box as="h2" text="title" weight="strong" breaks="phrase">
            <bdi id={nameId}>{instance.name}</bdi>
          </Box>
          <Box as="p" text="caption" tone="muted" breaks="anywhere">
            <bdi dir="ltr">{instance.domain}</bdi>
          </Box>
        </Stack>
      </Row>
      {text}
    </Stack>
  );
}

/** Who runs the server: its account, and its address; the crawler's page shows them too. */
export function ContactDetails({ contact }: { contact: NonNullable<Instance["contact"]> }): ReactNode {
  return (
    <>
      {contact.account !== undefined && <AccountLine account={contact.account} />}
      {contact.email !== undefined && (
        <Box as="p" breaks="anywhere">
          <bdi dir="ltr">
            <ContentLink kind="link" href={`mailto:${contact.email}`}>
              {contact.email}
            </ContentLink>
          </bdi>
        </Box>
      )}
    </>
  );
}

/** The section of who runs the server. */
function Contact({ contact, t }: { contact: NonNullable<Instance["contact"]>; t: T }) {
  return (
    <Section title={t("about.contact")}>
      <ContactDetails contact={contact} />
    </Section>
  );
}

/** The rules in effect, numbered, each its hint under it. */
function Rules({ rules, t }: { rules: Instance["rules"]; t: T }) {
  return (
    <Section title={t("about.rules")}>
      <Stack as="ol" gap="screen-row-gap" plain lang="">
        {rules.map((rule, i) => (
          <Row as="li" key={rule.id} gap="entry-name" align="baseline">
            <Box as="span" tone="muted" numeric rigid aria-hidden>
              {t("about.rules.number", { n: i + 1 })}
            </Box>
            <Stack grow gap="hair">
              <Box as="span" breaks="phrase">
                <bdi>{rule.text}</bdi>
              </Box>
              {rule.hint !== undefined && (
                <Box as="span" text="caption" tone="muted" breaks="phrase">
                  <bdi>{rule.hint}</bdi>
                </Box>
              )}
            </Stack>
          </Row>
        ))}
      </Stack>
    </Section>
  );
}

/** The terms of service and the privacy policy, where the server published them. */
function Documents({ instance, t }: { instance: Instance; t: T }) {
  if (instance.terms_url === undefined && instance.privacy_policy_url === undefined) return null;
  return (
    <Section title={t("about.documents")}>
      <Stack gap="hair">
        {instance.terms_url !== undefined && <OpenRow href={termsPagePath} icon="file" title={t("about.terms")} />}
        {instance.privacy_policy_url !== undefined && (
          <OpenRow href={privacyPagePath} icon="file" title={t("about.privacy")} />
        )}
      </Stack>
    </Section>
  );
}

/** The domains the server moderates, where it lists them to the reader; to a visitor, the way to sign in. */
function ModeratedDomains({ domains, t }: { domains: NonNullable<AboutDomains>; t: T }) {
  return (
    <Section title={t("about.domains")}>
      {"list" in domains ? (
        <DomainList list={domains.list} t={t} />
      ) : (
        <FeedNotice start action={<TextLink href={domains.signIn}>{t("about.sign_in")}</TextLink>}>
          {t("about.domains.signed_in")}
        </FeedNotice>
      )}
    </Section>
  );
}

/** The list of the moderated domains, a page at a time. */
function DomainList({ list, t }: { list: PagedList<InstanceDomainAction>; t: T }) {
  const added = useAddedFocus(list.items.length);
  if (list.status === "pending") return <FeedNotice start>{t("about.domains.loading")}</FeedNotice>;
  if (list.status === "error")
    return (
      <FeedNotice start alert action={<TextButton onPress={list.refetch}>{t("about.retry")}</TextButton>}>
        {t("about.domains.failed")}
      </FeedNotice>
    );
  if (list.items.length === 0) return <FeedNotice start>{t("about.domains.none")}</FeedNotice>;
  return (
    <Stack gap="screen-row-gap">
      <Stack as="ul" plain gap="screen-row-gap" aria-label={t("about.domains")}>
        {list.items.map((item, i) => (
          <li key={item.digest} ref={added.item(i)}>
            <Domain item={item} t={t} />
          </li>
        ))}
      </Stack>
      <ShowMore
        more={list.hasNextPage}
        busy={list.isFetchingNextPage}
        failed={list.failedNextPage}
        onPress={added.press(list.fetchNextPage)}
      />
    </Stack>
  );
}

/**
 * A moderated domain: its name on a line of its own, then what was done, with the public reason after a middle dot, so
 * that a long name never parts the state from the line it heads.
 */
function Domain({ item, t }: { item: InstanceDomainAction; t: T }) {
  return (
    <Stack gap="hair">
      <Box as="span" breaks="anywhere">
        <bdi dir="ltr">{item.domain}</bdi>
      </Box>
      <Box as="span" text="caption" tone="muted" breaks="phrase">
        {t("about.domains.kind", { kind: item.kind })}
        {item.comment !== undefined && (
          <>
            {"\u00a0\u00b7 "}
            <bdi lang="">{item.comment}</bdi>
          </>
        )}
      </Box>
    </Stack>
  );
}

/** What the server sends to an external review before it accepts it, each with what narrows it. */
function Reviews({
  hooks,
  scope,
  t,
}: {
  hooks: NonNullable<Instance["external_reviews"]>;
  scope: Instance["external_review_scope"];
  t: T;
}) {
  const line = (hook: (typeof hooks)[number]): string => {
    const name = t("about.reviews.hook", { hook: hook.replace(".", "_") });
    const days =
      hook === "local.status"
        ? scope?.local_status_account_age_days
        : hook === "local.profile"
          ? scope?.local_profile_account_age_days
          : undefined;
    if (days !== undefined && days > 0) return t("about.reviews.new_accounts", { what: name, days });
    if (hook === "inbound.status" && scope?.inbound_status_unfollowed_direct === true)
      return t("about.reviews.unfollowed_direct", { what: name });
    return name;
  };
  return (
    <Section title={t("about.reviews")}>
      <Box as="p" breaks="phrase">
        {t("about.reviews.lead")}
      </Box>
      <Stack as="ul" gap="hair">
        {hooks.map((hook) => (
          <Box as="li" key={hook} breaks="phrase">
            {line(hook)}
          </Box>
        ))}
      </Stack>
    </Section>
  );
}

/** The most days the server keeps a post, and what a person may keep longer. */
function AutoDelete({ limit, t }: { limit: NonNullable<Instance["auto_delete_limit"]>; t: T }) {
  const { locale } = useLocale();
  const kept = [
    ...(limit.allow_pinned
      ? [limit.kept_pins >= 5 ? t("about.auto_delete.pinned") : t("about.auto_delete.pins", { n: limit.kept_pins })]
      : []),
    ...(limit.allow_bookmarked ? [t("about.auto_delete.bookmarked")] : []),
    ...(limit.allow_favourited ? [t("about.auto_delete.favourited")] : []),
    ...(limit.allow_media ? [t("about.auto_delete.media")] : []),
    ...(limit.allow_polls ? [t("about.auto_delete.polls")] : []),
    ...(limit.allow_articles === true ? [t("about.auto_delete.articles")] : []),
    ...(limit.allow_visibilities ? [t("about.auto_delete.visibilities")] : []),
    ...(limit.allow_thresholds ? [t("about.auto_delete.thresholds")] : []),
  ];
  return (
    <Section title={t("about.auto_delete")}>
      <Box as="p" breaks="phrase">
        {sentences(locale, [
          t("about.auto_delete.max", { days: limit.max_age_days }),
          ...(kept.length > 0
            ? [
                t("about.auto_delete.keep", {
                  kinds: new Intl.ListFormat(locale, { type: "conjunction" }).format(kept),
                }),
              ]
            : []),
        ])}
      </Box>
    </Section>
  );
}

/**
 * What the server states to search engines and to AI crawlers, how it states it and how far that reaches, and that it
 * is no legal advice (ADR-0057 §5); then that it fetches linked pages as a crawler of its own, and the page for the
 * operators of those websites.
 */
function Crawlers({ instance, t }: { instance: Instance; t: T }) {
  const { locale } = useLocale();
  return (
    <Section title={t("about.crawlers")}>
      <Box as="p" breaks="phrase">
        {sentences(locale, [
          t("about.crawlers.search", { value: instance.search_indexing }),
          t("about.crawlers.ai", { value: instance.ai_usage }),
        ])}
      </Box>
      <Box as="p" breaks="phrase">
        {t("about.crawlers.how")}
      </Box>
      <Box as="p" text="caption" tone="muted" breaks="phrase">
        {t("about.crawlers.legal")}
      </Box>
      <Box as="p" breaks="phrase">
        {t("about.crawlers.link_cards")}
      </Box>
      <OpenRow href={crawlerPagePath} icon="linkCards" title={t("about.crawlers.crawler")} />
    </Section>
  );
}

/** The funding meter of the month and the way to the costs and income, where the server shows either. */
function Funding({ instance, t }: { instance: Instance; t: T }) {
  const meter = instance.funding_meter;
  const published = financePublished(instance).any;
  if (meter === undefined && !published) return null;
  return (
    <Section title={t("about.funding")}>
      {meter !== undefined && <FundingMeterView meter={meter} />}
      <OpenRow href={costsPagePath} icon="finance" title={t("about.funding.page")} />
    </Section>
  );
}

/** The way to the plans, the tips and the supporters, and to the commerce disclosure, where the server has them. */
function Support({ instance, t }: { instance: Instance; t: T }) {
  const plans = instance.supporters_enabled;
  const tips = instance.tips_enabled;
  const list = plans && instance.supporter_list_visibility !== "off";
  const commerce = instance.commerce_disclosure_url !== undefined;
  if (!plans && !tips && !commerce) return null;
  const offer = plans && tips ? "both" : plans ? "plans" : tips ? "tips" : "none";
  return (
    <Section title={t("about.support")}>
      <Stack gap="hair">
        {offer !== "none" && (
          <OpenRow
            href={supportersPagePath}
            icon="funding"
            title={t("about.support.page", { offer })}
            hint={list ? t("about.support.list") : undefined}
          />
        )}
        {commerce && <OpenRow href={commercePagePath} icon="file" title={t("about.commerce")} />}
      </Stack>
    </Section>
  );
}

/** An address as a link reads it: without its scheme and a lone trailing slash. */
const shownAddress = (url: string) => url.replace(/^(?:https?:\/\/|mailto:)/, "").replace(/\/$/, "");

/** The server's version, the source code it runs (AGPL-3.0 §13), and its status page. */
function Server({ instance, t }: { instance: Instance; t: T }) {
  const link = (href: string) => (
    <Box as="bdi" dir="ltr" breaks="anywhere">
      <ContentLink kind="link" href={href}>
        {shownAddress(href)}
      </ContentLink>
    </Box>
  );
  const items: Fact[] = [
    ...(instance.status_page_url === undefined
      ? []
      : [{ term: t("about.server.status"), value: link(instance.status_page_url) }]),
    { term: t("about.server.source"), value: link(instance.source_url) },
    { term: t("about.server.version"), value: <bdi dir="ltr">{instance.version}</bdi> },
  ];
  return (
    <Section title={t("about.server")}>
      <FactList label={t("about.server")} items={items} />
    </Section>
  );
}
