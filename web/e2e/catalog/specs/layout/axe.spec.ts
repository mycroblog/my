// SPDX-License-Identifier: AGPL-3.0-only

// The axe rules that need computed style (ADR-0171 §2: the contrast of a text on what is painted under it, the size
// of a target) on the design system's parts that draw colours of their own, in both themes: a story per part, the one
// that shows the most of its states. The layout parts' own stories are checked by their layout tests; the screens are
// not rendered per theme, since they are made of these parts.

import { expectNoComputedViolations } from "../axe";
import { test } from "../test";

// One page walks a group of stories, a step each: a page of its own per story costs a context and a cold load each,
// and four groups keep the workers busy side by side.
const groups = [
  [
    "tokens-type-and-color--type",
    "tokens-type-and-color--color",
    "design-system-article--head",
    "design-system-avatar--no-image",
    "design-system-button--variants",
  ],
  [
    "design-system-composer--hashtags",
    "app-components-direct-messages--conversations",
    "design-system-emoji-picker--popover",
    "design-system-follow-requests--follow-requests",
    "design-system-icon-button--pressed",
  ],
  [
    "design-system-inline-prompt--offered",
    "design-system-list-management--list-members",
    "design-system-menu--on-own-post",
    "design-system-name-line--default",
    "design-system-notice-line--linked",
  ],
  [
    "design-system-rows--account-list",
    "design-system-search-field--with-error",
    "design-system-switch--states",
    "design-system-tag-input--full",
  ],
];

for (const stories of groups)
  test(`${stories.join(", ")} pass the computed rules of axe in both themes`, async ({ page, open }) => {
    for (const story of stories)
      await test.step(story, async () => {
        await open(story);
        await expectNoComputedViolations(page);
      });
  });
