// SPDX-License-Identifier: AGPL-3.0-only
// The landing page (docs/architecture/web-client/screens/landing.md): the server's calling card, then three sections
// under a progress line (what kind of place this is, what people agree to, how to join). The footer with the links to
// the information page, the documents and the software the server runs is the shell's (routes/-layout/SiteFooter.tsx).
import {
  AccountItem,
  Box,
  Button,
  ButtonLink,
  Cluster,
  Feed,
  Heading,
  Hint,
  Link,
  PostSkeleton,
  ProfileHeader,
  RegionError,
  RoleBadgeList,
  Section,
  SectionProgress,
  Sparkline,
  Stack,
  StatTile,
  type Step,
  Steps,
  Text,
  VisuallyHidden,
  Wordmark,
} from "@my/ui";
import { type ReactNode, useId, useRef } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { useSignIn } from "../../session";
import { StatusCard } from "../../status";
import type { RecentPost } from "../api/landing";
import { type Landing as LandingData, type RecentPosts, useLanding } from "../hooks/useLanding";

export type LandingProps = {
  /** The page's heading (`PageHeading`) with the server's name, which the calling card carries. */
  heading: ReactNode;
};

/** The ids of the headings that the progress line and the calling card link to. */
const ids = {
  place: "landing-place",
  operators: "landing-operators",
  promise: "landing-promise",
  join: "landing-join",
  elsewhere: "landing-elsewhere",
} as const;

/**
 * A name inside a sentence of the UI's language, in Unicode directional isolates (FSI … PDI), so that a right-to-left
 * name does not reorder the sentence around it (web-client/README.md Y10).
 */
function isolate(name: string): string {
  return `⁨${name}⁩`;
}

/** A handle inside a sentence: it reads left to right whatever the sentence's direction (LRI … PDI). */
function isolateLtr(handle: string): string {
  return `⁦${handle}⁩`;
}

/** Whether people may sign up now; undefined (not yet known) counts as not. */
function takesAccounts(
  landing: LandingData,
): landing is LandingData & { registrationMode: "open" | "approval" | "invite" } {
  return landing.registrationMode !== undefined && landing.registrationMode !== "closed";
}

/**
 * The landing page for people who are not signed in (`/`). The software it runs is named only in the footer: the page
 * is about this server.
 */
export function Landing({ heading }: LandingProps) {
  const t = useT();
  const landing = useLanding();
  const placeRef = useRef<HTMLHeadingElement>(null);
  const promiseRef = useRef<HTMLHeadingElement>(null);
  const joinRef = useRef<HTMLHeadingElement>(null);
  return (
    <Stack gap="8">
      <CallingCard heading={heading} landing={landing} />
      <SectionProgress
        label={t("client:landing.progress.label")}
        parts={[
          { id: ids.place, title: t("client:landing.place.title") },
          { id: ids.promise, title: t("client:landing.promise.title") },
          { id: ids.join, title: t("client:landing.join.title") },
        ]}
      >
        {/* The headings take focus from the links of the calling card and of the progress line. */}
        <Section title={t("client:landing.place.title")} headingId={ids.place} headingRef={placeRef}>
          <Place landing={landing} />
        </Section>
        <Section title={t("client:landing.promise.title")} headingId={ids.promise} headingRef={promiseRef}>
          <Rules rules={landing.rules} />
        </Section>
        <Section title={t("client:landing.join.title")} headingId={ids.join} headingRef={joinRef}>
          <Join landing={landing} />
        </Section>
      </SectionProgress>
    </Stack>
  );
}

/**
 * The calling card, in the look of a profile: the server's image, its mark, name, domain, summary and counts, with
 * signing in at the top end. Joining moves down to the section on how to join, past the rules; while the server takes no
 * new accounts (or has not said yet), signing in is the primary action.
 */
function CallingCard({ heading, landing }: { heading: ReactNode; landing: LandingData }) {
  const t = useT();
  // After signing in, the root address opens the home timeline.
  const { signIn, isPending } = useSignIn("/");
  const open = takesAccounts(landing);
  const { counts, operators } = landing;
  const [firstOperator] = operators;
  return (
    <Stack gap="4">
      <ProfileHeader
        {...(landing.thumbnail !== undefined && {
          image: { still: [{ url: landing.thumbnail.url, width: landing.thumbnail.width }] },
        })}
        // The server's mark: the custom icon of the server takes this place once there is one.
        mark={<Wordmark name="my" decorative size="sm" />}
        name={landing.name}
        heading={heading}
        {...(landing.domain !== undefined && { handle: landing.domain })}
        actions={
          <Button variant={open ? "secondary" : "primary"} onPress={signIn} isPending={isPending}>
            {t("client:landing.sign_in")}
          </Button>
        }
        {...(landing.summary !== "" && {
          // The administrator's text, in a language the app does not know: lang="" rather than the app's (Y10).
          note: (
            <Text as="p" dir="auto" lang="" size="lg">
              {landing.summary}
            </Text>
          ),
        })}
        counts={[
          ...(counts === undefined
            ? []
            : [
                {
                  key: "members",
                  content: <Text weight="semibold">{t("client:landing.members", { count: counts.members })}</Text>,
                },
                {
                  key: "since",
                  content: <Text tone="muted">{t("client:landing.since", { date: counts.since.date })}</Text>,
                },
              ]),
          ...(firstOperator === undefined
            ? []
            : [
                {
                  key: "operators",
                  content: (
                    <Link href={`#${ids.operators}`} variant="standalone">
                      {t("client:landing.run_by", { name: isolate(firstOperator.name), others: operators.length - 1 })}
                    </Link>
                  ),
                },
              ]),
        ]}
      />
      <Box paddingX="4">
        <Cluster gap="4" align="center">
          {open ? (
            <ButtonLink href={`#${ids.join}`} variant="primary" size="lg">
              {t("client:landing.join")}
            </ButtonLink>
          ) : (
            landing.registrationMode === "closed" && (
              <Hint size="inherit">{t("client:landing.registrations.closed")}</Hint>
            )
          )}
          <Link href={`#${ids.elsewhere}`} variant="standalone">
            {t("client:landing.elsewhere.link")}
          </Link>
        </Cluster>
      </Box>
    </Stack>
  );
}

/** ① What kind of place this is: the description, who runs it, and what people post. */
function Place({ landing }: { landing: LandingData }) {
  return (
    <>
      {landing.description.map((paragraph) => (
        // Paragraphs of one text have no IDs; the text itself is the key (two equal paragraphs are not expected).
        <Text as="p" key={paragraph} dir="auto" lang="" whitespace="preLine">
          {paragraph}
        </Text>
      ))}
      {landing.operators.length > 0 && <Operators operators={landing.operators} />}
      <Activity landing={landing} />
    </>
  );
}

function Operators({ operators }: { operators: LandingData["operators"] }) {
  const t = useT();
  return (
    <Stack gap="3">
      <Heading level={3} id={ids.operators} focusable>
        {t("client:landing.operators.title")}
      </Heading>
      <Stack as="ul" gap="3" aria-labelledby={ids.operators}>
        {operators.map((operator) => (
          <li key={operator.id}>
            <AccountItem
              name={operator.name}
              handle={operator.handle}
              avatar={operator.avatar}
              avatarSeed={operator.id}
              href={operator.href}
              trailingPlacement="below"
              trailing={<RoleBadgeList label={t("client:profile.badges")} badges={operator.badges} />}
            />
          </li>
        ))}
      </Stack>
    </Stack>
  );
}

/** Placeholder cards while the recent posts load. */
const placeholders = ["first", "second"] as const;

/**
 * What people post: the public posts of the last 7 days as a number and its line, and the newest posts. A quiet or new
 * server, with no recent public post, shows none of it rather than an empty list.
 */
function Activity({ landing }: { landing: LandingData }) {
  const t = useT();
  const intl = useIntl();
  const headingId = useId();
  const { counts, recentPosts } = landing;
  const noPosts = recentPosts.status === "off" || (recentPosts.status === "ready" && recentPosts.posts.length === 0);
  const quiet = counts === undefined || counts.weekPosts === 0;
  if (noPosts && quiet) return null;
  return (
    <Stack gap="3">
      <Heading level={3} id={headingId}>
        {t("client:landing.activity.title")}
      </Heading>
      {counts !== undefined && (
        <StatTile label={t("client:landing.activity.week")} value={intl.formatNumber(counts.weekPosts)}>
          <Sparkline values={counts.dailyPosts} />
        </StatTile>
      )}
      {!noPosts && <RecentPostsBody posts={recentPosts} headingId={headingId} />}
      {landing.localTimeline && (
        <p>
          <Link href="/local" variant="standalone">
            {t("client:landing.posts.more")}
          </Link>
        </p>
      )}
    </Stack>
  );
}

function RecentPostsBody({ posts, headingId }: { posts: Exclude<RecentPosts, { status: "off" }>; headingId: string }) {
  const t = useT();
  switch (posts.status) {
    case "loading":
      return (
        <Stack aria-busy="true">
          <VisuallyHidden>{t("client:timeline.loading")}</VisuallyHidden>
          {placeholders.map((key) => (
            <PostSkeleton key={key} />
          ))}
        </Stack>
      );
    case "error":
      return (
        <RegionError
          heading={t("client:landing.posts.failed")}
          headingLevel={4}
          reason={posts.reason}
          retryLabel={t("client:timeline.retry")}
          onRetry={posts.retry}
          isRetrying={posts.isRetrying}
        />
      );
    case "ready":
      return (
        <Stack>
          {/* The feed role has no keyboard convention, so its keys are told before it (Feed's documentation). */}
          <VisuallyHidden>
            <p>{t("client:timeline.keyboard_hint")}</p>
          </VisuallyHidden>
          <Feed<RecentPost>
            aria-labelledby={headingId}
            items={posts.posts}
            getKey={(post) => post.id}
            // A sample to read: acting on a post takes signing in first.
            renderItem={(post) => <StatusCard status={post} withActions={false} />}
            isLoading={false}
            hasMore={false}
            onLoadMore={() => undefined}
            loadMoreLabel={t("client:timeline.load_more")}
            loadingLabel={t("client:timeline.loading")}
          />
        </Stack>
      );
  }
}

/** ② What people agree to: the server's rules, numbered. */
function Rules({ rules }: { rules: LandingData["rules"] }) {
  const t = useT();
  if (rules.length === 0) return <Hint size="inherit">{t("client:landing.rules.none")}</Hint>;
  return (
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
  );
}

/** The sign-up link's text for each mode in which people may sign up. */
const signUpLabels = {
  open: "client:landing.sign_up.open",
  approval: "client:landing.sign_up.approval",
  invite: "client:landing.sign_up.invite",
} as const;

/** What happens after joining, in the server's registration mode, as the keys of the steps' messages. */
function joinSteps(mode: "open" | "approval" | "invite", sendsEmail: boolean) {
  switch (mode) {
    case "open":
      // Without email, a moderator approves each new account (docs/operations/registration.md).
      return [
        "client:landing.steps.form",
        sendsEmail ? "client:landing.steps.confirm_email" : "client:landing.steps.review",
        "client:landing.steps.start",
      ] as const;
    case "approval":
      return [
        "client:landing.steps.apply",
        "client:landing.steps.review",
        sendsEmail ? "client:landing.steps.approved_email" : "client:landing.steps.approved_sign_in",
      ] as const;
    case "invite":
      return [
        "client:landing.steps.invite_get",
        "client:landing.steps.invite_open",
        "client:landing.steps.start",
      ] as const;
  }
}

/**
 * ③ How to join: the steps of the server's registration mode with its action at the end, then how to follow people
 * here from an account on another server, which needs no account here.
 */
function Join({ landing }: { landing: LandingData }) {
  const t = useT();
  const elsewhereId = ids.elsewhere;
  const example = landing.operators[0]?.fullHandle;
  const elsewhere: Step[] = [
    {
      key: "find",
      title: t("client:landing.elsewhere.find"),
      ...(landing.localTimeline && {
        detail: (
          <Link href="/local" variant="standalone">
            {t("client:landing.posts.more")}
          </Link>
        ),
      }),
    },
    {
      key: "copy",
      title:
        example === undefined
          ? t("client:landing.elsewhere.copy_plain")
          : t("client:landing.elsewhere.copy", { handle: isolateLtr(example) }),
    },
    {
      key: "search",
      title: t("client:landing.elsewhere.search"),
      detail: t("client:landing.elsewhere.search.detail"),
    },
  ];
  return (
    <>
      {takesAccounts(landing) ? (
        <Steps
          steps={joinSteps(landing.registrationMode, landing.sendsEmail).map((key) => ({ key, title: t(key) }))}
          action={
            // The registration page is the server's own; the link loads it in place of the app (lib/paths.ts).
            <ButtonLink href={landing.signUpHref} variant="primary">
              {t(signUpLabels[landing.registrationMode])}
            </ButtonLink>
          }
        />
      ) : (
        landing.registrationMode === "closed" && <Hint size="inherit">{t("client:landing.registrations.closed")}</Hint>
      )}
      <Stack gap="3">
        <Heading level={3} id={elsewhereId} focusable>
          {t("client:landing.elsewhere.title")}
        </Heading>
        <Steps steps={elsewhere} labelledBy={elsewhereId} />
      </Stack>
    </>
  );
}
