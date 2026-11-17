// SPDX-License-Identifier: AGPL-3.0-only

// The axe rules that need computed style (ADR-0171 §2: the contrast of a text on what is painted under it, the size
// of a target) on the design system's parts that draw colours of their own, in both themes: a story per part, the one
// that shows the most of its states. The layout parts' own stories are checked by their layout tests; the screens are
// not rendered per theme, since they are made of these parts.

import { expectNoComputedViolations } from "../axe";
import { test } from "../test";

const stories = [
  "tokens-type-and-color--type",
  "tokens-type-and-color--color",
  "design-system-article--head",
  "design-system-avatar--no-image",
  "design-system-button--variants",
  "design-system-composer--hashtags",
  "design-system-direct-messages--conversations",
  "design-system-emoji-picker--popover",
  "design-system-follow-requests--follow-requests",
  "design-system-icon-button--pressed",
  "design-system-inline-prompt--offered",
  "design-system-list-management--list-members",
  "design-system-menu--on-own-post",
  "design-system-name-line--default",
  "design-system-notice-line--linked",
  "design-system-rows--account-list",
  "design-system-search-field--with-error",
  "design-system-switch--states",
  "design-system-tag-input--full",
];

for (const story of stories)
  test(`${story} passes the computed rules of axe in both themes`, async ({ page, open }) => {
    await open(story);
    await expectNoComputedViolations(page);
  });
