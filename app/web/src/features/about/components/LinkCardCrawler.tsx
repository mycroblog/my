// SPDX-License-Identifier: AGPL-3.0-only
import {
  Callout,
  CodeBlock,
  Heading,
  Hint,
  Link,
  RegionError,
  Section,
  Spinner,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableColumnHeader,
  TableHead,
  TableRow,
  TableRowHeader,
  Text,
} from "@my/ui";
import { useT } from "../../../lib/i18n";
import { aboutPath } from "../../../lib/paths";
import { useLinkCardCrawler } from "../hooks/useLinkCardCrawler";
import { ContactDetails } from "./About";

/** The examples are what a website's operator copies, the same in every language. */
const robotsExample = "User-agent: my-linkcard\nDisallow: /";
const directivesExample = `<meta name="robots" content="nosnippet">
<meta name="my-linkcard" content="noimageindex">
X-Robots-Tag: my-linkcard: nosnippet`;

/**
 * The body of the page about the link card crawler (`/about/crawler`), under its heading: first what the crawler does
 * on this server by its settings, then what a website's operator sees, how to refuse it, and whom to ask
 * (web-client/screens/about-crawler.md). Nothing says that this server obeys robots.txt until its settings have been
 * read, so a failure takes the place of the whole body.
 */
export function LinkCardCrawler() {
  const t = useT();
  const crawler = useLinkCardCrawler();
  switch (crawler.status) {
    case "loading":
      return (
        <div aria-busy="true">
          <Spinner label={t("client:about.link_card_crawler.loading")} />
        </div>
      );
    case "error":
      return (
        <RegionError
          heading={t("client:about.link_card_crawler.failed")}
          headingLevel={2}
          reason={crawler.reason}
          retryLabel={t("client:about.link_card_crawler.retry")}
          onRetry={crawler.retry}
          isRetrying={crawler.isRetrying}
        />
      );
    case "ready":
      return (
        <Stack gap="8">
          <Stack gap="3">
            <Text as="p" size="lg">
              {t("client:about.link_card_crawler.intro")}
            </Text>
            <Callout>
              <p>
                {crawler.enabled
                  ? crawler.ignoresRobotsTxt
                    ? t("client:about.link_card_crawler.state.ignores")
                    : t("client:about.link_card_crawler.state.obeys")
                  : t("client:about.link_card_crawler.state.off")}
              </p>
            </Callout>
          </Stack>
          <Section title={t("client:about.link_card_crawler.identity.title")}>
            <p>{t("client:about.link_card_crawler.identity.user_agent")}</p>
            <CodeBlock dir="ltr">{crawler.userAgent}</CodeBlock>
            <p>{t("client:about.link_card_crawler.identity.token")}</p>
            <Hint>{t("client:about.link_card_crawler.identity.others")}</Hint>
          </Section>
          <Section title={t("client:about.link_card_crawler.trigger.title")}>
            <p>{t("client:about.link_card_crawler.trigger.post")}</p>
            <p>{t("client:about.link_card_crawler.trigger.remote")}</p>
          </Section>
          <Section title={t("client:about.link_card_crawler.fetches.title")}>
            <Stack as="ul" marker="disc" gap="2">
              <li>{t("client:about.link_card_crawler.fetches.page")}</li>
              <li>{t("client:about.link_card_crawler.fetches.image")}</li>
              {!crawler.ignoresRobotsTxt && <li>{t("client:about.link_card_crawler.fetches.robots")}</li>}
            </Stack>
            <p>{t("client:about.link_card_crawler.fetches.nothing_else")}</p>
          </Section>
          <Section title={t("client:about.link_card_crawler.sends.title")}>
            <p>{t("client:about.link_card_crawler.sends.headers")}</p>
            <p>{t("client:about.link_card_crawler.sends.not")}</p>
          </Section>
          <Section title={t("client:about.link_card_crawler.frequency.title")}>
            <Stack as="ul" marker="disc" gap="2">
              <li>{t("client:about.link_card_crawler.frequency.once")}</li>
              <li>{t("client:about.link_card_crawler.frequency.failure")}</li>
              <li>{t("client:about.link_card_crawler.frequency.host")}</li>
              <li>{t("client:about.link_card_crawler.frequency.throttled")}</li>
              {!crawler.ignoresRobotsTxt && <li>{t("client:about.link_card_crawler.frequency.robots")}</li>}
            </Stack>
          </Section>
          <OptOut ignoresRobotsTxt={crawler.ignoresRobotsTxt} />
          <Section title={t("client:about.link_card_crawler.contact.title")}>
            <p>{t("client:about.link_card_crawler.contact.intro")}</p>
            {crawler.contact === undefined ? (
              <>
                <p>{t("client:about.link_card_crawler.contact.none")}</p>
                <p>
                  <Link href={aboutPath} variant="standalone">
                    {t("client:about.link_card_crawler.contact.about")}
                  </Link>
                </p>
              </>
            ) : (
              <ContactDetails contact={crawler.contact} />
            )}
          </Section>
        </Stack>
      );
  }
}

/** How a website refuses the crawler: in robots.txt, unless this server does not read it, and in the page's directives. */
function OptOut({ ignoresRobotsTxt }: { ignoresRobotsTxt: boolean }) {
  const t = useT();
  return (
    <Section title={t("client:about.link_card_crawler.opt_out.title")}>
      <Stack gap="3">
        <Heading level={3}>{t("client:about.link_card_crawler.opt_out.robots.title")}</Heading>
        {ignoresRobotsTxt && (
          <Callout>
            <p>{t("client:about.link_card_crawler.opt_out.robots.ignored")}</p>
          </Callout>
        )}
        <p>{t("client:about.link_card_crawler.opt_out.robots.intro")}</p>
        <CodeBlock dir="ltr">{robotsExample}</CodeBlock>
        <p>{t("client:about.link_card_crawler.opt_out.robots.star")}</p>
        {!ignoresRobotsTxt && <p>{t("client:about.link_card_crawler.opt_out.robots.timing")}</p>}
      </Stack>
      <Stack gap="3">
        <Heading level={3}>{t("client:about.link_card_crawler.opt_out.directives.title")}</Heading>
        <p>{t("client:about.link_card_crawler.opt_out.directives.intro")}</p>
        <Table caption={t("client:about.link_card_crawler.opt_out.directives.caption")}>
          <TableHead>
            <TableColumnHeader>{t("client:about.link_card_crawler.opt_out.directives.directive")}</TableColumnHeader>
            <TableColumnHeader>{t("client:about.link_card_crawler.opt_out.directives.effect")}</TableColumnHeader>
          </TableHead>
          <TableBody>
            <TableRow>
              <TableRowHeader>
                <Directives names={["nosnippet", "max-snippet:0"]} />
              </TableRowHeader>
              <TableCell>{t("client:about.link_card_crawler.opt_out.directives.nosnippet")}</TableCell>
            </TableRow>
            <TableRow>
              <TableRowHeader>
                <Directives names={["noimageindex", "max-image-preview:none"]} />
              </TableRowHeader>
              <TableCell>{t("client:about.link_card_crawler.opt_out.directives.noimageindex")}</TableCell>
            </TableRow>
            <TableRow>
              <TableRowHeader>
                <Directives names={["noindex", "none"]} />
              </TableRowHeader>
              <TableCell>
                {ignoresRobotsTxt
                  ? t("client:about.link_card_crawler.opt_out.directives.noindex_ignored")
                  : t("client:about.link_card_crawler.opt_out.directives.noindex")}
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <p>{t("client:about.link_card_crawler.opt_out.directives.rest")}</p>
        <p>{t("client:about.link_card_crawler.opt_out.directives.where")}</p>
        <CodeBlock dir="ltr">{directivesExample}</CodeBlock>
      </Stack>
    </Section>
  );
}

/** The directives of one row, as a website's operator writes them: left to right in every locale. */
function Directives({ names }: { names: readonly string[] }) {
  return (
    <Stack gap="1">
      {names.map((name) => (
        <Text key={name} as="p" dir="ltr" align="start" wrap="anywhere">
          {name}
        </Text>
      ))}
    </Stack>
  );
}
