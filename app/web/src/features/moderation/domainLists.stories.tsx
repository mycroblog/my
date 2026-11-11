// SPDX-License-Identifier: AGPL-3.0-only
// The allowlist of federation and the blocklist subscriptions beside the domain actions
// (/moderation/domain-actions/allowed and /moderation/domain-actions/subscriptions; web-client/screens/
// moderation-domain-allows.md, moderation-domain-list-subscriptions.md, moderation-domain-list-subscription-new.md,
// moderation-domain-list-subscription.md): the allowed domains, the subscriptions, the form of a new one, and the page
// of one with its drafts and fetches, in the states of ADR-0165 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { longDomainAccountFixture } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import {
  answering,
  failingOn,
  inLocale,
  loadingOn,
  narrow,
  renderScreen,
  screenLoader,
  settled,
} from "../../test/stories";
import {
  domainAllowFixture,
  domainListDraftFixture,
  domainListFetchFixture,
  domainListSubscriptionFixture,
} from "./test/handlers";
import { hoursAgo, moderatorViewer, showing, storyId, withoutPermission } from "./test/stories";

const storyAllows = [
  domainAllowFixture({ domain: "friend.example", comment: "Our partner server.", created_at: hoursAgo(300) }),
  domainAllowFixture({ domain: longDomainAccountFixture.domain, created_at: hoursAgo(200) }),
  domainAllowFixture({ domain: "xn--r8jz45g.jp", comment: "姉妹サーバー", created_at: hoursAgo(20) }),
];

const sharedFetch = domainListFetchFixture({ id: storyId("f0", 1), entry_count: 312, fetched_at: hoursAgo(3) });
const shared = domainListSubscriptionFixture({
  id: storyId("e0", 1),
  title: "Shared blocklist of small servers",
  created_at: hoursAgo(400),
  updated_at: hoursAgo(400),
  last_fetch: sharedFetch,
});
const failing = domainListSubscriptionFixture({
  id: storyId("e0", 2),
  title: "Blocks of a Mastodon server",
  url: "https://mastodon.example/api/v1/instance/domain_blocks",
  format: "mastodon_json",
  mode: "automatic",
  consecutive_failures: 3,
  created_at: hoursAgo(900),
  updated_at: hoursAgo(100),
  last_fetch: domainListFetchFixture({
    id: storyId("f0", 2),
    outcome: "failed",
    http_status: 403,
    error_code: "http_status",
    entry_count: 85,
    fetched_at: hoursAgo(5),
  }),
});
const storySubscriptions = [shared, failing];

const storyFetches = [
  sharedFetch,
  domainListFetchFixture({
    id: storyId("f0", 3),
    outcome: "not_modified",
    http_status: 304,
    entry_count: 310,
    fetched_at: hoursAgo(27),
  }),
  domainListFetchFixture({
    id: storyId("f0", 4),
    outcome: "failed",
    error_code: "network",
    entry_count: 310,
    fetched_at: hoursAgo(51),
  }),
];

const storyDrafts = [
  domainListDraftFixture({
    key: "lift:limit:fixed.example",
    change: "lift",
    domain: "fixed.example",
    action_id: storyId("a0", 9),
  }),
  domainListDraftFixture({
    key: "issue:suspend:spam.example",
    domain: "spam.example",
    kind: "suspend",
    severity: "suspend",
    public_comment: "Spam and advertising from every account.",
  }),
  domainListDraftFixture({
    key: `issue:limit:${longDomainAccountFixture.domain}`,
    domain: longDomainAccountFixture.domain,
  }),
  domainListDraftFixture({
    key: "issue:limit:odd.example",
    domain: "odd.example",
    held: "unknown_severity",
    severity: "quarantine",
  }),
];

const handlers = [
  moderatorViewer,
  http.get(`${apiBase}/admin/domain_allows`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: storyAllows })),
  ),
  http.get(`${apiBase}/admin/domain_list_subscriptions`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: storySubscriptions })),
  ),
  http.get(`${apiBase}/admin/domain_list_subscriptions/:id`, ({ request, params }) => {
    const found = storySubscriptions.find((item) => item.id === String(params["id"]));
    if (found === undefined) return undefined;
    return withViewer(request, () => HttpResponse.json(found));
  }),
  http.get(`${apiBase}/admin/domain_list_subscriptions/:id/fetches`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: storyFetches })),
  ),
  http.get(`${apiBase}/admin/domain_list_subscriptions/:id/drafts`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: storyDrafts })),
  ),
];

const meta = {
  title: "Screens/Moderation/DomainLists",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/moderation/domain-actions/allowed", signedIn: true, handlers } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The allowed domains, one with a comment in Japanese and one in non-Latin letters. */
export const Allowlist: Story = showing("Our partner server.");

/** No domain is allowed. */
export const AllowlistEmpty: Story = answering(
  http.get(`${apiBase}/admin/domain_allows`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] })),
  ),
);

/** The subscriptions: one in the draft mode, one automatic that failed three times in a row. */
export const Subscriptions: Story = {
  ...showing("Shared blocklist of small servers"),
  parameters: { screen: { path: "/moderation/domain-actions/subscriptions" } },
};

/** The form of a new subscription. */
export const NewSubscription: Story = {
  parameters: { screen: { path: "/moderation/domain-actions/subscriptions/new" } },
};

/** A subscription's page: drafts to lift, issue and hold, the fetches, the settings, and stopping. */
export const Subscription: Story = {
  ...showing("Spam and advertising from every account."),
  parameters: { screen: { path: `/moderation/domain-actions/subscriptions/${shared.id}` } },
};

/** The drafts have not arrived yet. */
const draftsLoading = loadingOn("get", "/admin/domain_list_subscriptions/:id/drafts");
export const SubscriptionLoading: Story = {
  ...draftsLoading,
  parameters: {
    screen: { ...draftsLoading.parameters.screen, path: `/moderation/domain-actions/subscriptions/${shared.id}` },
  },
};

/** The server failed to answer the subscriptions. */
const subscriptionsFailing = failingOn("get", "/admin/domain_list_subscriptions");
export const ServerError: Story = {
  parameters: {
    screen: { ...subscriptionsFailing.parameters.screen, path: "/moderation/domain-actions/subscriptions" },
  },
};

/** A member without manage_domains is told why. */
export const WithoutPermission: Story = withoutPermission;

/** A subscription's page in Japanese. */
export const Japanese: Story = {
  ...inLocale("ja"),
  parameters: { screen: { path: `/moderation/domain-actions/subscriptions/${shared.id}` } },
};

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = {
  ...inLocale("ar-XB"),
  parameters: { screen: { path: `/moderation/domain-actions/subscriptions/${shared.id}` } },
};

/** The allowlist at 320 CSS px. */
export const Narrow: Story = narrow;

/** A subscription's page at 320 CSS px. */
export const SubscriptionNarrow: Story = {
  ...narrow,
  parameters: { screen: { path: `/moderation/domain-actions/subscriptions/${shared.id}` } },
};
