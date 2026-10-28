// SPDX-License-Identifier: AGPL-3.0-only
import {
  AccountItem,
  Button,
  Cluster,
  EmptyState,
  Hint,
  Link,
  RegionError,
  Section,
  Spinner,
  Stack,
  Switch,
  Text,
} from "@my/ui";
import { handleOf } from "../../../lib/handle";
import { useT } from "../../../lib/i18n";
import { imageSources } from "../../../lib/images";
import { profilePath, signInPath } from "../../../lib/paths";
import { useFormatMoney } from "../../costs";
import type { SupporterListEntry } from "../api/supporters";
import {
  type SupporterGroup,
  useAboutSupporters,
  useSupporterList,
  useSupporterListing,
} from "../hooks/useSupporterList";

/**
 * The body of the supporter list (`/about/supporters`, ADR-0071 §4): the members who chose to be listed, under a
 * heading per plan, each as their avatar, name and handle linking to their profile, and never an amount, a date or a
 * count. The instance's setting decides who may read it.
 */
export function SupporterListView() {
  const t = useT();
  const list = useSupporterList();
  switch (list.status) {
    case "hidden":
      return (
        <EmptyState
          heading={t("client:supporters.list.hidden")}
          action={
            <Link href="/about" variant="standalone">
              {t("client:supporters.list.about")}
            </Link>
          }
        />
      );
    case "signed-in-only":
      return (
        <Cluster as="p" gap="1" rowGap="0">
          <span>{t("client:supporters.list.signed_in_only")}</span>
          <Link href={signInPath("/about/supporters")} variant="standalone">
            {t("client:supporters.list.sign_in")}
          </Link>
        </Cluster>
      );
    case "loading":
      return (
        <div aria-busy="true">
          <Spinner label={t("client:supporters.list.loading")} />
        </div>
      );
    case "error":
      return (
        <RegionError
          heading={t("client:supporters.list.failed")}
          reason={list.reason}
          retryLabel={t("client:supporters.list.retry")}
          onRetry={list.retry}
          isRetrying={list.isRetrying}
        />
      );
    case "ready":
      return (
        <Stack gap="8">
          <Hint size="inherit">{t("client:supporters.list.description")}</Hint>
          {list.groups.length === 0 ? (
            <EmptyState heading={t("client:supporters.list.empty")} />
          ) : (
            list.groups.map((group) => <PlanGroup key={group.plan.id} group={group} />)
          )}
          {list.moreError !== undefined && (
            <RegionError heading={t("client:supporters.list.more_failed")} reason={list.moreError} />
          )}
          {list.hasMore && (
            <div>
              <Button onPress={list.loadMore} isPending={list.isLoadingMore}>
                {t("client:supporters.list.more")}
              </Button>
            </div>
          )}
        </Stack>
      );
  }
}

function PlanGroup({ group }: { group: SupporterGroup }) {
  // The administrators' name of the plan, in a language the app does not know (web-client/README.md Y10).
  return (
    <Section
      title={
        <span dir="auto" lang="">
          {group.plan.name}
        </span>
      }
    >
      <Stack as="ul" gap="3">
        {group.members.map((member) => (
          <li key={member.account.id}>
            <Member member={member} />
          </li>
        ))}
      </Stack>
    </Section>
  );
}

/** A member as one link to their profile: the avatar, the name and the handle. */
function Member({ member }: { member: SupporterListEntry }) {
  const { account } = member;
  return (
    <AccountItem
      name={account.display_name || account.username}
      handle={handleOf(account.acct)}
      avatar={imageSources(account.avatar)}
      avatarSeed={account.id}
      href={profilePath(account.acct)}
      layout="inline"
    />
  );
}

/**
 * The supporter program on the information page (ADR-0072 §14, ADR-0071 §4): the plans that take subscriptions with
 * their prices and the way to subscribe, which the Tauri app does not show, and the link to the supporter list while
 * the instance publishes it. Absent when there is neither.
 */
export function AboutSupportersSection() {
  const t = useT();
  const format = useFormatMoney();
  const about = useAboutSupporters();
  const plans = about.plans;
  if ((plans === undefined || plans.length === 0) && !about.listPublished) return null;
  return (
    <Section title={t("client:supporters.about.title")}>
      {plans !== undefined && plans.length > 0 && (
        <>
          <p>{t("client:supporters.about.description")}</p>
          <Stack as="ul" gap="3">
            {plans.map((plan) => (
              <Stack as="li" gap="1" key={plan.id}>
                <Text weight="medium" dir="auto" lang="">
                  {plan.name}
                </Text>
                <Hint as="span">
                  {t("client:supporter.plans.prices", {
                    monthly: t("client:supporter.per_month", { amount: format(plan.monthly_price) }),
                    yearly: t("client:supporter.per_year", { amount: format(plan.yearly_price) }),
                  })}
                </Hint>
                {plan.description !== "" && (
                  <Text size="sm" whitespace="preLine" dir="auto" lang="">
                    {plan.description}
                  </Text>
                )}
              </Stack>
            ))}
          </Stack>
          <p>
            <Link href="/settings/supporter" variant="standalone">
              {t("client:supporters.about.subscribe")}
            </Link>
          </p>
        </>
      )}
      {about.listPublished && (
        <p>
          <Link href="/about/supporters" variant="standalone">
            {t("client:supporters.about.list")}
          </Link>
        </p>
      )}
    </Section>
  );
}

/**
 * The user's choice to be on the supporter list (ADR-0071 §4), on the badges settings page: one switch, saved as soon as
 * it changes. Shown only to a member of a plan while the instance publishes the list; it stays in the Tauri app.
 */
export function SupporterListingSettings() {
  const t = useT();
  const listing = useSupporterListing();
  if (listing === undefined) return null;
  return (
    <Section title={t("client:supporter.listing.title")}>
      <Hint>{t("client:supporter.listing.description")}</Hint>
      <Switch
        isSelected={listing.listed}
        onChange={listing.onChange}
        {...(listing.error !== undefined && {
          isInvalid: true,
          errorMessage: t("client:supporter.listing.failed", { reason: listing.error }),
        })}
      >
        {t("client:supporter.listing.switch")}
      </Switch>
    </Section>
  );
}
