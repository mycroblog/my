// SPDX-License-Identifier: AGPL-3.0-only
import {
  AccountItem,
  BannerImage,
  Button,
  Cluster,
  EmptyState,
  ExternalLink,
  Heading,
  Hint,
  Link,
  RegionError,
  Spinner,
  Stack,
  Text,
} from "@my/ui";
import { type ReactNode, useId } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { handleOf } from "../../../lib/handle";
import { domainToUnicode } from "../../../lib/idn";
import { imageSources } from "../../../lib/images";
import { profilePath } from "../../../lib/paths";
import { useStatusPageUrl } from "../../../lib/statusPage";
import { DocumentLink, DocumentsSection } from "../../documents";
import { AboutSupportersSection } from "../../supporters";
import type { Instance, PublicDomainAction } from "../api/about";
import { type ModeratedDomains, useAbout, useModeratedDomains } from "../hooks/useAbout";

/**
 * The body of the instance's information page (`/about`), under the page's heading: the thumbnail, the summary, the
 * description, the rules followed by its terms of service and privacy policy, the remote domains it moderates as far
 * as the instance lists them, what it states to search engines and AI crawlers, what running it costs and what its
 * supporters bring in as far as the instance publishes them, its supporter plans and the supporter list, its commerce disclosures, how to reach the operators,
 * its status page, where to get its source code, and the license notices of what the web app bundles.
 * Everyone may read it, signed in or not.
 */
export function About() {
  const about = useAbout();
  const statusPageUrl = useStatusPageUrl();
  return (
    <Stack gap="8">
      {about.thumbnail !== undefined && <ServerThumbnail thumbnail={about.thumbnail} />}
      {about.summary !== "" && (
        // The administrator's text, in a language the app does not know: lang="" rather than the app's (web-client/README.md Y10).
        <Text as="p" dir="auto" lang="" size="lg">
          {about.summary}
        </Text>
      )}
      {about.description.length > 0 && <Description paragraphs={about.description} />}
      {/* What users agree to stands together: the rules, then the terms and policies. */}
      {about.rules.length > 0 && <Rules rules={about.rules} />}
      <DocumentsSection />
      {/* How the server treats other servers and crawlers. */}
      <ModeratedDomainsSection />
      {about.crawlers !== undefined && (
        <Crawlers searchIndexing={about.crawlers.searchIndexing} aiUsage={about.crawlers.aiUsage} />
      )}
      {/* Who runs the server, what it costs and what it runs. */}
      {about.finance !== undefined &&
        about.income !== undefined &&
        (about.finance !== "private" || about.income !== "private") && (
          <Costs finance={about.finance} income={about.income} />
        )}
      <AboutSupportersSection />
      {about.commerceDisclosureUrl !== undefined && <CommerceDisclosure url={about.commerceDisclosureUrl} />}
      {about.contact !== undefined && <Contact contact={about.contact} />}
      {statusPageUrl !== undefined && <StatusPage url={statusPageUrl} />}
      {about.sourceUrl !== undefined && <SourceCode url={about.sourceUrl} />}
      <Licenses />
    </Stack>
  );
}

/** A section of the page: an `h2` that names a `<section>`. */
function Section({ title, children }: { title: string; children: ReactNode }) {
  const headingId = useId();
  return (
    <Stack as="section" aria-labelledby={headingId} gap="3">
      <Heading level={2} id={headingId}>
        {title}
      </Heading>
      {children}
    </Stack>
  );
}

function Description({ paragraphs }: { paragraphs: readonly string[] }) {
  const t = useT();
  return (
    <Section title={t("client:about.description.title")}>
      {paragraphs.map((paragraph) => (
        // Paragraphs of one text have no IDs; the text itself is the key (two equal paragraphs are not expected).
        <Text as="p" key={paragraph} dir="auto" lang="" whitespace="preLine">
          {paragraph}
        </Text>
      ))}
    </Section>
  );
}

function Rules({ rules }: { rules: ReturnType<typeof useAbout>["rules"] }) {
  const t = useT();
  return (
    <Section title={t("client:about.rules.title")}>
      <Stack as="ol" marker="decimal" gap="2">
        {rules.map((rule) => (
          // The item follows the page's direction, so that its number stays in the list's padding; the rule's text
          // follows its own (dir="auto"); lang="" because the administrator's text has no known language.
          <li key={rule.id} lang="">
            <Text as="p" dir="auto" weight="medium">
              {rule.text}
            </Text>
            {rule.hint !== undefined && <Hint dir="auto">{rule.hint}</Hint>}
          </li>
        ))}
      </Stack>
    </Section>
  );
}

/**
 * The image that represents the instance, at the width of the page. Its alt text is the administrator's; without
 * one, the image is decorative, as the page names the server in its heading.
 */
export function ServerThumbnail({ thumbnail }: { thumbnail: NonNullable<Instance["thumbnail"]> }) {
  return (
    <BannerImage src={thumbnail.url} alt={thumbnail.alt ?? ""} width={thumbnail.width} height={thumbnail.height} />
  );
}

/** How to reach the operators: their account on the server and their email address, as far as the server has them. */
function Contact({ contact }: { contact: NonNullable<Instance["contact"]> }) {
  const t = useT();
  const { account, email } = contact;
  return (
    <Section title={t("client:about.contact.title")}>
      <Stack as="dl" gap="3">
        {account !== undefined && (
          <Stack gap="1">
            <Text as="dt" size="sm" tone="muted">
              {t("client:about.contact.account")}
            </Text>
            <dd>
              <AccountItem
                name={account.display_name || account.username}
                handle={handleOf(account.acct)}
                avatar={imageSources(account.avatar)}
                avatarSeed={account.id}
                href={profilePath(account.acct)}
                layout="inline"
              />
            </dd>
          </Stack>
        )}
        {email !== undefined && (
          <Stack gap="1">
            <Text as="dt" size="sm" tone="muted">
              {t("client:about.contact.email")}
            </Text>
            <dd>
              {/* An address reads left to right in every locale. */}
              <Link href={`mailto:${email}`} variant="standalone">
                <Text dir="ltr" wrap="anywhere">
                  {email}
                </Text>
              </Link>
            </dd>
          </Stack>
        )}
      </Stack>
    </Section>
  );
}

/**
 * What the instance states to search engines and AI crawlers (FR-725, ADR-0057), and what such a statement is: a
 * request that crawlers may or may not follow, with the legal standing it may have, never a promise of one.
 */
function Crawlers({
  searchIndexing,
  aiUsage,
}: {
  searchIndexing: Instance["search_indexing"];
  aiUsage: Instance["ai_usage"];
}) {
  const t = useT();
  return (
    <Section title={t("client:about.crawlers.title")}>
      <p>{t("client:about.crawlers.search", { setting: searchIndexing })}</p>
      <p>{t("client:about.crawlers.ai", { setting: aiUsage })}</p>
      <Hint>{t("client:about.crawlers.how")}</Hint>
      <Hint>{t("client:about.crawlers.accounts")}</Hint>
      <Hint>{t("client:about.crawlers.legal")}</Hint>
    </Section>
  );
}

/**
 * What running the server costs and what its supporters bring in (FR-903, FR-917, ADR-0067 §8, ADR-0073 §7): what the
 * instance publishes of each, whether the balance is published (only when both are), and the link to the page of the
 * totals. Absent when the instance publishes neither.
 */
function Costs({
  finance,
  income,
}: {
  finance: Instance["finance_visibility"];
  income: Instance["income_visibility"];
}) {
  const t = useT();
  return (
    <Section title={t("client:about.costs.title")}>
      {finance !== "private" && <p>{t("client:about.costs.description", { visibility: finance })}</p>}
      {income !== "private" && <p>{t("client:about.income.description", { visibility: income })}</p>}
      {finance !== "private" && income !== "private" && <p>{t("client:about.income.balance")}</p>}
      <p>
        <Link href="/about/costs" variant="standalone">
          {t("client:about.costs.link", {
            shown: finance === "private" ? "income" : income === "private" ? "costs" : "both",
          })}
        </Link>
      </p>
    </Section>
  );
}

/**
 * The link to the server's page of the commerce disclosures of its supporter plans (ADR-0072 §13). Its page is an information page
 * of the app, which opens in the same tab. Absent when the instance has none.
 */
function CommerceDisclosure({ url }: { url: string }) {
  const t = useT();
  return (
    <Section title={t("client:about.commerce.title")}>
      <p>
        <DocumentLink href={url}>{t("client:about.commerce.link")}</DocumentLink>
      </p>
    </Section>
  );
}

/**
 * The link to the instance's status page, where its operators tell of outages and maintenance (FR-810, ADR-0085 §6).
 * The page is another site, so the link leaves the app (web-client/README.md Y12).
 */
function StatusPage({ url }: { url: string }) {
  const t = useT();
  return (
    <Section title={t("client:about.status.title")}>
      <p>{t("client:about.status.description")}</p>
      <p>
        <ExternalLink href={url} variant="standalone">
          {t("client:about.status.link")}
        </ExternalLink>
      </p>
    </Section>
  );
}

/** The address of a mailto: URI (without its query), or undefined when the URI is not one. */
function mailtoAddress(url: string): string | undefined {
  if (!url.startsWith("mailto:")) return undefined;
  const address = url.slice("mailto:".length).split("?")[0] ?? "";
  try {
    return decodeURIComponent(address);
  } catch {
    return address;
  }
}

/**
 * The link to the source code the server runs (AGPL-3.0 section 13, ADR-0011): to the page that publishes it, which
 * leaves the app, or, for a mailto: URI, to an email that requests it.
 */
export function SourceCodeLink({ url }: { url: string }) {
  const t = useT();
  if (mailtoAddress(url) !== undefined) {
    return (
      <Link href={url} variant="standalone">
        {t("client:about.source.request")}
      </Link>
    );
  }
  return (
    <ExternalLink href={url} variant="standalone">
      {t("client:about.source.view")}
    </ExternalLink>
  );
}

/** Where users get the source code the server runs, and why they may (AGPL-3.0 section 13). */
function SourceCode({ url }: { url: string }) {
  const t = useT();
  const address = mailtoAddress(url);
  return (
    <Section title={t("client:about.source.title")}>
      <p>{t("client:about.source.license")}</p>
      <p>
        <SourceCodeLink url={url} />
      </p>
      {address !== undefined && (
        <Stack as="dl" gap="1">
          <Text as="dt" size="sm" tone="muted">
            {t("client:about.source.address")}
          </Text>
          {/* An address reads left to right in every locale. */}
          <Text as="dd" dir="ltr" align="start" wrap="anywhere">
            {address}
          </Text>
        </Stack>
      )}
    </Section>
  );
}

/**
 * The license notices of the third-party code, data and fonts that the build bundles, which Go serves as plain text at
 * /licenses/ (a page of the server, outside the app): those of the npm packages of the app and of its Service Worker
 * (Vite's build.license) and those of the bundled data that is no npm package (vite-plugins/licenses.ts): the Unicode
 * emoji data (ADR-0091 §1) and the Noto Color Emoji fonts (ADR-0095 §6). Keep it in step with vite.config.ts.
 */
const licenseNotices: readonly { href: string; label: ClientMessageKey }[] = [
  { href: "/licenses/npm-packages.md", label: "client:about.licenses.npm" },
  { href: "/licenses/npm-packages-sw.md", label: "client:about.licenses.npm_sw" },
  { href: "/licenses/LICENSE-Unicode.txt", label: "client:about.licenses.unicode" },
  { href: "/licenses/OFL.txt", label: "client:about.licenses.noto" },
];

/** The links to the license notices of what the web app bundles: the same on every server, so always shown. */
function Licenses() {
  const t = useT();
  return (
    <Section title={t("client:about.licenses.title")}>
      <p>{t("client:about.licenses.intro")}</p>
      <Stack as="ul" marker="disc" gap="2">
        {licenseNotices.map(({ href, label }) => (
          <li key={href}>
            <Link href={href} variant="standalone">
              {t(label)}
            </Link>
          </li>
        ))}
      </Stack>
    </Section>
  );
}

/** The section of the moderated domains; absent when the instance lists them to nobody. */
function ModeratedDomainsSection() {
  const t = useT();
  const domains = useModeratedDomains();
  if (domains.status === "hidden") return null;
  return (
    <Section title={t("client:about.domains.title")}>
      <Hint>{t("client:about.domains.intro")}</Hint>
      <ModeratedDomainsBody domains={domains} />
    </Section>
  );
}

function ModeratedDomainsBody({ domains }: { domains: Exclude<ModeratedDomains, { status: "hidden" }> }) {
  const t = useT();
  switch (domains.status) {
    case "signed-in-only":
      return (
        <Cluster as="p" gap="1" rowGap="0">
          <span>{t("client:about.domains.signed_in_only")}</span>
          <Link href={`/login?${new URLSearchParams({ return_to: "/about" }).toString()}`} variant="standalone">
            {t("client:about.domains.sign_in")}
          </Link>
        </Cluster>
      );
    case "loading":
      return (
        <div aria-busy="true">
          <Spinner label={t("client:about.domains.loading")} />
        </div>
      );
    case "error":
      return (
        <RegionError
          heading={t("client:about.domains.failed")}
          headingLevel={3}
          reason={domains.reason}
          retryLabel={t("client:about.domains.retry")}
          onRetry={domains.retry}
          isRetrying={domains.isRetrying}
        />
      );
    case "ready":
      if (domains.items.length === 0) return <EmptyState headingLevel={3} heading={t("client:about.domains.empty")} />;
      return (
        <Stack gap="4">
          {domains.items.some(({ action }) => action.domain.includes("*")) && (
            <Hint>{t("client:about.domains.obfuscated")}</Hint>
          )}
          <Stack as="ul" divided>
            {domains.items.map(({ key, action }) => (
              <Stack as="li" key={key} gap="1" paddingY="3">
                <ModeratedDomain action={action} />
              </Stack>
            ))}
          </Stack>
          {domains.moreError !== undefined && (
            <RegionError heading={t("client:about.domains.more_failed")} reason={domains.moreError} />
          )}
          {domains.hasMore && (
            <div>
              <Button onPress={domains.loadMore} isPending={domains.isLoadingMore}>
                {t("client:about.domains.more")}
              </Button>
            </div>
          )}
        </Stack>
      );
  }
}

const kindLabels: Readonly<Record<PublicDomainAction["kind"], ClientMessageKey>> = {
  suspend: "client:about.domains.kind.suspend",
  limit: "client:about.domains.kind.limit",
};

function ModeratedDomain({ action }: { action: PublicDomainAction }) {
  const t = useT();
  return (
    <>
      <Cluster as="p" align="baseline" gap="3" rowGap="0">
        {/* A domain reads left to right in every locale; an obfuscated one is shown as the server sent it. */}
        <Text dir="ltr" weight="medium" wrap="anywhere">
          {domainToUnicode(action.domain)}
        </Text>
        <Hint as="span">{t(kindLabels[action.kind])}</Hint>
      </Cluster>
      {action.comment !== undefined && action.comment !== "" && (
        // The moderators' public comment, in any language (web-client/README.md Y10).
        <Text as="p" dir="auto" size="sm" wrap="anywhere">
          {action.comment}
        </Text>
      )}
    </>
  );
}
