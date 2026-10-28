// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useConfig } from "../../../lib/config";
import { errorMessage } from "../../../lib/errors";
import { shellQueryReader } from "../../../lib/query";
import { type Instance, instanceQueryOptions, type RecentPost, recentPostsQueryOptions } from "../api/landing";

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

export type Landing = {
  /** The one-paragraph summary, as plain text; "" when there is none or the instance has not loaded. */
  summary: string;
  /** The paragraphs of the full description (plain text, split at blank lines); empty when there is none. */
  description: readonly string[];
  rules: Instance["rules"];
  /** The image that represents the instance; undefined when there is none. */
  thumbnail: Instance["thumbnail"];
  /** Who may sign up; undefined until the instance loads (or when it cannot), when the page offers only to sign in. */
  registrationMode: RegistrationMode | undefined;
  /** The address of the server's registration page. */
  signUpHref: string;
  recentPosts: RecentPosts;
  /** Where users get the source code the server runs (AGPL-3.0 section 13); undefined until the instance loads. */
  sourceUrl: string | undefined;
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
 * on every page and the route's loader has asked for), and a sample of the local timeline when the instance lets
 * everyone read it. A failure of the instance leaves the page with the name and the way to sign in; a failure of the sample shows
 * in its own section.
 */
export function useLanding(): Landing {
  const api = useApi();
  const { apiOrigin } = useConfig();
  const { data: instance } = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
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

  return {
    summary: instance?.short_description?.trim() ?? "",
    description: paragraphs(instance?.description),
    rules: instance?.rules ?? [],
    thumbnail: instance?.thumbnail,
    registrationMode: instance?.registrations.mode,
    signUpHref: new URL(signUpPath, apiOrigin).href,
    recentPosts,
    sourceUrl: instance?.source_url,
  };
}
