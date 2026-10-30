// SPDX-License-Identifier: AGPL-3.0-only
import type { BadgeColor } from "@my/ui";
import { useQuery } from "@tanstack/react-query";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useConfig } from "../../../lib/config";
import { errorMessage } from "../../../lib/errors";
import { handleOf } from "../../../lib/handle";
import { domainToUnicode } from "../../../lib/idn";
import { imageSources } from "../../../lib/images";
import { useInstanceName } from "../../../lib/instanceName";
import { profilePath } from "../../../lib/paths";
import { shellQueryReader } from "../../../lib/query";
import {
  type Instance,
  instanceQueryOptions,
  overviewQueryOptions,
  type RecentPost,
  recentPostsQueryOptions,
} from "../api/landing";

/** Who may sign up (GET /instance `registrations.mode`). */
type RegistrationMode = Instance["registrations"]["mode"];

/** The server-rendered registration page (ADR-0030), which a link loads in place of the app. */
const signUpPath = "/auth/sign_up";

/** The sample of the local timeline: absent when the instance does not serve it (or has not said yet). */
export type RecentPosts =
  | { status: "off" }
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | { status: "ready"; posts: readonly RecentPost[] };

/** The account people contact about the server (the setting `contact_account`), as the landing page shows it. */
type Contact = {
  id: string;
  /** The display name, or the username when there is none. */
  name: string;
  /** The handle on this server (`@alice`). */
  handle: string;
  /** The full handle that people on other servers search for (`@alice@example.social`). */
  fullHandle: string;
  avatar: ReturnType<typeof imageSources>;
  href: string;
  /** The role badges the account shows, highest role first; their labels are the staff's text. */
  badges: { id: string; label: string; color: BadgeColor }[];
};

/** The counts of the server, while the server shows them (`public_counts`). */
type Counts = {
  members: number;
  /** The public posts of each of the last 7 days, the oldest first. */
  dailyPosts: readonly number[];
  /** Their sum. */
  weekPosts: number;
  /** When the server started: the ISO date for `<time>` and the month and year to show. */
  since: { dateTime: string; date: string };
};

export type Landing = {
  /** The server's name: from the instance, or the shell's runtime configuration until it loads. */
  name: string;
  /** The server's domain in Unicode; undefined until the instance loads. */
  domain: string | undefined;
  /** The one-paragraph summary, as plain text; "" when there is none or the instance has not loaded. */
  summary: string;
  /** The paragraphs of the full description (plain text, split at blank lines); empty when there is none. */
  description: readonly string[];
  rules: Instance["rules"];
  /** The image that represents the instance; undefined when there is none. */
  thumbnail: Instance["thumbnail"];
  /** Who may sign up; undefined until the instance loads (or when it cannot), when the page offers only to sign in. */
  registrationMode: RegistrationMode | undefined;
  /** Whether the server sends email (the confirmation of the address, the news of an approval). */
  sendsEmail: boolean;
  /** The address of the server's registration page. */
  signUpHref: string;
  /** Whether the local timeline is open to everyone, so that a visitor can look around. */
  localTimeline: boolean;
  /** The counts; undefined while they load, when they fail, and when the server hides them. */
  counts: Counts | undefined;
  /** The account to contact about the server; undefined while the instance loads, when it cannot, and when it has none. */
  contact: Contact | undefined;
  recentPosts: RecentPosts;
};

/**
 * The address of the server's registration page while the server takes new accounts (any mode but `closed`), for the
 * navigation of people who are not signed in; undefined while the instance loads, when it cannot, and when closed.
 */
export function useSignUpHref(): string | undefined {
  const api = useApi();
  const { apiOrigin } = useConfig();
  const { data: mode } = useQuery({
    ...instanceQueryOptions(api),
    ...shellQueryReader,
    select: (instance) => instance.registrations.mode,
  });
  return mode === undefined || mode === "closed" ? undefined : new URL(signUpPath, apiOrigin).href;
}

/** Splits a plain-text description into paragraphs at blank lines, without empty ones. */
function paragraphs(text: string | undefined): string[] {
  if (text === undefined) return [];
  return text
    .split(/\n\s*\n/)
    .map((paragraph) => paragraph.trim())
    .filter((paragraph) => paragraph !== "");
}

/**
 * What the landing page shows people who are not signed in: the instance from GET /instance (which the shell reads
 * on every page and the route's loader has asked for), its counts from GET /instance/overview, and a
 * sample of the local timeline when the instance lets everyone read it. A failure of the instance leaves the page with
 * the name and the way to sign in; a failure of the overview leaves out the counts; a failure of the
 * sample shows in its own part.
 */
export function useLanding(): Landing {
  const api = useApi();
  const intl = useIntl();
  const { apiOrigin } = useConfig();
  const name = useInstanceName();
  const { data: instance } = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  const { data: overview } = useQuery(overviewQueryOptions(api));
  const localOn = instance?.timelines.local === "everyone";
  const posts = useQuery({ ...recentPostsQueryOptions(api), enabled: localOn });

  let recentPosts: RecentPosts;
  if (!localOn) recentPosts = { status: "off" };
  else if (posts.data !== undefined) recentPosts = { status: "ready", posts: posts.data.items };
  else if (posts.error !== null)
    recentPosts = {
      status: "error",
      reason: errorMessage(posts.error),
      retry: () => void posts.refetch(),
      isRetrying: posts.isFetching,
    };
  else recentPosts = { status: "loading" };

  const domain = instance === undefined ? undefined : domainToUnicode(instance.domain);
  const counts = overview?.counts;
  const account = instance?.contact?.account;
  return {
    name,
    domain,
    summary: instance?.short_description?.trim() ?? "",
    description: paragraphs(instance?.description),
    rules: instance?.rules ?? [],
    thumbnail: instance?.thumbnail,
    registrationMode: instance?.registrations.mode,
    sendsEmail: instance?.sends_email ?? true,
    signUpHref: new URL(signUpPath, apiOrigin).href,
    localTimeline: localOn,
    counts:
      counts === undefined
        ? undefined
        : {
            members: counts.members,
            dailyPosts: counts.daily_posts,
            weekPosts: counts.daily_posts.reduce((sum, n) => sum + n, 0),
            since: {
              dateTime: counts.started_at,
              date: intl.formatDate(counts.started_at, { year: "numeric", month: "long" }),
            },
          },
    contact:
      account === undefined
        ? undefined
        : {
            id: account.id,
            name: account.display_name || account.username,
            handle: handleOf(account.acct),
            fullHandle: `@${account.username}@${domain ?? domainToUnicode(account.domain)}`,
            avatar: imageSources(account.avatar),
            href: profilePath(account.acct),
            badges: account.badges.map((b) => ({ id: b.role_id, label: b.label, color: b.color })),
          },
    recentPosts,
  };
}
