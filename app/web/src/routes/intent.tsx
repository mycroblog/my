// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";
import { useReply } from "../features/compose";
import { ActivityIntent, type ActivityIntentKind, maxSearchLength } from "../features/search";
import { statusPath } from "../features/status";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { HomeLink } from "./-layout/HomeLink";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The templates of the Activity Intents that WebFinger publishes (FEP-3b86, federation.md §6.1): `follow` is the
 * account a follow button of another server names (`{object}` of the Follow intent), and `reply` the post a reply
 * button names (`{inReplyTo}` of the Create intent). Anything else is dropped (web-client/README.md S3).
 */
const target = z.string().trim().min(1).max(maxSearchLength).optional().catch(undefined);
const searchSchema = z.object({ follow: target, reply: target });

/**
 * Where the follow and reply buttons of other servers send a user of this server: after signing in, it shows what
 * the button asks for, and its button opens the profile to follow or the post with a reply started
 * (web-client/screens/intent.md).
 */
export const Route = createFileRoute("/intent")({
  staticData: { title: "client:intent.title", back: true },
  validateSearch: searchSchema,
  beforeLoad: requireSignIn,
  component: IntentPage,
});

function IntentPage() {
  const t = useT();
  const { follow, reply: inReplyTo } = Route.useSearch();
  const navigate = Route.useNavigate();
  const reply = useReply();
  const kind: ActivityIntentKind | null = follow !== undefined ? "follow" : inReplyTo !== undefined ? "reply" : null;
  return (
    <>
      <PageHeading>{t("client:intent.title")}</PageHeading>
      {kind === null ? (
        <>
          <p>{t("client:intent.empty")}</p>
          <HomeLink />
        </>
      ) : (
        <ActivityIntent
          // Another address is another lookup: nothing of the previous one carries over (web-client/README.md R10).
          key={`${kind} ${follow ?? inReplyTo ?? ""}`}
          kind={kind}
          target={follow ?? inReplyTo ?? ""}
          // The page has done its part: going back leaves it for the page of the other server.
          onAccount={(acct) => void navigate({ to: "/@{$acct}", params: { acct }, replace: true })}
          onStatus={(status) => {
            void navigate({ to: statusPath(status), replace: true }).then(() => reply?.(status));
          }}
        />
      )}
    </>
  );
}
