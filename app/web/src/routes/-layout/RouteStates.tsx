// SPDX-License-Identifier: AGPL-3.0-only
// What a route shows instead of its page: while it loads, when it fails, and for an unknown address
// (web-client/README.md E1, E3, web-client/16-look-states-and-wording.md G16). Each has the page's h1, so the layout moves focus to it and titles the document.
import { RegionError, RouteLoading, StandalonePage, Text } from "@my/ui";
import { useQueryClient, useQueryErrorResetBoundary } from "@tanstack/react-query";
import { type ErrorComponentProps, useMatch, useRouter } from "@tanstack/react-router";
import { type ReactNode, useState } from "react";
import { type ErrorKind, errorKind, errorMessage } from "../../lib/errors";
import { type PlainClientMessageKey, useT } from "../../lib/i18n";
import { useOnReconnect } from "../../lib/network";
import { PageHeading } from "./PageHeading";
import { HomeLink } from "./HomeLink";

/** The router's defaultPendingComponent: shown only when a page takes longer than defaultPendingMs to load. */
export function RoutePending() {
  const t = useT();
  return <RouteLoading label={t("client:shell.loading")} />;
}

/** What happened, for each kind of failure (web-client/16-look-states-and-wording.md G16). */
const errorHeadings: Readonly<Record<ErrorKind, PlainClientMessageKey>> = {
  network: "client:shell.error.network.heading",
  server: "client:shell.error.server.heading",
  "not-found": "client:shell.error.not_found.heading",
  forbidden: "client:shell.error.forbidden.heading",
  closed: "client:shell.error.closed.heading",
  unexpected: "client:shell.error.unexpected.heading",
};

/** The failures that another try may get past; trying again finds the same 403 or 404. */
const retryableKinds: ReadonlySet<ErrorKind> = new Set(["network", "server", "unexpected"]);

export type RouteErrorProps = ErrorComponentProps & {
  /** The page's h1, for a page whose name depends on its params; others name it in the route's `staticData.title`. */
  heading?: ReactNode;
};

/**
 * The error of one page, inside the layout so that the navigation stays usable (the router's
 * defaultErrorComponent). The page keeps its h1, so the layout still focuses it and names the document after
 * the page, and the error takes the place of the page's content: what happened, why, and "Try again"
 * (web-client/16-look-states-and-wording.md G16).
 */
export function RouteError({ error: pageError, reset, heading }: RouteErrorProps) {
  const t = useT();
  const router = useRouter();
  const queryClient = useQueryClient();
  const queryErrorResetBoundary = useQueryErrorResetBoundary();
  // The error component renders inside the failed route's match (TanStack Router, "Static Route Data").
  const title = useMatch({ strict: false, select: (match) => match.staticData.title });
  /** The error of the last retry, which failed again; until then the page's error is shown. */
  const [retryError, setRetryError] = useState<{ error: unknown } | null>(null);
  const [isRetrying, setIsRetrying] = useState(false);
  const error = retryError === null ? pageError : retryError.error;
  const kind = errorKind(error);

  const retry = async () => {
    setIsRetrying(true);
    // The queries that failed with this error load again first, in place: until it is known whether the page
    // can load, the error and focus on its button stay, and a retry that fails again is the user's result.
    const failed = queryClient.getQueryCache().findAll({ predicate: (query) => query.state.error === error });
    await Promise.all(failed.map((query) => queryClient.refetchQueries({ queryKey: query.queryKey, exact: true, type: "all" })));
    const again = failed.find((query) => query.state.status === "error");
    if (again !== undefined) {
      setRetryError({ error: again.state.error });
      setIsRetrying(false);
      return;
    }
    // Queries that failed into this boundary refetch instead of throwing their cached error again, and
    // invalidate() runs the loaders again, which find the data just loaded (TanStack Router docs, "Data
    // Loading" with TanStack Query).
    queryErrorResetBoundary.reset();
    reset();
    await router.invalidate();
    // The page, or its error, has taken the place of this one: focus its heading, as the layout does after a
    // navigation (web-client/README.md Y5); the button that had focus is gone.
    document.querySelector<HTMLElement>("main h1")?.focus();
  };
  // A page that failed for the network loads again by itself when the app is back online (web-client/16-look-states-and-wording.md G17).
  useOnReconnect(() => {
    void retry();
  }, kind === "network" && !isRetrying);

  return (
    <>
      <PageHeading>{heading ?? t(title ?? "client:shell.error.title")}</PageHeading>
      <RegionError
        headingLevel={2}
        heading={t(errorHeadings[kind])}
        // A failed request's message is localized and says why (for the network, whether the app is offline);
        // anything else is a fault of the page, and its message is not for the user.
        reason={kind === "unexpected" ? t("client:shell.error.unexpected.reason") : errorMessage(error)}
        {...(retryableKinds.has(kind) && {
          retryLabel: t("client:shell.error.retry"),
          onRetry: () => {
            void retry();
          },
          isRetrying,
        })}
        alert={retryError !== null && !isRetrying}
      />
    </>
  );
}

/** The error of the layout itself: the whole page, since the layout cannot be shown. */
export function RootError(props: ErrorComponentProps) {
  return (
    <StandalonePage>
      <RouteError {...props} />
    </StandalonePage>
  );
}

/** An address that no route matches. */
export function NotFound() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:shell.not_found.title")}</PageHeading>
      <Text as="p">{t("client:shell.not_found.body")}</Text>
      <HomeLink />
    </>
  );
}
