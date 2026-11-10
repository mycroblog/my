// SPDX-License-Identifier: AGPL-3.0-only
// The hashtags a profile features (ADR-0006 §17; web-client/screens/profile.md): what the profile shows of each, in the
// owner's order. The region is secondary to the profile, so it shows nothing until they load, and nothing when they fail.
import { useQuery } from "@tanstack/react-query";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useT } from "../../../lib/i18n";
import { tagPath } from "../../../lib/paths";
import { accountFeaturedTagsQueryOptions } from "../api/tags";

/** A featured hashtag as the profile shows it. */
export type FeaturedTagView = {
  name: string;
  /** The hashtag's page. */
  href: string;
  /** How many of the account's posts carry it, and when it was last used, in words. */
  stats: string;
};

/** The hashtags the account `accountId` features, in its order; empty while they load or when they fail. */
export function useAccountFeaturedTags(accountId: string): FeaturedTagView[] {
  const api = useApi();
  const t = useT();
  const intl = useIntl();
  const { data } = useQuery(accountFeaturedTagsQueryOptions(api, accountId));
  return (data?.items ?? []).map((tag) => ({
    name: tag.name,
    href: tagPath(tag.name),
    stats:
      tag.last_status_at === undefined
        ? t("client:tag.featured.count", { count: tag.statuses_count })
        : t("client:tag.featured.stats", {
            count: tag.statuses_count,
            date: intl.formatDate(tag.last_status_at, { dateStyle: "medium" }),
          }),
  }));
}
